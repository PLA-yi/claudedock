---
phase: 31-cli
plan: 01-errcodes-mutagen-embed
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/cloudclaude/errcodes/codes.go
  - internal/cloudclaude/errcodes/mount.go
  - internal/cloudclaude/errcodes/net.go
  - internal/cloudclaude/errcodes/codes_test.go
  - internal/cloudclaude/mutagen_bin.go
  - internal/cloudclaude/mutagen_bin_test.go
  - internal/cloudclaude/envcheck_fs.go
  - internal/cloudclaude/envcheck_fs_test.go
  - internal/cloudclaude/mutagen_bin/.gitattributes
  - internal/cloudclaude/mutagen_bin/darwin_amd64/mutagen
  - internal/cloudclaude/mutagen_bin/darwin_arm64/mutagen
  - internal/cloudclaude/mutagen_bin/linux_amd64/mutagen
  - internal/cloudclaude/mutagen_bin/linux_arm64/mutagen
  - internal/cloudclaude/mutagen_bin/SHA256SUMS
  - scripts/fetch-mutagen-bins.sh
autonomous: true
requirements:
  - REQ-F8-A
  - REQ-F8-B
must_haves:
  truths:
    - "errcodes 包导出 Code/Severity/Entry/Registry()/Lookup()/Format() 六类对外 API，Phase 34 doctor 可直接复用"
    - "errcodes 注册表至少 15 条（11 个 MOUNT_* + 1 个 MOUNT_MUTAGEN_TRANSPORT_FAILED + 3 个 NET_*），全部通过命名规范 + 中文文案非空 + NextAction ≤ 80 runes 单测"
    - "errcodes.Format(code, args...) 输出严格匹配模板：第一行 [<CODE>] <Message>；第二行两空格缩进 建议: <NextAction>"
    - "internal/cloudclaude/mutagen_bin.go 通过 //go:embed 嵌入 4 平台 mutagen 二进制（按 GOOS_GOARCH 选择），ExtractTo 函数幂等：已存在且 mutagen version 含 0.18.1 直接复用，否则覆盖写入 0755"
    - "scripts/fetch-mutagen-bins.sh 拉取 mutagen v0.18.1 4 平台二进制并校验 SHA256SUMS，失败时退出非 0；脚本可重复执行"
    - "envcheck_fs.go 提供 IsCaseInsensitiveFS(dir string) bool 跨平台 probe（os.CreateTemp + 大小写变体 Stat），不依赖 macOS diskutil"
  artifacts:
    - path: "internal/cloudclaude/errcodes/codes.go"
      provides: "错误码 Code/Severity/Entry 类型 + Registry/Lookup/MustRegister/Format 工具函数"
      contains: "func Format"
    - path: "internal/cloudclaude/errcodes/mount.go"
      provides: "MOUNT_* 系列 12 条注册（含 MOUNT_MUTAGEN_TRANSPORT_FAILED）"
      contains: "MOUNT_MUTAGEN_VERSION_SKEW"
    - path: "internal/cloudclaude/errcodes/net.go"
      provides: "NET_OAUTH_* 三态注册"
      contains: "NET_OAUTH_EXPIRED"
    - path: "internal/cloudclaude/errcodes/codes_test.go"
      provides: "注册表完整性单测：去重、命名正则、中文文案非空、NextAction 长度、≥15 条计数"
      contains: "TestErrcodesRegistry"
    - path: "internal/cloudclaude/mutagen_bin.go"
      provides: "go:embed Mutagen v0.18.1 多平台二进制 + ExtractTo 幂等抽取"
      contains: "//go:embed mutagen_bin"
    - path: "internal/cloudclaude/mutagen_bin/SHA256SUMS"
      provides: "4 平台 mutagen 二进制 sha256 列表（fetch 脚本写入，运行时校验）"
      contains: "mutagen"
    - path: "internal/cloudclaude/envcheck_fs.go"
      provides: "跨平台大小写敏感探测函数 IsCaseInsensitiveFS"
      contains: "IsCaseInsensitiveFS"
    - path: "scripts/fetch-mutagen-bins.sh"
      provides: "从 GitHub release 拉取 mutagen v0.18.1 4 平台二进制 + sha256 校验 + 落到 internal/cloudclaude/mutagen_bin/<plat>/mutagen"
      contains: "v0.18.1"
  key_links:
    - from: "internal/cloudclaude/mutagen_bin.go"
      to: "internal/cloudclaude/mutagen_bin/<GOOS_GOARCH>/mutagen"
      via: "go:embed mutagen_bin"
      pattern: "go:embed mutagen_bin"
    - from: "internal/cloudclaude/errcodes/codes_test.go"
      to: "internal/cloudclaude/errcodes/{mount,net}.go"
      via: "init() MustRegister 调用，TestErrcodesRegistry 遍历全表"
      pattern: "MustRegister"
---

<plan_dependencies>
- 无（Wave 1 起点；Plan 02 / Plan 03 依赖本 plan 产出的 errcodes 包与 mutagen_bin 抽取能力）
</plan_dependencies>

<objective>
为 Phase 31 后续两个 plan 搭好底座：

1. **errcodes 包雏形**：落地 Phase 34 doctor / explain 复用的统一错误码注册表（CONTEXT D-02 / D-19 / D-20 / D-21 / D-32；RESEARCH §5），提前在 Phase 31 兑现 REQ-F8-A / REQ-F8-B 的命名规范与三要素输出格式。
2. **Mutagen 多平台二进制 embed 基础设施**：按 CONTEXT D-03 / D-04 + RESEARCH §1.1 修订（单文件 ~12MB、共 ~49MB、4 平台并存）落地 `go:embed` + `ExtractTo` + sha256 校验脚本。Plan 02 的 `mountMutagen` 在此基础上仅需 `extract → daemon start → sync create`。
3. **跨平台 case-insensitive 文件系统探测**：按 RESEARCH §7 修订 D-09，用 Go probe 替代 `diskutil`，供 Plan 02 的 `mount_strategy.go` 在 macOS APFS 时强制 two-way-resolved 模式。

