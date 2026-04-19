package cloudclaude

import (
	"runtime"
	"testing"
)

func Test_IsCaseInsensitiveFS_TempDir(t *testing.T) {
	dir := t.TempDir()
	got := IsCaseInsensitiveFS(dir)

	switch runtime.GOOS {
	case "linux":
		if got {
			t.Errorf("Linux 临时目录通常 ext4 / tmpfs，应返回 false（case-sensitive），实际返回 true")
		}
	case "darwin", "windows":
		// macOS APFS 默认 case-insensitive 但用户可能创建 case-sensitive 卷；
		// Windows NTFS 默认 case-insensitive 但 dev 容器可能 case-sensitive。
		// 不做强断言，只要求不 panic 且返回 bool（go 类型系统已保证）。
		_ = got
	default:
		_ = got
	}
}

func Test_IsCaseInsensitiveFS_NoWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 路径语义不同，跳过 NoWrite 用例")
	}
	got := IsCaseInsensitiveFS("/dev/null/x-not-a-dir")
	if got {
		t.Errorf("不可写路径应返回 false（保守降级），实际返回 true")
	}
}
