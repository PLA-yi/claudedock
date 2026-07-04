package claudedock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const wrapperTemplate = `#!/bin/bash
CMD=$(basename "$0")
UUID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$$-$(date +%%s%%N)")
DIR="{{REMOTE_PATH}}/.claudedock/exec"
REQ="$DIR/req-$UUID.json"
RES="$DIR/res-$UUID.json"

printf '{"cmd":"%s","args":%s,"cwd":"%s"}\n' \
  "$CMD" \
  "$(printf '%s\n' "$@" | jq -R . | jq -cs .)" \
  "$(pwd)" > "$REQ"

while [ ! -f "$RES" ]; do sleep 0.05; done

cat "$DIR/res-$UUID.stdout" 2>/dev/null
cat "$DIR/res-$UUID.stderr" >&2 2>/dev/null
EXIT=$(jq -r '.exit_code // 1' "$RES")
rm -f "$REQ" "$RES" "$DIR/res-$UUID.stdout" "$DIR/res-$UUID.stderr"
exit "$EXIT"
`

// InstallWrappers 在 localDir/.claudedock/bin/ 下为每个命令创建 wrapper 脚本。
// 这些脚本通过 sshfs 在容器内可见，会拦截指定命令并通过文件 IPC 转发到本地执行。
func InstallWrappers(localDir string, commands []string, remotePath string) error {
	binDir := filepath.Join(localDir, ".claudedock", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建 wrapper bin 目录失败: %w", err)
	}

	script := strings.ReplaceAll(wrapperTemplate, "{{REMOTE_PATH}}", remotePath)

	for _, cmd := range commands {
		path := filepath.Join(binDir, cmd)
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			return fmt.Errorf("写入 wrapper 脚本 %s 失败: %w", cmd, err)
		}
	}

	return nil
}
