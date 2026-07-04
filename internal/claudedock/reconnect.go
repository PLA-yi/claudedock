package claudedock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/claudedock/claudedock/internal/claudedock/errcodes"
)

// ConnState 是 BufferedStdin / Reconnector 共享的连接状态枚举。
type ConnState int32

const (
	StateConnected ConnState = iota
	StateReconnecting
	StateGaveUp
)

// ErrReconnectGaveUp 在 fast-retry budget 用尽时由 Reconnector.Run 返回。
// Plan 02 在该错误时退出 ExitNetworkError。
var ErrReconnectGaveUp = errors.New("reconnect gave up after fast-retry budget exceeded")

// backoffSeq 是固定退避序列（CONTEXT D-05 第 1 条）；30s 后维持 30s 周期。
var backoffSeq = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	30 * time.Second,
}

// Reconnector 自实现的 SSH 重连状态机。
//
// 行为（CONTEXT D-05 / D-22 / D-23）：
//   - 退避序列 [1s, 2s, 4s, 8s, 30s]，30s 后维持 30s 周期重试
//   - 每次重连调 sshConnect(cfg)，复用启动期已缓存 SSHConfig.Password（不弹密码）
//   - Trigger() 由 input_buffer 在 \r/\n 时调用，立即唤醒 select；channel 满则 drop
//   - fastRetry 60s 滑动窗口 5 次仍失败 → 返回 ErrReconnectGaveUp
//   - renderStatus goroutine 每 100ms 评估三态 UX 文本写 statusWriter
type Reconnector struct {
	cfg             SSHConfig
	onConnLost      func()
	onReconnected   func(*ssh.Client) error
	triggerCh       chan struct{}
	state           atomic.Int32
	disconnectStart atomic.Int64
	fastRetryCount  int
	fastRetryWindow time.Time
	noColor         bool
	statusWriter    io.Writer
	reconnectCount  atomic.Int64
}

// NewReconnector 用启动期 SSHConfig（含已缓存 password）+ 三个回调构造。
// statusWriter 用于三态 UX 渲染（通常是 os.Stderr）；noColor 控制 ANSI escape 是否输出。
func NewReconnector(cfg SSHConfig, onConnLost func(), onReconnected func(*ssh.Client) error,
	statusWriter io.Writer, noColor bool,
) *Reconnector {
	r := &Reconnector{
		cfg:           cfg,
		onConnLost:    onConnLost,
		onReconnected: onReconnected,
		triggerCh:     make(chan struct{}, 1),
		noColor:       noColor,
		statusWriter:  statusWriter,
	}
	r.state.Store(int32(StateConnected))
	return r
}

// State 返回当前连接状态。
// input_buffer 用其判定走 Connected 直传还是 Reconnecting 缓冲分支。
func (r *Reconnector) State() ConnState { return ConnState(r.state.Load()) }

// StateAddr 暴露 atomic.Int32 指针，供 BufferedStdin 共享读取。
func (r *Reconnector) StateAddr() *atomic.Int32 { return &r.state }

// Trigger 由 input_buffer 在用户按 \r/\n 时调用，立即唤醒重连 select；
// channel 满则 drop（防 Enter spam）。
func (r *Reconnector) Trigger() {
	select {
	case r.triggerCh <- struct{}{}:
	default:
	}
}

// ReconnectCount 暴露累计成功重连次数，用于 last-session.json ReconnectCount 字段写入。
func (r *Reconnector) ReconnectCount() int { return int(r.reconnectCount.Load()) }

