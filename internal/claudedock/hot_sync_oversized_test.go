package claudedock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 单元测试 fixture：
//
//   - 通过 Truncate 创建稀疏文件，避免占用真实磁盘空间（Phase 36 D-21）。
//   - 三场景对应 plan 的 behavior 列表（60MB / ignore / 30MB），覆盖
//     Phase 36 D-06 单文件熔断的边界与 ignore 第一层互补语义。
func createFixtureFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建 fixture 文件失败: %v", err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Fatalf("Truncate %s 失败: %v", path, err)
	}
}

// applyTestOversizedFilter 镜像 HotSyncEngine.applyOversizedFilter 的 initialSync 行为，
// 让单测可以在不触达 SSH 链路的前提下断言「scanLocalSyncFiles + 单文件熔断」契约。
//
// 与生产实现保持同一阈值与同一语义（>= maxBytes 进入 oversized 并 delete）。
func applyTestOversizedFilter(localFiles map[string]syncFileState, maxBytes int64) []OversizedFile {
	var oversized []OversizedFile
	for rel, state := range localFiles {
		if state.Size >= maxBytes {
			oversized = append(oversized, OversizedFile{Path: rel, SizeBytes: state.Size})
			delete(localFiles, rel)
		}
	}
	return oversized
}

// TestHotSyncOversized_60MB_NotIgnored 断言 60MB 未 ignore 文件进入 OversizedFiles
// 并被 delete 出 localFiles，对应 plan behavior #1。
func TestHotSyncOversized_60MB_NotIgnored(t *testing.T) {
	dir := t.TempDir()
	createFixtureFile(t, filepath.Join(dir, "bigfile.bin"), 60*1024*1024)

	matcher := NewIgnoreMatcher(dir, nil)
	localFiles, err := scanLocalSyncFiles(dir, matcher, nil)
	if err != nil {
		t.Fatalf("scanLocalSyncFiles 失败: %v", err)
	}

	const maxBytes = 50 * 1024 * 1024
	oversized := applyTestOversizedFilter(localFiles, maxBytes)

	if len(oversized) != 1 {
		t.Fatalf("oversized 应含 1 条，got %d: %+v", len(oversized), oversized)
	}
	if oversized[0].Path != "bigfile.bin" {
		t.Errorf("oversized[0].Path 应为 bigfile.bin，got %q", oversized[0].Path)
	}
	if oversized[0].SizeBytes != 60*1024*1024 {
		t.Errorf("oversized[0].SizeBytes 应为 60MB，got %d", oversized[0].SizeBytes)
	}
	if _, ok := localFiles["bigfile.bin"]; ok {
		t.Error("bigfile.bin 应已从 localFiles 中删除")
	}
}

// TestHotSyncOversized_IgnoreHit_NotCounted 断言 ignore 命中的 60MB 文件不进入 OversizedFiles
// （第一层 ignore 已在 scanLocalSyncFiles 内部跳过，第二层熔断观察不到），
// 对应 plan behavior #2。
func TestHotSyncOversized_IgnoreHit_NotCounted(t *testing.T) {
	dir := t.TempDir()
	createFixtureFile(t, filepath.Join(dir, "ignored_big.bin"), 60*1024*1024)

	matcher := NewIgnoreMatcher(dir, []string{"ignored_big.bin"})
	localFiles, err := scanLocalSyncFiles(dir, matcher, nil)
	if err != nil {
		t.Fatalf("scanLocalSyncFiles 失败: %v", err)
	}

	if _, ok := localFiles["ignored_big.bin"]; ok {
		t.Fatalf("ignore 命中的文件不应出现在 scanLocalSyncFiles 结果中: %+v", localFiles)
	}

	const maxBytes = 50 * 1024 * 1024
	oversized := applyTestOversizedFilter(localFiles, maxBytes)

	if len(oversized) != 0 {
		t.Errorf("ignore 命中的文件不应进入 oversized，got %+v", oversized)
	}
}