Purpose: Wave 1 必须提供下游 Wave 2/3 所需要的所有公共类型、二进制、探测函数；Wave 2/3 不再回头改 errcodes / fetch 脚本。
Output: 一个新包 + 两个工具函数 + 4 个二进制 + 1 个拉取脚本，**不**改动任何 v2.0 现网代码路径。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/31-cli/31-RESEARCH.md
@internal/cloudclaude/ssh_doctor.go
@internal/cloudclaude/envcheck.go

<interfaces>
<!-- 本 plan 创建的对外接口，Plan 02/03 与 Phase 34 doctor 必须按此调用。 -->

internal/cloudclaude/errcodes/codes.go 导出：

```go
package errcodes

type Code string

type Severity int
const (
    SeverityInfo Severity = iota
    SeverityWarn
    SeverityError
    SeverityFatal
)

type Entry struct {
    Code       Code
    Severity   Severity
    Message    string // 可含 %s/%d 等 fmt 占位符
    NextAction string // 中文，长度 ≤ 80 runes
}

// MustRegister 注册一条错误码；duplicate code 或不合法格式 panic。
// 由各域文件的 init() 调用，单元测试遍历 Registry 验证。
func MustRegister(e Entry)

// Lookup 根据 Code 取出 Entry；未注册返回 (zero, false)。
func Lookup(c Code) (Entry, bool)

// Registry 返回注册表的浅拷贝（避免外部直接修改）；Phase 34 doctor 与 explain 子命令复用。
func Registry() map[Code]Entry

// Format 渲染统一两段输出：
//   [<CODE>] <Message>
//     建议: <NextAction>
// args 用于填充 Message 中的 %s/%d 占位。code 未注册时 panic（由调用方在测试中保证调用前 Lookup）。
func Format(c Code, args ...any) string
```

internal/cloudclaude/mutagen_bin.go 导出：

```go
package cloudclaude

// MutagenBinaryVersion 是 embed 的 Mutagen 客户端版本。Plan 02 的版本握手会比对此常量与远端 /etc/cloud-claude/mutagen.version。
const MutagenBinaryVersion = "v0.18.1"

// ExtractMutagenBinary 把当前 GOOS_GOARCH 的 embed mutagen 二进制写到 dst（建议 ~/.cloud-claude/bin/mutagen）。
// 行为：
//   1. 父目录不存在则 0700 创建（与 ConfigDir 一致）
//   2. 目标已存在且执行 `<dst> version` 输出含 "v0.18.1" → 视为复用，直接 return nil（幂等）
//   3. 否则覆盖写入，权限 0755
//   4. 当前平台无对应 embed 二进制 → 返回 errcodes.Format(MOUNT_MUTAGEN_TRANSPORT_FAILED, ...) 包装的 error
func ExtractMutagenBinary(dst string) error

// IsCaseInsensitiveFS 跨平台 probe 当前路径所在文件系统是否大小写不敏感（macOS APFS 默认 / Windows）。
// 行为：CreateTemp 一个含小写名的文件，Stat 其全大写变体；Stat 成功且文件名不同 → true。
// 失败（无写权限、临时目录不可用）→ 返回 false（保守降级，避免 panic）。
func IsCaseInsensitiveFS(dir string) bool
```
</interfaces>

<errcode_registry>
<!-- 必须按下表完整注册（共 15 条），文案严格按 RESEARCH §5.2。Plan 02/03 直接 errcodes.Format(...) 调用。 -->