// Run 阻塞执行重连循环；正常重连成功 return nil；fastRetry 兜底失败 return ErrReconnectGaveUp。
//
// 内部分两个 goroutine：
//  1. 主 goroutine：select{ ctx.Done | timer.C | triggerCh } + sshConnect retry
//  2. renderStatus goroutine：100ms ticker，按 disconnectStart 评估三态 UX 文本写 statusWriter
func (r *Reconnector) Run(ctx context.Context) error {
	r.disconnectStart.Store(time.Now().UnixNano())
	r.state.Store(int32(StateReconnecting))
	if r.onConnLost != nil {
		r.onConnLost()
	}

	statusCtx, cancelStatus := context.WithCancel(ctx)
	defer cancelStatus()
	go r.renderStatus(statusCtx)

	backoffIdx := 0
	for {
		delay := backoffSeq[backoffIdx]
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-r.triggerCh:
			timer.Stop()
			r.recordFastRetry()
			if r.exceededFastRetryBudget() {
				r.state.Store(int32(StateGaveUp))
				return ErrReconnectGaveUp
			}
		case <-timer.C:
		}

		newConn, err := sshConnect(r.cfg)
		if err == nil {
			if r.onReconnected != nil {
				if cbErr := r.onReconnected(newConn); cbErr != nil {
					_ = newConn.Close()
					if backoffIdx < len(backoffSeq)-1 {
						backoffIdx++
					}
					continue
				}
			}
			r.disconnectStart.Store(0)
			r.reconnectCount.Add(1)
			r.state.Store(int32(StateConnected))
			return nil
		}

		if backoffIdx < len(backoffSeq)-1 {
			backoffIdx++
		}
	}
}

func (r *Reconnector) recordFastRetry() {
	now := time.Now()
	if r.fastRetryWindow.IsZero() || now.Sub(r.fastRetryWindow) > 60*time.Second {
		r.fastRetryWindow = now
		r.fastRetryCount = 1
		return
	}
	r.fastRetryCount++
}

func (r *Reconnector) exceededFastRetryBudget() bool {
	return r.fastRetryCount > 5
}

// renderStatus 每 100ms 评估 disconnectDuration → 输出三态文本（行内覆盖 \r\x1b[K）。
func (r *Reconnector) renderStatus(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	lastRendered := ""
	for {
		select {
		case <-ctx.Done():
			if r.statusWriter != nil && lastRendered != "" {
				fmt.Fprint(r.statusWriter, "\r\x1b[K")
			}
			return
		case <-ticker.C:
			if r.statusWriter == nil {
				continue
			}
			startNs := r.disconnectStart.Load()
			if startNs == 0 {
				if lastRendered != "" {
					fmt.Fprint(r.statusWriter, "\r\x1b[K")
					lastRendered = ""
				}
				continue
			}
			elapsed := time.Duration(time.Now().UnixNano() - startNs)
			text := renderDisconnectStatus(elapsed, r.noColor)
			if text != lastRendered {
				fmt.Fprint(r.statusWriter, "\r\x1b[K"+text)
				lastRendered = text
			}
		}
	}
}

// renderDisconnectStatus 是纯函数，方便单测直接断言（CONTEXT D-22 三态阈值）。
// noColor=true → 不输出 ANSI escape，仅纯文本（D-23）。
func renderDisconnectStatus(d time.Duration, noColor bool) string {
	secs := int(d.Seconds())
	switch {
	case d < 1500*time.Millisecond:
		return ""
	case d < 8*time.Second:
		if noColor {
			return fmt.Sprintf("… %.1fs", d.Seconds())
		}
		return fmt.Sprintf("\x1b[90m… %.1fs\x1b[0m", d.Seconds())
	case d < 30*time.Second:
		if noColor {
			return fmt.Sprintf("⚠ 网络抖动中（%d 秒未响应）", secs)
		}
		return fmt.Sprintf("\x1b[33m⚠ 网络抖动中（%d 秒未响应）\x1b[0m", secs)
	default:
		if noColor {
			return fmt.Sprintf("✗ 网络已断 %d 秒，正在自动重试…", secs)
		}
		return fmt.Sprintf("\x1b[31m✗ 网络已断 %d 秒，正在自动重试…\x1b[0m", secs)
	}
}

// FormatGiveUpMessage 由 Plan 02 在 Run 返回 ErrReconnectGaveUp 后调用，
// 输出 NET_RECONNECT_GAVE_UP 错误码。
func FormatGiveUpMessage(retries int, totalDuration time.Duration) string {
	return errcodes.Format(errcodes.NET_RECONNECT_GAVE_UP, retries, totalDuration.Round(time.Second))
}
