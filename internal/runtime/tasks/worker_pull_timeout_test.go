package tasks

import (
	"testing"
	"time"
)

// TestPullImageTimeout_IsBounded 守护 pullImage 单次超时的合理范围 —
// 太短会让大镜像 layer 拉取失败回退到本地旧镜像；太长会回到 Phase 33 那次精神分裂事故
// （rebuild_host 卡死 + DB host=running、容器 not found、task=pending 的三态不一致）。
//
// 行为验证（pullImage 真的被 ctx 卡住返回）由集成测试覆盖；本测试只锁常量边界。
func TestPullImageTimeout_IsBounded(t *testing.T) {
	if pullImageTimeout <= 0 {
		t.Fatalf("pullImageTimeout must be > 0, got %v", pullImageTimeout)
	}
	if pullImageTimeout < time.Minute {
		t.Errorf("pullImageTimeout %v < 1m: too short, large image layers may abort prematurely", pullImageTimeout)
	}
	if pullImageTimeout > 30*time.Minute {
		t.Errorf("pullImageTimeout %v > 30m: too long, host action may hold for an unacceptable duration", pullImageTimeout)
	}
}
