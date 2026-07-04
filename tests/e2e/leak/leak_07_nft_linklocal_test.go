//go:build e2e && linux

// leak_07_nft_linklocal_test.go 是 Phase 49 LEAK-07 的 e2e 主用例：
//
//   - `nft list ruleset` 输出经 ParseNftRules 解析。
//   - 至少一条 Action=drop 且 Dst 前缀 `169.254` 的规则。
//
// 当前 grep `internal/network/firewall_helpers.go` 未发现任何针对 IPv4
// destination 169.254.0.0/16 的显式 drop 规则（只有 IPv6 chain default drop +
// UDP dport drop（mDNS/LLMNR/NetBIOS）+ 链末 sbfw-drop 兜底）。本用例**预期 fail**，
// gap 列入 Phase 51 QUAL-06 / QUAL-07。

package leak

import (
	"context"
	"testing"
	"time"

	e2e "github.com/claudedock/claudedock/tests/e2e"
)

func TestLeak_07_NftLinkLocalDrop_RuleExists(t *testing.T) {
	g, skip := StartLeakGolden(t)
	if skip {
		return
	}
	EnsureDumper(t, g)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := g.ListNftRulesOnHost(ctx)
	if err != nil {
		t.Skipf("nft list ruleset unavailable: %v", err)
		return
	}
	if len(raw) == 0 {
		t.Skipf("nft list ruleset returned empty body; deferred-to-CI")
		return
	}

	rules := e2e.ParseNftRules(raw)
	t.Logf("LEAK-07 parsed %d rules", len(rules))

	if e2e.HasLinkLocalDropRule(rules) {
		// 已显式 drop 169.254 → Pass。
		return
	}

	// 预期 fail：列出所有 drop 规则前 10 条便于审计。
	dropCount := 0
	for _, r := range rules {
		if r.Action != "drop" {
			continue
		}
		if dropCount < 10 {
			t.Logf("  drop rule: table=%q chain=%q dst=%q proto=%q port=%d comment=%q",
				r.Table, r.Chain, r.Dst, r.Proto, r.Port, r.Comment)
		}
		dropCount++
	}
	t.Errorf(
		"LEAK-07 nft 规则集中**未发现** dst 前缀 169.254 的显式 drop 规则（共 %d 条 drop）。"+
			"backend GAP：internal/network/firewall_helpers.go 仅有 IPv6 chain default drop + "+
			"UDP dport drop (mDNS/LLMNR/NetBIOS) + 链末 sbfw-drop 兜底。"+
			"修复方案见 Phase 51 QUAL-06 / QUAL-07：在 worker netns nft output 链显式追加 "+
			"`ip daddr 169.254.0.0/16 counter drop comment \"linklocal-drop\"`。",
		dropCount)
}