| Code | Severity | Message（可含 %s/%d） | NextAction（≤ 80 runes） |
|------|----------|----------------------|------------------------|
| MOUNT_MUTAGEN_VERSION_SKEW | SeverityError | Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only | 升级容器镜像到 v3.0.0+ 或重装 cloud-claude |
| MOUNT_MUTAGEN_WHITELIST_REJECT | SeverityError | 同步候选目录 %s 体积 %dMB（>50MB），已自动降级 sshfs。当前最大子目录: %s | 在 .mutagen.yml 添加 ignore 规则，或运行 du -sh %s/* 查看大目录 |
| MOUNT_MUTAGEN_SAFETY_GUARD | SeverityFatal | 检测到本地目录 %s 为空但容器内 /workspace-hot 已有文件，拒绝同步以防反向清空 | 如确认从远端拉取，先 cloud-claude exec rsync /workspace-hot/ ./ |
| MOUNT_MUTAGEN_DAEMON_UNAVAILABLE | SeverityError | Mutagen daemon 启动失败: %s | 检查 ~/.cloud-claude/mutagen/ 目录权限，或重启 cloud-claude |
| MOUNT_MUTAGEN_SYNC_FAILED | SeverityError | Mutagen sync 创建失败: %s | 检查 SSH 连通性，或运行 cloud-claude doctor mount |
| MOUNT_MUTAGEN_TRANSPORT_FAILED | SeverityError | Mutagen ssh 子进程启动失败: %s | 检查本机 ssh 客户端是否可用，或安装 sshpass 作为后备 |
| MOUNT_SSHFS_FAILED | SeverityError | sshfs 挂载失败: %s | 检查 /dev/fuse 是否可用，或运行 cloud-claude doctor ssh |
| MOUNT_SSHFS_DISCONNECTED | SeverityWarn | sshfs 已断开 ≥15 秒，已从 mergerfs 摘除 /workspace-cold | 网络恢复后运行 cloud-claude doctor mount --fix 重新挂载 |
| MOUNT_MERGERFS_FAILED | SeverityError | mergerfs 挂载失败: %s | 检查容器是否启用 SYS_ADMIN + /dev/fuse，或运行 cloud-claude doctor mount |
| MOUNT_AUTO_DOWNGRADED | SeverityWarn | 文件映射已从 %s 降级到 %s，原因: [%s] %s | 运行 cloud-claude doctor mount 查看详细修复建议 |
| MOUNT_FORCE_MODE_FAILED | SeverityFatal | --mount-mode=%s 模式下 %s 层失败: %s | 移除 --mount-mode flag 让自动降级生效，或运行 cloud-claude doctor mount |
| MOUNT_APFS_CASE_INSENSITIVE | SeverityInfo | 检测到 macOS APFS case-insensitive 文件系统，已强制启用 two-way-resolved 同步模式 | 无需操作；如需 case-sensitive 行为请创建 case-sensitive APFS 卷 |
| NET_OAUTH_EXPIRED | SeverityFatal | Claude OAuth 凭证已过期（账号: %s） | 在容器内运行 cloud-claude exec claude login 重新登录 |
| NET_OAUTH_EXPIRING_SOON | SeverityWarn | Claude OAuth 凭证将在 %d 分钟后过期 | 建议尽快 cloud-claude exec claude login |
| NET_OAUTH_NOT_FOUND | SeverityFatal | 容器内未找到 Claude OAuth 凭证文件（账号: %s） | 在容器内运行 cloud-claude exec claude login 完成首次登录 |
</errcode_registry>
</context>

<tasks>

<task type="auto">
  <name>Task 1.1: 落地 errcodes 包核心类型 + Format helper + 注册表完整性单测</name>
  <files>
    internal/cloudclaude/errcodes/codes.go
    internal/cloudclaude/errcodes/mount.go
    internal/cloudclaude/errcodes/net.go
    internal/cloudclaude/errcodes/codes_test.go
  </files>
  <read_first>
    - .planning/phases/31-cli/31-CONTEXT.md（D-02、D-19、D-20、D-21、D-32 — 包结构、命名规则、Format 模板）
    - .planning/phases/31-cli/31-RESEARCH.md §5（包结构、Severity 枚举、Format 模板、TestErrcodesRegistry 模板、15 条文案表）
    - internal/cloudclaude/ssh_doctor.go（v2.0 错误返回风格 — 中文 + fmt.Errorf 包装；errcodes 包必须保留 errors.As/Is 兼容）
  </read_first>
  <action>
    1. 创建包目录 internal/cloudclaude/errcodes/。

    2. internal/cloudclaude/errcodes/codes.go：
       - 包注释：「Phase 31 引入的统一错误码注册表雏形；Phase 34 doctor / explain 直接复用本包的 Registry / Lookup / Format。命名规范 ^[A-Z]+_[A-Z]+_[A-Z0-9]+$。」
       - 类型定义按 <interfaces> 中 errcodes/codes.go 的全部内容（Code、Severity 枚举、Entry struct、MustRegister、Lookup、Registry、Format）。
       - Severity 字符串化：实现 func (s Severity) String() string 返回 INFO/WARN/ERROR/FATAL（仅供日志，调用方不必关心）。
       - 全局：var registry = map[Code]Entry{}；var codeRe = regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+$`)。
       - MustRegister 必须在 duplicate / 命名不合法 / Message 为空 / NextAction 为空时 panic（fail fast，由 init 阶段触发）。
       - Lookup 与 Registry 返回浅拷贝（防止外部修改 internal map）。
       - Format 实现：
         ```go
         func Format(c Code, args ...any) string {
             e, ok := registry[c]
             if !ok {
                 return fmt.Sprintf("[%s] (unknown code)\n  建议: 联系维护者", c)
             }
             msg := e.Message
             if len(args) > 0 {
                 msg = fmt.Sprintf(e.Message, args...)
             }
             return fmt.Sprintf("[%s] %s\n  建议: %s", c, msg, e.NextAction)
         }
         ```
       - 不使用任何第三方包（仅 fmt + regexp + sync）。

    3. internal/cloudclaude/errcodes/mount.go：在 init() 函数中按 <errcode_registry> 表前 12 条（MOUNT_*）顺次 MustRegister。code 与文案逐字符对齐表格内容（含中文标点 / 占位符 %s %d）。

    4. internal/cloudclaude/errcodes/net.go：在 init() 函数中按 <errcode_registry> 表后 3 条（NET_OAUTH_*）顺次 MustRegister。

    5. internal/cloudclaude/errcodes/codes_test.go：
       - TestErrcodesRegistry：遍历 Registry()，断言：
         (a) 无重复 code；
         (b) 每个 code 匹配 ^[A-Z]+_[A-Z]+_[A-Z0-9]+$；
         (c) Message != ""，NextAction != ""；
         (d) utf8.RuneCountInString(NextAction) ≤ 80；
         (e) len(Registry()) >= 15。
       - TestFormat_Render：调用 errcodes.Format(MOUNT_MUTAGEN_VERSION_SKEW, "v0.18.1", "v0.99.99")，断言输出严格等于：
         `[MOUNT_MUTAGEN_VERSION_SKEW] Mutagen 客户端版本 (v0.18.1) 与容器内 agent 版本 (v0.99.99) 不一致，已降级到 sshfs-only\n  建议: 升级容器镜像到 v3.0.0+ 或重装 cloud-claude`。
       - TestFormat_UnknownCode：调用 Format("FAKE_CODE_X")，断言输出包含 "(unknown code)" 且不 panic。
       - TestLookup_Hit / TestLookup_Miss：分别断言已注册 / 未注册 code 的返回值。

    6. 在 codes.go 顶部 export Code 常量（避免下游写裸字符串）：
       ```go
       const (
           MOUNT_MUTAGEN_VERSION_SKEW       Code = "MOUNT_MUTAGEN_VERSION_SKEW"
           MOUNT_MUTAGEN_WHITELIST_REJECT   Code = "MOUNT_MUTAGEN_WHITELIST_REJECT"
           MOUNT_MUTAGEN_SAFETY_GUARD       Code = "MOUNT_MUTAGEN_SAFETY_GUARD"
           MOUNT_MUTAGEN_DAEMON_UNAVAILABLE Code = "MOUNT_MUTAGEN_DAEMON_UNAVAILABLE"
           MOUNT_MUTAGEN_SYNC_FAILED        Code = "MOUNT_MUTAGEN_SYNC_FAILED"
           MOUNT_MUTAGEN_TRANSPORT_FAILED   Code = "MOUNT_MUTAGEN_TRANSPORT_FAILED"
           MOUNT_SSHFS_FAILED               Code = "MOUNT_SSHFS_FAILED"
           MOUNT_SSHFS_DISCONNECTED         Code = "MOUNT_SSHFS_DISCONNECTED"
           MOUNT_MERGERFS_FAILED            Code = "MOUNT_MERGERFS_FAILED"
           MOUNT_AUTO_DOWNGRADED            Code = "MOUNT_AUTO_DOWNGRADED"
           MOUNT_FORCE_MODE_FAILED          Code = "MOUNT_FORCE_MODE_FAILED"
           MOUNT_APFS_CASE_INSENSITIVE      Code = "MOUNT_APFS_CASE_INSENSITIVE"
           NET_OAUTH_EXPIRED                Code = "NET_OAUTH_EXPIRED"
           NET_OAUTH_EXPIRING_SOON          Code = "NET_OAUTH_EXPIRING_SOON"
           NET_OAUTH_NOT_FOUND              Code = "NET_OAUTH_NOT_FOUND"
       )
       ```
       注意：变量名违反 Go lint 风格（含下划线），但 Phase 34 doctor `--explain <code>` 用户输入的就是这些字符串，常量名与 Code 字面一致便于 grep / 维护；在 codes.go 文件顶部加 `//nolint:revive,stylecheck` 关闭 lint。
  </action>
  <acceptance_criteria>
    - 所有 4 个文件存在且 `gofmt -l` 输出为空：
      `gofmt -l internal/cloudclaude/errcodes/`
    - `go build ./internal/cloudclaude/errcodes/` 退出码 0
    - `go test ./internal/cloudclaude/errcodes/ -count=1 -v` 退出码 0；输出包含 `TestErrcodesRegistry`、`TestFormat_Render`、`TestFormat_UnknownCode`、`TestLookup_Hit`、`TestLookup_Miss` 五项 PASS
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/mount.go` 输出 12
    - `grep -c "MustRegister(Entry{" internal/cloudclaude/errcodes/net.go` 输出 3
    - `grep -E "MOUNT_MUTAGEN_VERSION_SKEW.*Code = " internal/cloudclaude/errcodes/codes.go` 命中 1 行
    - `grep -E "^[[:space:]]+建议: " internal/cloudclaude/errcodes/codes.go` 命中至少 1 行（Format 模板）
    - 测试 TestFormat_Render 字面断言通过 `[MOUNT_MUTAGEN_VERSION_SKEW] Mutagen 客户端版本 (v0.18.1)`
  </acceptance_criteria>
  <verify>
    <automated>go test ./internal/cloudclaude/errcodes/ -count=1 -v && [ "$(grep -c 'MustRegister(Entry{' internal/cloudclaude/errcodes/mount.go)" = 12 ] && [ "$(grep -c 'MustRegister(Entry{' internal/cloudclaude/errcodes/net.go)" = 3 ] && gofmt -l internal/cloudclaude/errcodes/ | wc -l | grep -q '^0$'</automated>
  </verify>
  <done>
    errcodes 包提供 6 类对外 API（Code/Severity/Entry/Registry/Lookup/MustRegister/Format），15 条错误码注册完整，单元测试覆盖注册表完整性 + Format 字面输出 + Lookup 命中/未命中三类用例，go test 全 PASS。
  </done>
</task>

<task type="auto">
  <name>Task 1.2: 拉取 + 校验 Mutagen v0.18.1 4 平台二进制（fetch 脚本 + .gitattributes + 占位文件）</name>
  <files>
    scripts/fetch-mutagen-bins.sh
    internal/cloudclaude/mutagen_bin/.gitattributes
    internal/cloudclaude/mutagen_bin/SHA256SUMS
    internal/cloudclaude/mutagen_bin/darwin_amd64/mutagen
    internal/cloudclaude/mutagen_bin/darwin_arm64/mutagen
    internal/cloudclaude/mutagen_bin/linux_amd64/mutagen
    internal/cloudclaude/mutagen_bin/linux_arm64/mutagen
  </files>
  <read_first>
    - .planning/phases/31-cli/31-CONTEXT.md（D-03 — 4 平台 embed、版本锁定 v0.18.1、fetch 脚本职责）
    - .planning/phases/31-cli/31-RESEARCH.md §1.1（实际单文件 ~12MB / 共 ~49MB，cloud-claude 终态 ~80MB；GitHub release tarball 命名）
    - scripts/ 目录下任意现有 shell 脚本（如 deploy/scripts/host-preflight.sh）— 沿用 #!/usr/bin/env bash + set -euo pipefail 风格
  </read_first>
  <action>
    1. scripts/fetch-mutagen-bins.sh（可执行 chmod +x）：

       ```bash
       #!/usr/bin/env bash
       # 拉取 Mutagen v0.18.1 4 平台 release tarball、解包出 mutagen 二进制并 sha256 校验。
       # 用法：scripts/fetch-mutagen-bins.sh [--check-only]
       #   默认：拉取并写入 internal/cloudclaude/mutagen_bin/<plat>/mutagen
       #   --check-only：只校验已存在文件的 sha256，不联网
       set -euo pipefail

       VERSION="v0.18.1"
       BASE_URL="https://github.com/mutagen-io/mutagen/releases/download/${VERSION}"
       OUT_DIR="$(cd "$(dirname "$0")/.." && pwd)/internal/cloudclaude/mutagen_bin"
       SUMS="${OUT_DIR}/SHA256SUMS"

       declare -A PLATFORMS=(
         [darwin_amd64]="mutagen_darwin_amd64_v0.18.1.tar.gz"
         [darwin_arm64]="mutagen_darwin_arm64_v0.18.1.tar.gz"
         [linux_amd64]="mutagen_linux_amd64_v0.18.1.tar.gz"
         [linux_arm64]="mutagen_linux_arm64_v0.18.1.tar.gz"
       )

       mkdir -p "${OUT_DIR}"

       if [[ "${1:-}" == "--check-only" ]]; then
         (cd "${OUT_DIR}" && sha256sum -c SHA256SUMS)
         exit $?
       fi

       tmp="$(mktemp -d)"
       trap 'rm -rf "$tmp"' EXIT

       : > "${SUMS}.new"

       for plat in "${!PLATFORMS[@]}"; do
         tarball="${PLATFORMS[$plat]}"
         echo "=== ${plat}: 拉取 ${tarball}"
         curl -fsSL --retry 3 -o "${tmp}/${tarball}" "${BASE_URL}/${tarball}"
         mkdir -p "${OUT_DIR}/${plat}"
         tar -xzf "${tmp}/${tarball}" -C "${tmp}" mutagen
         install -m 0755 "${tmp}/mutagen" "${OUT_DIR}/${plat}/mutagen"
         (cd "${OUT_DIR}" && sha256sum "${plat}/mutagen") >> "${SUMS}.new"
         rm -f "${tmp}/mutagen"
       done

       mv "${SUMS}.new" "${SUMS}"
       echo "=== 完成。SHA256SUMS:"
       cat "${SUMS}"
       ```

       注意：脚本在没有 sha256sum 的 macOS 上需要 `brew install coreutils` 或退回 `shasum -a 256`；在脚本头加：
       ```bash
       if ! command -v sha256sum >/dev/null 2>&1; then
         sha256sum() { shasum -a 256 "$@"; }
         export -f sha256sum
       fi
       ```

    2. internal/cloudclaude/mutagen_bin/.gitattributes（git 不做 EOL 转换；如果团队启用 git LFS，按需追加）：

       ```
       *  binary
       darwin_amd64/mutagen filter=lfs diff=lfs merge=lfs -text
       darwin_arm64/mutagen filter=lfs diff=lfs merge=lfs -text
       linux_amd64/mutagen filter=lfs diff=lfs merge=lfs -text
       linux_arm64/mutagen filter=lfs diff=lfs merge=lfs -text
       ```

       注：执行者本地若未配置 LFS，执行 `scripts/fetch-mutagen-bins.sh` 拉真二进制后 `git add` 时会自然提示。如团队拒绝 LFS，可改为裸提交（4×12MB ≈ 49MB，可接受），删除 `.gitattributes` 中 lfs 行即可。executor 自行决定，commit message 中说明选择。

    3. 执行脚本拉取 4 平台二进制（在能联网的环境下）：
       ```bash
       chmod +x scripts/fetch-mutagen-bins.sh
       scripts/fetch-mutagen-bins.sh
       ```
       如执行环境无外网，executor 必须在 commit message 与 SUMMARY.md 中明确标记「mutagen_bin/ 二进制由 CI 在 build-images workflow 中拉取」并落空目录 + SHA256SUMS 占位（每行格式 `PENDING-FETCH  <plat>/mutagen`），由后续工作流补齐。**禁止把任意未校验的二进制 commit**。

    4. 在 .gitignore 中**不**排除 `internal/cloudclaude/mutagen_bin/`（这些二进制需进入 git，否则 go:embed 会失败）；同时确认 `scripts/fetch-mutagen-bins.sh` 不在已有 .gitignore 中。
  </action>
  <acceptance_criteria>
    - `test -x scripts/fetch-mutagen-bins.sh` 退出码 0
    - `bash -n scripts/fetch-mutagen-bins.sh` 退出码 0（语法 OK）
    - `grep -F 'VERSION="v0.18.1"' scripts/fetch-mutagen-bins.sh` 命中 1 行
    - `grep -F 'sha256sum' scripts/fetch-mutagen-bins.sh` 命中至少 2 行（一处校验、一处生成 SUMS）
    - 4 个平台目录均存在：`for d in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do test -d internal/cloudclaude/mutagen_bin/$d || exit 1; done`
    - `internal/cloudclaude/mutagen_bin/SHA256SUMS` 存在且至少 4 行（每行 `<sha>  <plat>/mutagen`）；如二进制由 CI 后续补齐，4 行至少含 PENDING-FETCH 占位
    - 联网执行 `scripts/fetch-mutagen-bins.sh --check-only` 退出码 0（前提：二进制已 fetch）；离线 commit 场景跳过此项但 SUMMARY.md 必须说明
    - `internal/cloudclaude/mutagen_bin/.gitattributes` 存在
  </acceptance_criteria>
  <verify>
    <automated>test -x scripts/fetch-mutagen-bins.sh && bash -n scripts/fetch-mutagen-bins.sh && for d in darwin_amd64 darwin_arm64 linux_amd64 linux_arm64; do test -d internal/cloudclaude/mutagen_bin/$d || exit 1; done && test -f internal/cloudclaude/mutagen_bin/SHA256SUMS && grep -F 'VERSION="v0.18.1"' scripts/fetch-mutagen-bins.sh</automated>
  </verify>
  <done>
    fetch 脚本可独立执行拉取 + 校验 Mutagen v0.18.1 4 平台二进制；占位目录与 SHA256SUMS 文件就位，下游 Task 1.3 可直接 `//go:embed mutagen_bin` 编译通过。
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 1.3: go:embed Mutagen 二进制 + ExtractTo 幂等抽取 + IsCaseInsensitiveFS Go probe</name>
  <files>
    internal/cloudclaude/mutagen_bin.go
    internal/cloudclaude/mutagen_bin_test.go
    internal/cloudclaude/envcheck_fs.go
    internal/cloudclaude/envcheck_fs_test.go
  </files>
  <behavior>
    - mutagen_bin_test.go：
      * Test_ExtractMutagenBinary_FreshDir：dst 不存在 → 抽取后文件存在、size > 1MB、权限 0755
      * Test_ExtractMutagenBinary_Idempotent：连续两次调用，第二次无 IO 写入（用 mtime 比对，or 用 mock VersionChecker 注入）
      * Test_ExtractMutagenBinary_OverwriteWrongVersion：dst 已有同名假二进制（写一段 shell `#!/bin/sh\necho v0.99\n`），调用后被覆盖为真二进制（size > 1MB）
      * Test_ExtractMutagenBinary_UnsupportedPlatform：runtime.GOOS = "windows" 或 GOARCH = "386" 时返回非 nil error，error message 含 "MOUNT_MUTAGEN_TRANSPORT_FAILED"
    - envcheck_fs_test.go：
      * Test_IsCaseInsensitiveFS_TempDir：在 t.TempDir() 上跑，断言：linux runner 下返回 false（ext4 case-sensitive）；macOS runner 下结果取决于 APFS 卷类型，**只断言函数不 panic 且返回 bool**（不强断言值，因为 CI runner 类型不确定）
      * Test_IsCaseInsensitiveFS_NoWrite：传入不可写目录 "/proc"（Linux only）/ "/dev/null/x" → 返回 false 不 panic
  </behavior>
  <read_first>
    - .planning/phases/31-cli/31-CONTEXT.md（D-04 — extract 路径 ~/.cloud-claude/bin/mutagen + 幂等校验逻辑；D-09 修订为 Go probe）
    - .planning/phases/31-cli/31-RESEARCH.md §1.1（mutagen daemon 协议、MUTAGEN_DATA_DIRECTORY 隔离）
    - .planning/phases/31-cli/31-RESEARCH.md §7（Go probe 跨平台案例）
    - internal/cloudclaude/config.go（ConfigDir 函数 — extract 父目录沿用 ~/.cloud-claude/）
    - internal/cloudclaude/errcodes/codes.go（本 plan Task 1.1 产出；Format 函数与 MOUNT_MUTAGEN_TRANSPORT_FAILED 常量）
  </read_first>
  <action>
    1. internal/cloudclaude/mutagen_bin.go：

       ```go
       package cloudclaude

       import (
           "embed"
           "fmt"
           "io"
           "os"
           "os/exec"
           "path/filepath"
           "runtime"
           "strings"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       //go:embed mutagen_bin
       var mutagenFS embed.FS

       const MutagenBinaryVersion = "v0.18.1"

       // ExtractMutagenBinary 见 PLAN <interfaces>。
       func ExtractMutagenBinary(dst string) error {
           plat := runtime.GOOS + "_" + runtime.GOARCH
           switch plat {
           case "darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64":
           default:
               return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "unsupported platform "+plat))
           }

           // 幂等：dst 已存在且 version 包含 v0.18.1 → 复用
           if isMutagenAtVersion(dst, MutagenBinaryVersion) {
               return nil
           }

           if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
               return fmt.Errorf("创建 mutagen 二进制目录失败: %w", err)
           }

           src, err := mutagenFS.Open("mutagen_bin/" + plat + "/mutagen")
           if err != nil {
               return fmt.Errorf("%s", errcodes.Format(errcodes.MOUNT_MUTAGEN_TRANSPORT_FAILED, "embed 缺失 "+plat))
           }
           defer src.Close()

           tmp := dst + ".tmp"
           out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
           if err != nil {
               return fmt.Errorf("写入 mutagen 临时文件失败: %w", err)
           }
           if _, err := io.Copy(out, src); err != nil {
               out.Close()
               os.Remove(tmp)
               return fmt.Errorf("复制 mutagen 二进制失败: %w", err)
           }
           if err := out.Close(); err != nil {
               os.Remove(tmp)
               return err
           }
           if err := os.Rename(tmp, dst); err != nil {
               os.Remove(tmp)
               return fmt.Errorf("rename mutagen 二进制失败: %w", err)
           }
           return nil
       }

       func isMutagenAtVersion(path, want string) bool {
           if _, err := os.Stat(path); err != nil {
               return false
           }
           cmd := exec.Command(path, "version")
           out, err := cmd.Output()
           if err != nil {
               return false
           }
           return strings.Contains(string(out), strings.TrimPrefix(want, "v"))
       }
       ```

       注意 import path：本仓库 module `github.com/zanel1u/cloud-cli-proxy`（go.mod 已确认）。

    2. internal/cloudclaude/envcheck_fs.go：

       ```go
       package cloudclaude

       import (
           "os"
           "path/filepath"
           "strings"
       )

       // IsCaseInsensitiveFS 见 PLAN <interfaces>。
       // 实现：在 dir 下创建一个含小写名的临时文件，Stat 其全大写变体；
       //   Stat 成功且 dir/<UPPER> 不等于 dir/<lower>（路径字符串） → 文件系统不区分大小写。
       func IsCaseInsensitiveFS(dir string) bool {
           f, err := os.CreateTemp(dir, "ccprobe-")
           if err != nil {
               return false
           }
           name := f.Name()
           f.Close()
           defer os.Remove(name)

           upper := filepath.Join(filepath.Dir(name), strings.ToUpper(filepath.Base(name)))
           if upper == name {
               return false // 已经全大写或不可比较
           }
           if _, err := os.Stat(upper); err == nil {
               return true
           }
           return false
       }
       ```

    3. internal/cloudclaude/mutagen_bin_test.go：按 <behavior> 实现 4 个用例。
       - Test_ExtractMutagenBinary_UnsupportedPlatform 用 build tag 或在测试函数内 t.Skip 区分平台；可借助 setEnvForTest 改 GOOS — 但 runtime.GOOS 是常量。**简化**：直接构造一个本地 helper `extractFor(plat string, dst string) error`（不暴露），用 plat 入参替代 runtime.GOOS，测试调 helper。public ExtractMutagenBinary 只是 helper(runtime.GOOS+"_"+runtime.GOARCH, dst) 的薄包装。
       - Test_ExtractMutagenBinary_OverwriteWrongVersion：先 `os.WriteFile(dst, []byte("#!/bin/sh\necho fake\n"), 0755)`，调用 ExtractMutagenBinary 后断言 `os.Stat(dst).Size() > 1024*1024`。
       - Test_ExtractMutagenBinary_Idempotent：第一次调用后取 dst 的 mtime，sleep 100ms，第二次调用，断言 mtime 未变。**前提**：测试时 embed 必须真的有内容；如本地未 fetch 二进制，跳过本测试 `t.Skip("no embed binary; run scripts/fetch-mutagen-bins.sh first")`。

    4. internal/cloudclaude/envcheck_fs_test.go：按 <behavior> 实现 2 个用例，用 t.TempDir() 隔离。

    5. **关键导入注意**：本 plan 不在 cloudclaude 包内删/改 mount.go / ssh.go 任何代码（属于 Plan 02 的范围）。本 plan 新增的 mutagen_bin.go / envcheck_fs.go 不被任何现网代码 import，编译通过即可。
  </action>
  <acceptance_criteria>
    - `gofmt -l internal/cloudclaude/mutagen_bin.go internal/cloudclaude/envcheck_fs.go` 输出为空
    - `go vet ./internal/cloudclaude/...` 退出码 0
    - `go build ./internal/cloudclaude/...` 退出码 0（embed 路径必须存在 mutagen_bin/<plat>/mutagen 才能编译；占位文件即可）
    - `go test ./internal/cloudclaude/ -run Test_ExtractMutagenBinary -count=1 -v` 至少 1 个用例 PASS（Idempotent 在无真二进制环境可 SKIP）
    - `go test ./internal/cloudclaude/ -run Test_IsCaseInsensitiveFS -count=1 -v` 全 PASS
    - `grep -F '//go:embed mutagen_bin' internal/cloudclaude/mutagen_bin.go` 命中 1 行
    - `grep -F 'MutagenBinaryVersion = "v0.18.1"' internal/cloudclaude/mutagen_bin.go` 命中 1 行
    - `grep -E 'IsCaseInsensitiveFS\(dir string\) bool' internal/cloudclaude/envcheck_fs.go` 命中 1 行
    - 整仓 `go test ./... -count=1` 退出码 0（不破坏 v2.0 现有测试）
  </acceptance_criteria>
  <verify>
    <automated>go build ./internal/cloudclaude/... && go vet ./internal/cloudclaude/... && go test ./internal/cloudclaude/ -run 'Test_ExtractMutagenBinary|Test_IsCaseInsensitiveFS' -count=1 -v && go test ./... -count=1</automated>
  </verify>
  <done>
    cloudclaude 包新增 ExtractMutagenBinary（go:embed + 幂等抽取）+ IsCaseInsensitiveFS（跨平台 probe）两个对外函数；4 + 2 个单元测试 PASS；不破坏 v2.0 现有 mount_test / ssh_doctor_test 等任何回归测试。
  </done>
</task>

</tasks>

<verification>
本 plan 完成后必须满足以下端到端检查（在仓库根目录执行）：

```bash
# 1. 包结构与 errcodes 注册表
test -d internal/cloudclaude/errcodes
test -f internal/cloudclaude/errcodes/codes.go
test -f internal/cloudclaude/errcodes/mount.go
test -f internal/cloudclaude/errcodes/net.go
test -f internal/cloudclaude/errcodes/codes_test.go
go test ./internal/cloudclaude/errcodes/ -count=1   # 应全 PASS
[ "$(grep -c 'MustRegister(Entry{' internal/cloudclaude/errcodes/mount.go)" = 12 ]
[ "$(grep -c 'MustRegister(Entry{' internal/cloudclaude/errcodes/net.go)" = 3 ]

# 2. mutagen_bin embed 与 fetch 脚本
test -x scripts/fetch-mutagen-bins.sh
bash -n scripts/fetch-mutagen-bins.sh
test -d internal/cloudclaude/mutagen_bin/darwin_amd64
test -d internal/cloudclaude/mutagen_bin/darwin_arm64
test -d internal/cloudclaude/mutagen_bin/linux_amd64
test -d internal/cloudclaude/mutagen_bin/linux_arm64
test -f internal/cloudclaude/mutagen_bin/SHA256SUMS
go build ./internal/cloudclaude/...                  # embed 路径完整即可编译

# 3. envcheck_fs 探测
go test ./internal/cloudclaude/ -run Test_IsCaseInsensitiveFS -count=1 -v

# 4. 整仓回归
go test ./... -count=1                                # 不允许破坏 v2.0 任何现有测试
gofmt -l internal/cloudclaude/                       # 输出必须为空
go vet ./...                                          # 退出码 0
```

如本地无 mutagen 二进制（仅占位）：Task 1.3 的 ExtractMutagenBinary_Idempotent 与 OverwriteWrongVersion 测试可 SKIP，但 SUMMARY.md 必须明确说明并在 CI 环境（`scripts/fetch-mutagen-bins.sh`）补齐。
</verification>

<threat_model>
## Trust Boundaries

| Boundary | 描述 |
|----------|------|
| GitHub release → 本机 | fetch-mutagen-bins.sh 通过 HTTPS 拉取上游二进制；上游被攻陷可能注入恶意 mutagen |
| embed 二进制 → 用户 ~/.cloud-claude/bin/ | ExtractMutagenBinary 写入用户目录；若 embed 内容被篡改，写入的就是恶意二进制 |
| errcodes Format → stderr 输出 | Format 用 fmt.Sprintf 渲染中文模板；Plan 02 / 03 透传给 Format 的 args 是远端命令 stdout/stderr |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-31-01-01 | Tampering | scripts/fetch-mutagen-bins.sh 拉取的 mutagen 二进制 | mitigate | 脚本生成 SHA256SUMS，每次 fetch 重写；运行时 `--check-only` 子命令可在 CI 复核；Task 1.2 acceptance 含 sha256sum -c 校验 |
| T-31-01-02 | Tampering | git 仓库内的 internal/cloudclaude/mutagen_bin/<plat>/mutagen | accept | 仓库受 git 控制（commit hash + LFS hash 不可篡改）；如启用 LFS 则 Object 哈希由 LFS 服务校验；4 个平台二进制不变更（锁定 v0.18.1）确保 commit diff 唯一 |
| T-31-01-03 | Information Disclosure | errcodes.Format 渲染的远端命令 stdout（如 daemon 启动错误） | accept | %s 占位透传，不做日志脱敏；远端错误信息属于运维侧可见数据（无凭证字段）；Plan 02 在调用 Format 前需保证 args 不含 password / token（Plan 02 责任） |
| T-31-01-04 | Denial of Service | embed 二进制 ~49MB 拖慢编译 / 二进制下载体积 | accept | RESEARCH §1.1 已记录权衡，cloud-claude 终态 ~80MB 可接受；v3.1 可分平台 build tag 优化 |
| T-31-01-05 | Spoofing | 用户已有的 brew mutagen daemon 与本进程冲突 | mitigate | RESEARCH §1.1 + Plan 02 ExtractMutagenBinary 抽取到独立目录 ~/.cloud-claude/bin/，且 MUTAGEN_DATA_DIRECTORY 强制 ~/.cloud-claude/mutagen/（Plan 02 落实），与 brew 默认路径完全隔离 |
| T-31-01-06 | Elevation of Privilege | ExtractMutagenBinary 写 0755 二进制到家目录 | accept | 写入位置为用户家目录子路径（沿用 ConfigDir 0700 父目录约束），不涉及 sudo 或系统目录；Task 1.3 mkdir 0700 防御误写到 world-writable 目录 |

</threat_model>

<success_criteria>
- errcodes 包完整：6 类 API（Code/Severity/Entry/Registry/Lookup/Format）+ 15 条注册码 + 5 项单元测试 PASS
- Mutagen embed 基础设施完整：fetch 脚本 + .gitattributes + 4 平台占位 + SHA256SUMS + ExtractMutagenBinary 幂等抽取 + 4 项单测
- IsCaseInsensitiveFS 跨平台 probe 函数 + 2 项单测，不依赖 macOS diskutil
- 整仓 `go build ./...` + `go test ./... -count=1` + `gofmt -l` + `go vet ./...` 全部通过
- 无 v2.0 现网代码改动（mount.go / ssh.go / entry.go / main.go 均未触碰）
</success_criteria>

<output>
完成后创建 `.planning/phases/31-cli/plans/01-errcodes-mutagen-embed/SUMMARY.md`，列明：
- errcodes 包导出的 API 列表与 15 条 Code 常量名
- ExtractMutagenBinary / IsCaseInsensitiveFS 签名与调用入口
- 4 平台二进制 fetch 是否在本次执行已落地（如未落地，标记「CI 环境补齐」）
- 已知限制：cloud-claude 二进制体积从 ~30MB 涨到 ~80MB（RESEARCH §1.1 已知）
- 与 Plan 02 的接口契约：Plan 02 的 mount_mutagen.go 直接 import errcodes 包 + 调 ExtractMutagenBinary(filepath.Join(home, ".cloud-claude/bin/mutagen"))
</output>
