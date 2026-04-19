package cloudclaude

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 真实 mutagen 二进制阈值：v0.18.1 任一平台 binary 都 > 1MB。
// 占位 stub 只有几百字节，故 size > 1MB 同时表明 embed 已被真二进制替换。
const realMutagenMinSize int64 = 1 * 1024 * 1024

// hasRealMutagenEmbed 判断当前平台 embed 是否包含真实二进制（而非占位 stub）。
// 占位场景下，Test_ExtractMutagenBinary_FreshDir / Idempotent / OverwriteWrongVersion
// 三个用例会 t.Skip，避免误报。
func hasRealMutagenEmbed(t *testing.T) bool {
	t.Helper()
	plat := runtime.GOOS + "_" + runtime.GOARCH
	data, err := mutagenFS.ReadFile("mutagen_bin/" + plat + "/mutagen")
	if err != nil {
		return false
	}
	return int64(len(data)) > realMutagenMinSize
}

func Test_ExtractMutagenBinary_FreshDir(t *testing.T) {
	if !hasRealMutagenEmbed(t) {
		t.Skip("当前平台 embed 为占位 stub；运行 scripts/fetch-mutagen-bins.sh 后重测")
	}
	dst := filepath.Join(t.TempDir(), "bin", "mutagen")
	if err := ExtractMutagenBinary(dst); err != nil {
		t.Fatalf("ExtractMutagenBinary 失败: %v", err)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if st.Size() < realMutagenMinSize {
		t.Errorf("dst size = %d，期望 > %d", st.Size(), realMutagenMinSize)
	}
	if mode := st.Mode().Perm(); mode != 0o755 {
		t.Errorf("dst 权限 = %o，期望 0755", mode)
	}
}

func Test_ExtractMutagenBinary_Idempotent(t *testing.T) {
	if !hasRealMutagenEmbed(t) {
		t.Skip("当前平台 embed 为占位 stub；运行 scripts/fetch-mutagen-bins.sh 后重测")
	}
	dst := filepath.Join(t.TempDir(), "bin", "mutagen")
	if err := ExtractMutagenBinary(dst); err != nil {
		t.Fatalf("第一次 Extract 失败: %v", err)
	}
	st1, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("第一次 Stat 失败: %v", err)
	}
	mtime1 := st1.ModTime()

	time.Sleep(100 * time.Millisecond)

	if err := ExtractMutagenBinary(dst); err != nil {
		t.Fatalf("第二次 Extract 失败: %v", err)
	}
	st2, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("第二次 Stat 失败: %v", err)
	}
	if !st2.ModTime().Equal(mtime1) {
		t.Errorf("第二次 Extract 改写了文件（mtime 变化）；want idempotent reuse: %v -> %v", mtime1, st2.ModTime())
	}
}

func Test_ExtractMutagenBinary_OverwriteWrongVersion(t *testing.T) {
	if !hasRealMutagenEmbed(t) {
		t.Skip("当前平台 embed 为占位 stub；运行 scripts/fetch-mutagen-bins.sh 后重测")
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, "mutagen")
	if err := os.WriteFile(dst, []byte("#!/bin/sh\necho v0.99\n"), 0o755); err != nil {
		t.Fatalf("写入假 mutagen 失败: %v", err)
	}
	if err := ExtractMutagenBinary(dst); err != nil {
		t.Fatalf("Extract 失败: %v", err)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if st.Size() < realMutagenMinSize {
		t.Errorf("假 mutagen 应被覆盖为真二进制 (>%d bytes)，实际 size = %d", realMutagenMinSize, st.Size())
	}
}

func Test_ExtractMutagenBinary_UnsupportedPlatform(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "mutagen")
	err := extractMutagenFor("windows_386", dst)
	if err == nil {
		t.Fatalf("不支持的平台应返回 error")
	}
	if !strings.Contains(err.Error(), "MOUNT_MUTAGEN_TRANSPORT_FAILED") {
		t.Errorf("error 应包含 MOUNT_MUTAGEN_TRANSPORT_FAILED 错误码，实际: %v", err)
	}
}
