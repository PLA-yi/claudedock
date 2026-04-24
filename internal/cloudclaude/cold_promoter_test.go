package cloudclaude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- Test 1: Dedup ----------

func TestPromotionDedup(t *testing.T) {
	hotDir := t.TempDir()
	var logBuf bytes.Buffer

	cp := NewColdPromoter(nil, "/fake/cold", hotDir, &logBuf, filepath.Join(t.TempDir(), "cold-promoter.pid"))

	// 注入 mock copyFile：记录被调用的 path 和次数
	var copyCalls atomic.Int64
	var copiedPaths []string
	var copyMu sync.Mutex

	origCopyFn := promoterCopyFileFn
	t.Cleanup(func() { promoterCopyFileFn = origCopyFn })

	promoterCopyFileFn = func(cp *ColdPromoter, remotePath, localPath string) (int64, error) {
		copyCalls.Add(1)
		copyMu.Lock()
		copiedPaths = append(copiedPaths, remotePath)
		copyMu.Unlock()
		// 模拟成功写入 100 bytes
		return 100, nil
	}

	// 启动消费循环
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cp.runPromotionLoop(ctx)

	// 50 次 enqueue 同一 path，100ms 内完成
	start := time.Now()
	for i := 0; i < 50; i++ {
		cp.enqueue("test.bin")
	}
	enqueueElapsed := time.Since(start)
	t.Logf("50 次 enqueue 耗时: %v", enqueueElapsed)

	// 等待异步处理完成
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond) // 等待最后一个 promotePath goroutine 完成

	// 断言：实际 copy 调用次数 == 1
	if got := copyCalls.Load(); got != 1 {
		t.Errorf("copyFromCold 调用次数 = %d, want 1（去重失败）", got)
	}

	count, bytes, _ := cp.Stats()
	if count != 1 {
		t.Errorf("promotionCount = %d, want 1", count)
	}
	if bytes != 100 {
		t.Errorf("promotionBytes = %d, want 100", bytes)
	}
}

// ---------- Test 2: Retry Backoff ----------

func TestPromotionRetryBackoff(t *testing.T) {
	hotDir := t.TempDir()
	var logBuf bytes.Buffer

	cp := NewColdPromoter(nil, "/fake/cold", hotDir, &logBuf, filepath.Join(t.TempDir(), "cold-promoter.pid"))

	var attemptCount atomic.Int64

	origCopyFn := promoterCopyFileFn
	t.Cleanup(func() { promoterCopyFileFn = origCopyFn })

	promoterCopyFileFn = func(cp *ColdPromoter, remotePath, localPath string) (int64, error) {
		n := attemptCount.Add(1)
		if n < 3 {
			return 0, fmt.Errorf("模拟失败 attempt %d", n)
		}
		return 200, nil
	}

	start := time.Now()
	cp.promotePath("retry.bin")
	elapsed := time.Since(start)

	t.Logf("重试总耗时: %v, 尝试次数: %d", elapsed, attemptCount.Load())

	// 验证总耗时应在 1s+2s 之间（第 1 次立即失败 → sleep 1s → 第 2 次失败 → sleep 2s → 第 3 次成功）
	if elapsed < 1*time.Second {
		t.Errorf("重试耗时 %v < 1s，未触发退避", elapsed)
	}
	if elapsed > 4*time.Second {
		t.Errorf("重试耗时 %v > 4s，退避超预期", elapsed)
	}

	if got := attemptCount.Load(); got != 3 {
		t.Errorf("尝试次数 = %d, want 3", got)
	}

	count, bytes, failed := cp.Stats()
	if count != 1 {
		t.Errorf("promotionCount = %d, want 1（第 3 次应成功）", count)
	}
	if bytes != 200 {
		t.Errorf("promotionBytes = %d, want 200", bytes)
	}
	if failed != 0 {
		t.Errorf("promotionFailedCount = %d, want 0", failed)
	}
}

// ---------- Test 3: Circuit Breaker ----------

func TestPromotionCircuitBreaker(t *testing.T) {
	hotDir := t.TempDir()
	var logBuf bytes.Buffer

	cp := NewColdPromoter(nil, "/fake/cold", hotDir, &logBuf, filepath.Join(t.TempDir(), "cold-promoter.pid"))

	var attemptCount atomic.Int64

	origCopyFn := promoterCopyFileFn
	t.Cleanup(func() { promoterCopyFileFn = origCopyFn })

	// alwaysFailing: 永远失败
	promoterCopyFileFn = func(cp *ColdPromoter, remotePath, localPath string) (int64, error) {
		attemptCount.Add(1)
		return 0, errors.New("模拟永久失败")
	}

	// 第一次 promotePath：3 次重试全失败 → 熔断
	cp.promotePath("breaker.bin")
	callsBeforeSecond := attemptCount.Load()

	// 第二次 promotePath：应立即返回（熔断）
	cp.promotePath("breaker.bin")
	callsAfterSecond := attemptCount.Load()

	// 断言：总调用次数 == 第一次的 3 次（第二次不触发拉取）
	if callsAfterSecond != callsBeforeSecond {
		t.Errorf("第二次 promotePath 仍触发了 copy: before=%d, after=%d", callsBeforeSecond, callsAfterSecond)
	}
	if callsAfterSecond != 3 {
		t.Errorf("copy 调用次数 = %d, want 3（第一次 promotePath 的 3 次重试）", callsAfterSecond)
	}

	_, _, failed := cp.Stats()
	if failed != 1 {
		t.Errorf("promotionFailedCount = %d, want 1", failed)
	}

	if !strings.Contains(logBuf.String(), "晋升失败 breaker.bin") {
		t.Errorf("stderr 缺少熔断日志，内容: %s", logBuf.String())
	}
}

// ---------- Test 4: Start / Stop ----------

func TestPromoterStartStop(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "cold-promoter.pid")

	cp := NewColdPromoter(nil, "/fake/cold", "/fake/hot", io.Discard, pidFile)

	// 重写 inotify hooks
	origInit := promoterInitInotifyFn
	origClose := promoterCloseInotifyFn
	origRead := promoterReadEventsFn
	t.Cleanup(func() {
		promoterInitInotifyFn = origInit
		promoterCloseInotifyFn = origClose
		promoterReadEventsFn = origRead
	})

	promoterInitInotifyFn = func(coldRoot string) (int, error) {
		f, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
		if err != nil {
			return -1, err
		}
		return int(f.Fd()), nil
	}
	promoterCloseInotifyFn = func(fd int) error { return nil }
	// readEvents 模拟无事件（每次都返回 nil，由调用方的 ctx 取消控制退出）
	promoterReadEventsFn = func(fd int, buf []byte, enqueue func(string)) error {
		// 小 sleep 避免忙循环
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		cp.Run(ctx)
		close(done)
	}()

	// 等 Run 启动并写完 PID file
	time.Sleep(50 * time.Millisecond)

	// 断言 PID file 存在
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("PID file %s 不存在: %v", pidFile, err)
	}

	// cancel 并等待 Run 返回
	cancel()

	select {
	case <-done:
		// Run 已返回
	case <-time.After(5 * time.Second):
		t.Fatal("Run 在 ctx cancel 后 5s 内未返回")
	}

	// 断言 PID file 已清理
	if _, err := os.Stat(pidFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("PID file %s 未被清理: %v", pidFile, err)
	}
}
