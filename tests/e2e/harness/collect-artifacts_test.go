// Package harness 中的 collect-artifacts_test.go 验证 Phase 52 OBS-01 落地的
// `collect-artifacts.sh` 脚本：5 子目录建得齐、metadata.txt 写得对、缺工具
// 时不返回非 0、脚本源码内无个人路径泄露。
//
// 设计原则：
//   - 本文件**无 build tag**，darwin 上 `go test ./tests/e2e/harness/` 裸跑也跑。
//   - 不依赖 docker / nft / pg_dump，所有断言基于「目录结构 + 文件存在 + 元数据字段」。
//   - 用 t.TempDir() 作为输出目录，永不污染仓库根的 ./out/。
package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scriptPath 返回 collect-artifacts.sh 在仓库内的绝对路径（与本测试文件同目录）。
func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "collect-artifacts.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("collect-artifacts.sh not found at %s: %v", p, err)
	}
	return p
}

// runScript 调用 bash <scriptPath> <out> <scenario>，返回 (combined output, error)。
func runScript(t *testing.T, out, scenario string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", scriptPath(t), out, scenario)
	// 显式清掉可能干扰 postgres 采集的环境变量，避免本机有 DATABASE_URL 时跑出
	// 真实 schema dump 拖慢测试 / 引入外部依赖。
	env := []string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DATABASE_URL=") || strings.HasPrefix(kv, "PG_DUMP_URL=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// TestCollectArtifacts_Creates5SubdirsAndMetadata：脚本跑完后 5 子目录 + metadata.txt 全部存在。
func TestCollectArtifacts_Creates5SubdirsAndMetadata(t *testing.T) {
	out := t.TempDir()
	output, err := runScript(t, out, "smoke")
	if err != nil {
		t.Fatalf("script exit non-zero: %v; output=%s", err, output)
	}
	if !strings.Contains(output, "[collect-artifacts] done:") {
		t.Fatalf("missing done banner: %s", output)
	}

	root := filepath.Join(out, "smoke")
	for _, sub := range []string{"logs", "network", "docker", "postgres", "system"} {
		p := filepath.Join(root, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a dir", p)
		}
	}

	metaPath := filepath.Join(root, "metadata.txt")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	meta := string(data)
	for _, key := range []string{
		"timestamp=",
		"scenario=smoke",
		"hostname=",
		"kernel=",
		"git_sha=",
		"runner=",
		"script_version=v1",
	} {
		if !strings.Contains(meta, key) {
			t.Fatalf("metadata.txt missing %q; full content:\n%s", key, meta)
		}
	}
}

// TestCollectArtifacts_ExitZeroWithoutDocker：脚本在无 docker / 无 pg 环境下仍 exit 0。
func TestCollectArtifacts_ExitZeroWithoutDocker(t *testing.T) {
	out := t.TempDir()
	_, err := runScript(t, out, "no-deps")
	if err != nil {
		t.Fatalf("script must exit 0 even without docker/pg; got err=%v", err)
	}

	// 至少其中一个子目录会因为缺工具留下占位文件
	// （darwin 上 logs 与 postgres 几乎一定缺；ubuntu 上 docker 通常有）
	root := filepath.Join(out, "no-deps")
	placeholders := []string{
		filepath.Join(root, "postgres", "_skipped.txt"),
	}
	foundAny := false
	for _, p := range placeholders {
		if _, err := os.Stat(p); err == nil {
			foundAny = true
			break
		}
	}
	if !foundAny {
		t.Logf("note: no placeholder file found at %v (may be running in full CI env, OK)", placeholders)
	}
}

// TestCollectArtifacts_ScenarioIdSubpath：scenario-id 出现在路径中作为子目录。
func TestCollectArtifacts_ScenarioIdSubpath(t *testing.T) {
	out := t.TempDir()
	if _, err := runScript(t, out, "test-case-xyz"); err != nil {
		t.Fatalf("script err: %v", err)
	}
	expected := filepath.Join(out, "test-case-xyz", "metadata.txt")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected scenario subdir not created at %s: %v", expected, err)
	}
}

// TestCollectArtifacts_DefaultScenarioWhenOmitted：未传 scenario-id 时回退到 "default"。
func TestCollectArtifacts_DefaultScenarioWhenOmitted(t *testing.T) {
	out := t.TempDir()
	cmd := exec.Command("bash", scriptPath(t), out)
	cmd.Env = []string{}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script err: %v; output=%s", err, output)
	}
	expected := filepath.Join(out, "default", "metadata.txt")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected default scenario subdir at %s: %v", expected, err)
	}
}

// TestCollectArtifacts_FailsWithoutOutputDir：缺 output-dir 参数时必须非 0 退出（错误提示）。
func TestCollectArtifacts_FailsWithoutOutputDir(t *testing.T) {
	cmd := exec.Command("bash", scriptPath(t))
	cmd.Env = []string{}
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("script should fail without args; got success, output=%s", output)
	}
}

// TestCollectArtifacts_CopiesReadmes：脚本跑完后 5 子目录都有 README.md（Plan 02 模板）。
//
// 模板源是 tests/e2e/harness/artifacts/<sub>/README.md，由 Plan 02 落地；脚本内
// copy_readmes() 函数在 Plan 01 时已预埋，模板缺失时静默跳过（|| true），模板就位
// 时立即生效，无需再改脚本。
func TestCollectArtifacts_CopiesReadmes(t *testing.T) {
	out := t.TempDir()
	if _, err := runScript(t, out, "readme-check"); err != nil {
		t.Fatalf("script err: %v", err)
	}
	root := filepath.Join(out, "readme-check")
	for _, sub := range []string{"logs", "network", "docker", "postgres", "system"} {
		readme := filepath.Join(root, sub, "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			t.Fatalf("README missing at %s: %v", readme, err)
		}
		content := string(data)
		if !strings.Contains(content, "Phase 52") {
			t.Fatalf("%s missing 'Phase 52' marker: %s", readme, firstN(content, 120))
		}
		if !strings.Contains(content, "排障") {
			t.Fatalf("%s missing '排障' keyword: %s", readme, firstN(content, 120))
		}
	}
}

// firstN 返回前 n 个字符（按 byte 切，README 是 utf-8 但断言失败信息够用了）。
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TestCollectArtifacts_NoAbsoluteUserPathsInScript：脚本源码内不含个人绝对路径字面量。
//
// CONVENTIONS.md §Privacy 守护：禁止在被 git 跟踪的文件中写 /Users/<user>/ 或
// /home/<user>/ 这种带具体用户名的路径。脚本本身允许出现 `/home/` 或 `/Users/`
// 通用前缀（如注释里写「不在 /Users 下放代码」），但不允许出现具体用户名。
//
// 这里用保守的扫描：grep `/Users/zaneliu` / `/home/zaneliu` 这类，命中 0 次即合规。
// 同时也扫几个常见个人邮箱后缀防止误植。
func TestCollectArtifacts_NoAbsoluteUserPathsInScript(t *testing.T) {
	data, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	content := string(data)
	forbidden := []string{
		"/Users/zaneliu",
		"/home/zaneliu",
		"@gmail.com",
		"@qq.com",
		"@outlook.com",
	}
	for _, kw := range forbidden {
		if strings.Contains(content, kw) {
			t.Fatalf("collect-artifacts.sh contains forbidden private string %q", kw)
		}
	}
}