// TestHotSyncOversized_30MB_NotOversized 断言 30MB 未 ignore 文件不进入 OversizedFiles
// 且保留在 localFiles，对应 plan behavior #3。
func TestHotSyncOversized_30MB_NotOversized(t *testing.T) {
	dir := t.TempDir()
	createFixtureFile(t, filepath.Join(dir, "medium.bin"), 30*1024*1024)

	matcher := NewIgnoreMatcher(dir, nil)
	localFiles, err := scanLocalSyncFiles(dir, matcher, nil)
	if err != nil {
		t.Fatalf("scanLocalSyncFiles 失败: %v", err)
	}

	const maxBytes = 50 * 1024 * 1024
	oversized := applyTestOversizedFilter(localFiles, maxBytes)

	if len(oversized) != 0 {
		t.Errorf("30MB 文件不应进入 oversized，got %+v", oversized)
	}
	if _, ok := localFiles["medium.bin"]; !ok {
		t.Error("medium.bin 应保留在 localFiles 中")
	}
}

// TestHotSyncOversized_HotOnly_DoesNotClobberLocalOrDeleteRemote 是 CR-01 的回归测试。
//
// 场景：HotOnly + resetRemote=false（cfg.Cwd 直接做 hot 根），本地 60MB big.bin、
// 远端同路径有 30MB 旧版本、e.last 因上次 Full 模式遗留含同 rel 的 base 状态。
// 修复前两段静默数据丢失：
//   - initialSync：filter 把 big.bin 从 localFiles 删除 → !hasLocal && hasRemote →
//     chooseConflictWinner 返回 "remote" → applyRemote 把远端 30MB 旧版本回写本地，覆盖 60MB。
//   - syncOnce：filter 把 big.bin 从 localFiles 删除但 e.last 仍存 → localChanged=true,
//     remoteChanged=false → applyLocal({},false) → deleteRemote → 远端 big.bin 被删除。
//
// 修复后 applyOversizedFilter 同时 delete(e.last, rel) 并返回 oversizedSet，
// 调用方再 delete(remoteFiles, rel)，让大文件对状态机彻底「不存在」。
//
// 本测试不触达 SSH/SFTP，直接断言 applyOversizedFilter 的 3 项不变量：
//  1. localFiles 中移除 oversized rel
//  2. e.last 中移除 oversized rel（防 syncOnce 把它当本地删除 → 远端被删）
//  3. 返回 oversizedSet 含该 rel（防 initialSync 把远端旧版本反向覆盖本地）
func TestHotSyncOversized_HotOnly_DoesNotClobberLocalOrDeleteRemote(t *testing.T) {
	const maxBytes = int64(50 * 1024 * 1024)
	rel := "big.bin"

	e := &HotSyncEngine{
		maxFileBytes: maxBytes,
		last: map[string]syncFileState{
			// 模拟「上次 Full 模式留下的 base 集」中含该大文件（典型 HotOnly 触发链路）
			rel: {Size: 30 * 1024 * 1024, ModTime: time.Unix(1700000000, 0).UTC()},
		},
	}
	localFiles := map[string]syncFileState{
		rel: {Size: 60 * 1024 * 1024, ModTime: time.Unix(1700000100, 0).UTC()},
	}
	remoteFiles := map[string]syncFileState{
		rel: {Size: 30 * 1024 * 1024, ModTime: time.Unix(1700000000, 0).UTC()},
	}

	oversizedSet := e.applyOversizedFilter(localFiles, true)

	if _, ok := localFiles[rel]; ok {
		t.Errorf("CR-01: oversized rel 应从 localFiles 移除（避免被推上 hot），got %+v", localFiles)
	}
	if _, ok := e.last[rel]; ok {
		t.Errorf("CR-01: oversized rel 必须从 e.last 移除，否则 syncOnce 误判本地删除 → deleteRemote 静默删远端，got %+v", e.last)
	}
	if _, ok := oversizedSet[rel]; !ok {
		t.Fatalf("CR-01: applyOversizedFilter 必须返回含该 rel 的 oversizedSet 给调用方，got %+v", oversizedSet)
	}

	for r := range oversizedSet {
		delete(remoteFiles, r)
	}
	if _, ok := remoteFiles[rel]; ok {
		t.Errorf("CR-01: 调用方按 oversizedSet 剔除后 remoteFiles 应不再含该 rel（防 initialSync chooseConflictWinner 把远端旧版本反向覆盖本地），got %+v", remoteFiles)
	}

	if len(e.oversized) != 1 || e.oversized[0].Path != rel || e.oversized[0].SizeBytes != 60*1024*1024 {
		t.Errorf("CR-01: e.oversized 应记录该 60MB 文件供 last-session.json 与 doctor 复用，got %+v", e.oversized)
	}
}
