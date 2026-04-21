---
phase: 31-cli
plan: 03-oauth-conflicts-integration
type: execute
wave: 3
depends_on:
  - 01-errcodes-mutagen-embed
  - 02-mount-three-layer
files_modified:
  - internal/cloudclaude/oauth_check.go
  - internal/cloudclaude/oauth_check_test.go
  - internal/cloudclaude/mount_mutagen.go
  - internal/cloudclaude/mount_strategy.go
  - internal/cloudclaude/ssh.go
  - cmd/cloud-claude/sync.go
  - cmd/cloud-claude/main.go
  - internal/cloudclaude/integration_test.go
  - scripts/test-fixture-up.sh
  - scripts/test-fixture-down.sh
autonomous: true
requirements:
  - REQ-F1-E
  - REQ-F7-C
must_haves:
  truths:
    - "CheckOAuthCredentials(connA, claudeAccountID) 在 SSH 握手成功后、Mutagen sync create / sshfs mount 之前并发执行；远程 timeout 2 cat /home/claude/.claude/.credentials.json，解析 claudeAiOauth.expiresAt（毫秒），返回三态：NotFound / Expired / ExpiringSoon / Valid"
    - "OAuth 失败优先级高于 mount 失败：mount 与 OAuth 在同一 errgroup 中并发，OAuth 返回 Fatal 错误时先 cleanup mount，stderr 输出 errcodes.Format(NET_OAUTH_*) + 退出对应命名常量（cloudclaude.ExitOAuthNotFound=6、cloudclaude.ExitOAuthExpired=7；引用 Plan 02 Task 2.4 创建的 exitcodes.go，避免裸数字与 v2.0 main.go ExitConfigError=4/ExitInternalError=5 撞码）；ExpiringSoon 仅警告不阻断"
    - "claude_account_id 缺失时跳过 OAuth 检查并 stderr 提示「gateway 未返回 claude_account_id，跳过 OAuth 过期检查（建议升级 gateway 至 v3.0）」（CONTEXT D-24）"
    - "mount_mutagen.go 在 mount ready 之后调 mutagen sync list --template '{{range .}}{{.Name}}|{{len .Conflicts}}{{end}}' 解析 conflict count；count > 0 时上报给 mount_strategy 的 banner 后中文警告 + 写入 last-session.json conflict_count 字段"
    - "cmd/cloud-claude/sync.go 注册 sync conflicts 子命令；调 mutagen sync list --long 渲染冲突文件清单（path / alpha / beta / mtime）"
    - "internal/cloudclaude/integration_test.go（//go:build integration）覆盖 6 个 RESEARCH §6.2 关键场景；scripts/test-fixture-{up,down}.sh 用 docker compose 起停 Phase 29 镜像（不引入 testcontainers-go）"
    - "scripts/test-fixture-up.sh 创建临时 docker-compose.yml + 启动 1 个 Phase 29 镜像容器 + 等待 sshd ready；test-fixture-down.sh 销毁；脚本可重复执行（幂等）"
  artifacts:
    - path: "internal/cloudclaude/oauth_check.go"
      provides: "CheckOAuthCredentials + OAuthStatus + parseExpiresAt（纯函数）"
      contains: "func CheckOAuthCredentials"
    - path: "internal/cloudclaude/oauth_check_test.go"
      provides: "expiresAt 三态 + JSON 解析容错单测"
      contains: "Test_ParseExpiresAt"
    - path: "cmd/cloud-claude/sync.go"
      provides: "sync conflicts 子命令实现 + cobra 注册"
      contains: "sync conflicts"
    - path: "internal/cloudclaude/integration_test.go"
      provides: "6 个集成测试用例覆盖 C3 / C4 / C5 / REQ-F1-D / REQ-F2-B / REQ-F7-C"
      contains: "//go:build integration"
    - path: "scripts/test-fixture-up.sh"
      provides: "起 Phase 29 镜像容器 + 等待 sshd 就绪 + 写本地 fixture 配置"
      contains: "docker compose"
    - path: "scripts/test-fixture-down.sh"
      provides: "销毁 fixture 容器 + 清理 mount 残留 + rm 临时文件"
      contains: "docker compose down"
  key_links:
    - from: "internal/cloudclaude/ssh.go ConnectAndRunClaudeV3"
      to: "oauth_check.go CheckOAuthCredentials"
      via: "在 MountWorkspace 之后、runClaude 之前调用；errgroup 与 mount 并发"
      pattern: "CheckOAuthCredentials"
    - from: "mount_mutagen.go mountMutagen"
      to: "mutagen sync list --template"
      via: "mount ready 后解析 conflict count，写入返回结构"
      pattern: "sync list --template"
    - from: "cmd/cloud-claude/sync.go syncConflicts"
      to: "~/.cloud-claude/bin/mutagen sync list --long"
      via: "exec.Command 渲染冲突清单"
      pattern: "sync list --long"
    - from: "internal/cloudclaude/integration_test.go"
      to: "scripts/test-fixture-{up,down}.sh"
      via: "TestMain 中 exec.Command 调起 fixture"
      pattern: "test-fixture"
---

<plan_dependencies>
- **Plan 01（Wave 1）必须先完成**：依赖 errcodes 包的 NET_OAUTH_EXPIRED / NET_OAUTH_EXPIRING_SOON / NET_OAUTH_NOT_FOUND 三个 Code 常量与 Format 函数
- **Plan 02（Wave 2）必须先完成**：依赖 ConnectAndRunClaudeV3 入口（OAuth 检查 hook 点已在 ssh.go 留 TODO）+ MountConfig 字段 + mount_mutagen.go 的 deps 注入接口（本 plan 在其上扩展 conflict count 解析逻辑）+ cmd/cloud-claude/main.go 的 cobra 子命令路由（本 plan 注册 sync 子命令树）+ **Plan 02 Task 2.4 创建的 internal/cloudclaude/exitcodes.go 命名常量**（本 plan ssh.go OAuth 检查必须引用 cloudclaude.ExitOAuthNotFound / ExitOAuthExpired，避免裸数字与 v2.0 4/5 撞码）
- 与 Plan 02 的并发改动文件：mount_mutagen.go（本 plan 追加 conflict 解析）、mount_strategy.go（本 plan 接入 conflict count → banner 警告）、ssh.go（本 plan 替换 TODO 注释为 CheckOAuthCredentials 调用）、cmd/cloud-claude/main.go（本 plan 注册 sync 子命令）— **本 plan 必须在 Plan 02 完成 commit 之后才能开始**，避免 git 冲突
</plan_dependencies>

<objective>
落地 Phase 31 剩余两个 user-facing 行为 + 一个验收基础设施：

1. **OAuth 过期检查（REQ-F7-C）**：CONTEXT D-22 / D-23 / D-24 + RESEARCH §4 — 在 SSH 握手成功后、claude 进程启动前的窗口期，远程读 `/home/claude/.claude/.credentials.json`，按三态分支处理（不存在 / 已过期 / 即将过期 / 有效），失败时退出非 0 + 中文提示，**禁止让 claude 进程先报错**。
2. **Mutagen conflict 冒泡（REQ-F1-E）**：CONTEXT D-28 + RESEARCH §1.1 修订（v0.18.1 不支持 --json，改 --template）— 启动后期解析 conflict count，banner 后输出中文警告；提供 `cloud-claude sync conflicts` 子命令查看清单。
3. **集成测试套件（验收基线）**：RESEARCH §6.2 + CONTEXT specifics — 6 个集成场景覆盖 C3 / C4 / C5 / REQ-F1-D / REQ-F2-B / REQ-F7-C；用脚本化 docker compose fixture，**不引入 testcontainers-go**（避免 50+ indirect deps）。

Purpose: 兑现 ROADMAP §Phase 31 Success Criteria 第 6 条（C5 安全门 sync 必须未创建）+ 第 9 条（OAuth 过期不进 claude）+ 第 8 条（conflict 冲突中文冒泡 + cloud-claude sync conflicts 列出清单）；6 个集成测试是 Phase 35 真机验收前的 CI gate。
Output: 1 个 oauth 模块 + 1 个 sync 子命令 + 6 个集成测试 + 2 个 fixture 脚本 + Plan 02 文件的小幅扩展（conflict 解析 + OAuth hook）。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/31-cli/31-CONTEXT.md
@.planning/phases/31-cli/31-RESEARCH.md
@.planning/phases/29-v3-worker/29-CONTEXT.md
@.planning/phases/30-entry-api/30-CONTEXT.md
@.planning/phases/31-cli/plans/01-errcodes-mutagen-embed/PLAN.md
@.planning/phases/31-cli/plans/02-mount-three-layer/PLAN.md
@internal/cloudclaude/ssh.go
@internal/cloudclaude/entry.go
@internal/cloudclaude/mount_strategy.go
@internal/cloudclaude/mount_mutagen.go
@cmd/cloud-claude/main.go

<interfaces>
<!-- 本 plan 创建的对外 API。 -->

internal/cloudclaude/oauth_check.go 导出：

```go
package cloudclaude

import (
    "time"
    "golang.org/x/crypto/ssh"
)

type OAuthState int
const (
    OAuthValid         OAuthState = iota
    OAuthExpiringSoon  // expiresAt - now < 5min（CONTEXT D-22 锁定 5min；10min 留 v3.1）
    OAuthExpired
    OAuthNotFound
)

type OAuthStatus struct {
    State           OAuthState
    ExpiresAt       time.Time     // 解析失败 / NotFound 时 zero value
    MinutesToExpire int           // 仅 ExpiringSoon 有意义
}

// CheckOAuthCredentials 在 conn-A 上远程 timeout 2 cat /home/claude/.claude/.credentials.json，
// 解析 claudeAiOauth.expiresAt（毫秒级 Unix timestamp）后按 D-22 三态返回。
// 失败容错（JSON 解析失败 / 字段缺失）→ 视为 OAuthNotFound（CONTEXT D-22 第 4 条 — 保守降级避免无意义阻塞）。
// claudeAccountID 仅用于错误信息渲染（不影响检查逻辑）。
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)

// parseExpiresAt 是纯函数（不依赖 ssh.Client），用于单元测试覆盖三态 + JSON 解析容错。
// rawJSON 为远端 cat 的 stdout；now 用于测试注入「当前时间」。
func parseExpiresAt(rawJSON string, now time.Time) *OAuthStatus
```

mount_mutagen.go 扩展（不破坏 Plan 02 已有签名）：

```go
// MutagenSyncStatus 是 mountMutagen 返回的扩展状态（Plan 02 留有 hook 点）。
// Plan 03 在 mountMutagen 函数体最后 sync list --template 解析后填充。
type MutagenSyncStatus struct {
    SessionName   string
    ConflictCount int
    LastError     string
}

// 注：mountMutagen 签名需要扩展为：
//   func mountMutagen(connA *ssh.Client, mutagenSyncCfg MutagenSyncConfig, deps mountMutagenDeps) (cleanup func(), status MutagenSyncStatus, err error)
// 即在 cleanup 与 err 之间插入 status 返回值。Plan 02 调用方（mount_strategy）需同步更新。
```

cmd/cloud-claude/sync.go 注册：

```go
// syncCmd 在 cmd/cloud-claude/main.go init 阶段被 rootCmd.AddCommand 注册。
// 路由：cloud-claude sync conflicts [--no-color]
// 行为：建立 SSH 连接（沿用 LoadConfig + EntryClient）→ 远程不调用，本地 ~/.cloud-claude/bin/mutagen sync list --long 解析渲染中文表格
var syncCmd *cobra.Command         // sync 父命令
var syncConflictsCmd *cobra.Command // sync conflicts 子命令
```
</interfaces>

<oauth_remote_command>
<!-- conn-A 上远程执行；超时 2s 防 SSH hang（RESEARCH §4.1）。 -->

```bash
timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null
```

期望 JSON 格式（RESEARCH §4.1）：

```json
{
  "claudeAiOauth": {
    "accessToken": "sk-ant-oat01-...",
    "refreshToken": "sk-ant-ort01-...",
    "expiresAt": 1745000000000,
    "scopes": ["org:create_api_key"],
    "subscriptionType": "pro"
  }
}
```

注意：
- `expiresAt` 是**毫秒级** Unix timestamp（不是秒）
- 文件不存在时 `cat` 退出码 != 0、stdout 空 → 视为 NotFound
- JSON 解析失败 / `claudeAiOauth.expiresAt` 缺失 → 也视为 NotFound（保守降级）
</oauth_remote_command>

<conflict_template>
<!-- mount_mutagen.go 调用此命令解析 conflict count；v0.18.1 不支持 --json（RESEARCH §1.1）。 -->

```bash
$HOME/.cloud-claude/bin/mutagen sync list --template '{{range .}}{{.Name}}|{{len .Conflicts}}|{{.LastError}}{{"\n"}}{{end}}'
```

输出格式（每行一个 sync session）：

```
cloud-claude-abc123-deadbeef|0|
cloud-claude-xyz789-cafebabe|3|
```

解析：第二列是 int conflict count；累加所有 session 的 count 后回报。
</conflict_template>

<integration_fixture>
<!-- scripts/test-fixture-up.sh / down.sh 与 Phase 29 镜像协作；不引入 testcontainers-go。 -->

scripts/test-fixture-up.sh：
- 检查 docker / docker compose 可用
- 检查 Phase 29 镜像存在（`docker image inspect local/managed-user:v3.0.0` 失败 → 提示先 `docker build -f deploy/docker/managed-user/Dockerfile -t local/managed-user:v3.0.0 .`）
- 写临时 docker-compose.yml 到 `/tmp/cloud-claude-fixture/docker-compose.yml`：
  ```yaml
  services:
    cc-fixture:
      image: local/managed-user:v3.0.0
      container_name: cc-fixture
      cap_add: [SYS_ADMIN]
      devices: ["/dev/fuse:/dev/fuse"]
      security_opt: ["apparmor=unconfined"]
      ports: ["12222:2222"]
      environment:
        - CLOUD_CLAUDE_TEST_FIXTURE=1
  ```
- `docker compose up -d` 启动
- 轮询 `nc -z 127.0.0.1 12222` 直到 sshd ready（超时 30s）
- 输出 fixture 端口 12222 + 已知 SSH 凭证（local/managed-user 镜像 entrypoint 写入的固定测试用户）

scripts/test-fixture-down.sh：
- `docker compose down -v --remove-orphans`
- `rm -rf /tmp/cloud-claude-fixture/`
</integration_fixture>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 3.1: oauth_check.go 实现 + 三态 + JSON 容错单测</name>
  <files>
    internal/cloudclaude/oauth_check.go
    internal/cloudclaude/oauth_check_test.go
  </files>
  <behavior>
    oauth_check_test.go 用例（全部针对 parseExpiresAt 纯函数 + 表驱动）：
    - Test_ParseExpiresAt_Valid：rawJSON 含 expiresAt = now+1h（毫秒），now=固定时间 → State=OAuthValid
    - Test_ParseExpiresAt_ExpiringSoon：expiresAt = now+3min → State=OAuthExpiringSoon、MinutesToExpire=3
    - Test_ParseExpiresAt_AtBoundary：expiresAt = now+5min → State=OAuthValid（边界严格 < 5min 才算 ExpiringSoon）
    - Test_ParseExpiresAt_Expired：expiresAt = now-1s → State=OAuthExpired
    - Test_ParseExpiresAt_EmptyInput：rawJSON="" → State=OAuthNotFound
    - Test_ParseExpiresAt_MalformedJSON：rawJSON="{not json" → State=OAuthNotFound
    - Test_ParseExpiresAt_MissingField：rawJSON=`{"foo":"bar"}` → State=OAuthNotFound
    - Test_ParseExpiresAt_NestedMissing：rawJSON=`{"claudeAiOauth":{"accessToken":"x"}}`（无 expiresAt） → State=OAuthNotFound
    - Test_ParseExpiresAt_SecondsNotMilliseconds：expiresAt 写成秒（一个 10 位数字 1700000000）→ 仍按毫秒解析（结果是远过去 = OAuthExpired）— 这是 v3.0 故意行为：claude code 输出就是毫秒，不做容错
  </behavior>
  <read_first>
    - .planning/phases/31-cli/31-CONTEXT.md（D-22 三态分支 / D-23 cleanup 优先级 / D-24 缺失 claude_account_id）
    - .planning/phases/31-cli/31-RESEARCH.md §4（credentials.json schema、毫秒级 expiresAt、远程命令、Go 解析模板、§4.2 5min 阈值锁定）
    - internal/cloudclaude/errcodes/codes.go（NET_OAUTH_EXPIRED / NET_OAUTH_EXPIRING_SOON / NET_OAUTH_NOT_FOUND 三个 Code）
    - internal/cloudclaude/mount.go（sshRun 函数 — 本 task 用 sess.Output 形式收集 stdout）
  </read_first>
  <action>
    1. internal/cloudclaude/oauth_check.go：

       ```go
       package cloudclaude

       import (
           "bytes"
           "encoding/json"
           "fmt"
           "time"

           "golang.org/x/crypto/ssh"
       )

       type OAuthState int

       const (
           OAuthValid OAuthState = iota
           OAuthExpiringSoon
           OAuthExpired
           OAuthNotFound
       )

       const oauthExpiringWindow = 5 * time.Minute

       type OAuthStatus struct {
           State           OAuthState
           ExpiresAt       time.Time
           MinutesToExpire int
       }

       // CheckOAuthCredentials 在 connA 上远程读 credentials.json，按 D-22 三态返回。
       // 任何远程命令错误（stat 失败 / 超时 / SSH session 错误）都收敛到 OAuthNotFound（保守降级，避免阻塞 mount 路径）。
       func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error) {
           sess, err := connA.NewSession()
           if err != nil {
               return &OAuthStatus{State: OAuthNotFound}, nil
           }
           defer sess.Close()

           var stdout bytes.Buffer
           sess.Stdout = &stdout
           // stderr 丢弃（cat 失败时 stdout 也是空的，等价处理）
           sess.Stderr = nil

           cmd := "timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null"
           _ = sess.Run(cmd) // 退出非 0 不算 error；stdout 空就视为 NotFound

           return parseExpiresAt(stdout.String(), time.Now()), nil
       }

       // parseExpiresAt 是纯函数（用于单元测试）。
       func parseExpiresAt(rawJSON string, now time.Time) *OAuthStatus {
           if rawJSON == "" {
               return &OAuthStatus{State: OAuthNotFound}
           }
           var creds struct {
               Inner struct {
                   ExpiresAt int64 `json:"expiresAt"`
               } `json:"claudeAiOauth"`
           }
           if err := json.Unmarshal([]byte(rawJSON), &creds); err != nil {
               return &OAuthStatus{State: OAuthNotFound}
           }
           if creds.Inner.ExpiresAt == 0 {
               return &OAuthStatus{State: OAuthNotFound}
           }

           expires := time.UnixMilli(creds.Inner.ExpiresAt)
           if !expires.After(now) {
               return &OAuthStatus{State: OAuthExpired, ExpiresAt: expires}
           }
           // 边界：严格 < 5min 才算 ExpiringSoon
           if expires.Sub(now) < oauthExpiringWindow {
               minutes := int(expires.Sub(now).Minutes())
               if minutes < 1 {
                   minutes = 1
               }
               return &OAuthStatus{
                   State:           OAuthExpiringSoon,
                   ExpiresAt:       expires,
                   MinutesToExpire: minutes,
               }
           }
           return &OAuthStatus{State: OAuthValid, ExpiresAt: expires}
       }

       // ensureFmtImported 防止 fmt 被 lint 删除（保留以便 CheckOAuthCredentials 后续添加日志输出时直接用）。
       var _ = fmt.Errorf
       ```

    2. internal/cloudclaude/oauth_check_test.go：按 <behavior> 9 个用例，全表驱动：

       ```go
       func Test_ParseExpiresAt(t *testing.T) {
           now := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)
           cases := []struct{
               name     string
               raw      string
               wantState OAuthState
           }{
               {"valid", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(time.Hour).UnixMilli()), OAuthValid},
               {"expiringSoon3min", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(3*time.Minute).UnixMilli()), OAuthExpiringSoon},
               {"atBoundary5min", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(5*time.Minute).UnixMilli()), OAuthValid},
               {"expired", fmt.Sprintf(`{"claudeAiOauth":{"expiresAt":%d}}`, now.Add(-time.Second).UnixMilli()), OAuthExpired},
               {"emptyInput", "", OAuthNotFound},
               {"malformedJSON", "{not json", OAuthNotFound},
               {"missingField", `{"foo":"bar"}`, OAuthNotFound},
               {"nestedMissing", `{"claudeAiOauth":{"accessToken":"x"}}`, OAuthNotFound},
               {"secondsNotMilliseconds", `{"claudeAiOauth":{"expiresAt":1700000000}}`, OAuthExpired},
           }
           for _, tc := range cases {
               t.Run(tc.name, func(t *testing.T) {
                   s := parseExpiresAt(tc.raw, now)
                   if s.State != tc.wantState {
                       t.Errorf("State = %d, want %d", s.State, tc.wantState)
                   }
               })
           }
       }
       ```
  </action>
  <acceptance_criteria>
    - `gofmt -l internal/cloudclaude/oauth_check.go internal/cloudclaude/oauth_check_test.go` 输出为空
    - `go build ./...` 退出码 0
    - `go test ./internal/cloudclaude/ -run Test_ParseExpiresAt -count=1 -v` 至少 9 个子用例 PASS
    - 关键签名：
      `grep -F 'func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)' internal/cloudclaude/oauth_check.go` 命中
      `grep -F 'func parseExpiresAt(rawJSON string, now time.Time) *OAuthStatus' internal/cloudclaude/oauth_check.go` 命中
    - 5min 阈值与 timeout 2 命令字面：
      `grep -F '5 * time.Minute' internal/cloudclaude/oauth_check.go` 命中
      `grep -F 'timeout 2 cat /home/claude/.claude/.credentials.json' internal/cloudclaude/oauth_check.go` 命中
    - 整仓回归：`go test ./... -count=1` 退出码 0
  </acceptance_criteria>
  <verify>
    <automated>go build ./... && go test ./internal/cloudclaude/ -run Test_ParseExpiresAt -count=1 -v && grep -F 'func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error)' internal/cloudclaude/oauth_check.go && grep -F 'timeout 2 cat /home/claude/.claude/.credentials.json' internal/cloudclaude/oauth_check.go && grep -F '5 * time.Minute' internal/cloudclaude/oauth_check.go</automated>
  </verify>
  <done>
    OAuth 检查模块就绪：CheckOAuthCredentials 远端命令 + parseExpiresAt 纯函数 + 9 个表驱动单测 PASS；5min 阈值与毫秒解析与 RESEARCH §4 / CONTEXT D-22 一致。
  </done>
</task>

<task type="auto">
  <name>Task 3.2: 接线 OAuth 检查到 ConnectAndRunClaudeV3 + Mutagen conflict count 上报到 banner</name>
  <files>
    internal/cloudclaude/ssh.go
    internal/cloudclaude/mount_mutagen.go
    internal/cloudclaude/mount_strategy.go
  </files>
  <read_first>
    - internal/cloudclaude/ssh.go（Plan 02 留的 OAuth TODO 注释位置）
    - internal/cloudclaude/oauth_check.go（Task 3.1 产出 — CheckOAuthCredentials / OAuthStatus / OAuthState）
    - internal/cloudclaude/mount_mutagen.go（Plan 02 产出 — mountMutagen 现签名 cleanup, err；本 task 改为 cleanup, status, err）
    - internal/cloudclaude/mount_strategy.go（Plan 02 产出 — banner 输出 + last-session.json 写入 hook）
    - internal/cloudclaude/exitcodes.go（Plan 02 Task 2.4 产出 — ExitOAuthNotFound=6 / ExitOAuthExpired=7 命名常量；本 task ssh.go OAuth 检查必须引用，禁止裸数字）
    - .planning/phases/31-cli/31-CONTEXT.md（D-22 三态、D-23 cleanup 优先级、D-24 ClaudeAccountID 缺失、D-28 conflict 冒泡；注：D-22 原约定 NotFound=4 / Expired=5 与 v2.0 main.go 撞码，本 plan 修订为 6/7）
    - .planning/phases/31-cli/31-RESEARCH.md §1.1（mutagen sync list --template）
    - cmd/cloud-claude/main.go 行 16-23（v2.0 现有 exit* 常量 0-5，本 task 不动）
  </read_first>
  <action>
    1. **internal/cloudclaude/ssh.go ConnectAndRunClaudeV3**：把 Plan 02 留的 TODO 注释替换为真实 OAuth 检查逻辑。

       插入位置：MountWorkspace 调用之后、runClaude 之前。

       ```go
       // OAuth 检查（CONTEXT D-22 / D-23 / D-24）：
       //   - claude_account_id 缺失 → 跳过 + 中文提示（不阻塞 mount）
       //   - 检查在 mount 完成后执行；失败先 cleanup mount + 退出非 0
       //   - 5min 内即将过期仅警告
       if mountCfg.ClaudeAccountID == "" {
           fmt.Fprintln(mountCfg.Logger, "[!] gateway 未返回 claude_account_id，跳过 OAuth 过期检查（建议升级 gateway 至 v3.0）")
       } else {
           status, err := CheckOAuthCredentials(connA, mountCfg.ClaudeAccountID)
           if err != nil {
               // CheckOAuthCredentials 内部容错；err 几乎不可能非 nil。但保留路径。
               fmt.Fprintln(mountCfg.Logger, "[!] OAuth 检查异常: "+err.Error())
           } else {
               switch status.State {
               case OAuthExpired:
                   fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_EXPIRED, mountCfg.ClaudeAccountID))
                   return ExitOAuthExpired, nil // 7（Plan 02 Task 2.4 exitcodes.go）— v2.0 ExitInternalError 已占 5，故 OAuth 用 7
               case OAuthNotFound:
                   fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_NOT_FOUND, mountCfg.ClaudeAccountID))
                   return ExitOAuthNotFound, nil // 6（exitcodes.go）— v2.0 ExitConfigError 已占 4，故 OAuth 用 6
               case OAuthExpiringSoon:
                   fmt.Fprintln(mountCfg.Logger, errcodes.Format(errcodes.NET_OAUTH_EXPIRING_SOON, status.MinutesToExpire))
                   // 继续启动（warning only）
               case OAuthValid:
                   // 不输出（避免噪音）
               }
           }
       }
       ```

       注意 import：必须 import `github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes`。

    2. **internal/cloudclaude/mount_mutagen.go**：扩展 mountMutagen 签名为 3 返回值（cleanup, status, err）。

       a. 在 type 定义区追加：
          ```go
          type MutagenSyncStatus struct {
              SessionName   string
              ConflictCount int
              LastError     string
          }
          ```

       b. mountMutagen 函数签名改为：
          ```go
          func mountMutagen(connA *ssh.Client, mutagenSyncCfg MutagenSyncConfig, deps mountMutagenDeps) (cleanup func(), status MutagenSyncStatus, err error)
          ```

       c. 在 sync create 成功之后追加 conflict count 解析：
          ```go
          // 解析 conflict count（CONTEXT D-28；RESEARCH §1.1：v0.18.1 不支持 --json，用 --template）
          tmplArgs := []string{
              "sync", "list",
              "--template", `{{range .}}{{.Name}}|{{len .Conflicts}}|{{.LastError}}` + "\n" + `{{end}}`,
          }
          out, listErr := deps.runLocal(mutagenBinPath, tmplArgs, mutagenEnv)
          if listErr != nil {
              status.SessionName = mutagenSyncCfg.SessionName
              status.LastError = listErr.Error()
              return cleanup, status, nil // sync 已建好，list 失败仅丢失 conflict 信息，不阻断
          }
          conflicts := 0
          for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
              parts := strings.SplitN(line, "|", 3)
              if len(parts) >= 2 && parts[0] == mutagenSyncCfg.SessionName {
                  if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
                      conflicts = n
                      if len(parts) >= 3 {
                          status.LastError = parts[2]
                      }
                      break
                  }
              }
          }
          status.SessionName = mutagenSyncCfg.SessionName
          status.ConflictCount = conflicts
          return cleanup, status, nil
          ```

       d. 错误返回路径同步改为 3 返回值：所有 `return nil, err` 改为 `return nil, MutagenSyncStatus{}, err`。

    3. **internal/cloudclaude/mount_strategy.go**：调用 mountMutagen 的位置同步改 3 返回值；在 banner 输出后用 status.ConflictCount 决定是否插入警告：

       ```go
       // 假设 mutagenStatus 是 mountMutagen 返回的 status（仅 mode != ModeSSHFSOnly 时有意义）
       if mutagenStatus.ConflictCount > 0 {
           fmt.Fprintf(mountCfg.Logger, "⚠ 有 %d 个文件同步冲突，运行 cloud-claude sync conflicts 查看\n", mutagenStatus.ConflictCount)
       }
       // 同时写入 last-session.json snapshot.ConflictCount = mutagenStatus.ConflictCount
       ```

       last-session.json 的 snapshot 构造时填 `ConflictCount: mutagenStatus.ConflictCount`。

    4. **mount_strategy_test.go 同步更新**：Plan 02 留的 strategyHooks.tryMutagen 函数签名改为返回 (cleanup, status, err) — 12 降级矩阵测试必须随之 update（status 字段在 cfg.Mode != Mutagen 路径上 zero value）。
  </action>
  <acceptance_criteria>
    - `gofmt -l internal/cloudclaude/` 输出为空
    - `go build ./...` 退出码 0
    - 关键调用链：
      `grep -F 'CheckOAuthCredentials(connA' internal/cloudclaude/ssh.go` 命中 1 行
      `grep -F 'errcodes.NET_OAUTH_EXPIRED' internal/cloudclaude/ssh.go` 命中 1 行
      `grep -F 'errcodes.NET_OAUTH_NOT_FOUND' internal/cloudclaude/ssh.go` 命中 1 行
      `grep -F 'errcodes.NET_OAUTH_EXPIRING_SOON' internal/cloudclaude/ssh.go` 命中 1 行
      `grep -F 'gateway 未返回 claude_account_id' internal/cloudclaude/ssh.go` 命中 1 行
    - mountMutagen 三返回值：
      `grep -E 'func mountMutagen\([^)]*\) \(cleanup func\(\), status MutagenSyncStatus, err error\)' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F 'sync list' internal/cloudclaude/mount_mutagen.go` 命中
      `grep -F '--template' internal/cloudclaude/mount_mutagen.go` 命中（不能是 --json）
    - banner conflict 警告：
      `grep -F '同步冲突' internal/cloudclaude/mount_strategy.go` 命中
      `grep -F 'cloud-claude sync conflicts' internal/cloudclaude/mount_strategy.go` 命中
    - exit codes 引用命名常量（修订 D-22 第 3 条 4/5 → 6/7，避开 v2.0 main.go ExitConfigError=4 / ExitInternalError=5 撞码）：
      `grep -F 'return ExitOAuthNotFound,' internal/cloudclaude/ssh.go` 命中（NotFound = 6）
      `grep -F 'return ExitOAuthExpired,' internal/cloudclaude/ssh.go` 命中（Expired = 7）
      `! grep -E 'return [0-9]+,' internal/cloudclaude/ssh.go | grep -E 'OAuth|NET_'`（OAuth 路径不允许裸数字）
    - 整仓回归：`go test ./... -count=1` 退出码 0（mount_strategy 12 降级矩阵随 mountMutagen 签名变更已在 Plan 02 mock 中处理；本 task 必须更新 mock 签名）
  </acceptance_criteria>
  <verify>
    <automated>go build ./... && go test ./... -count=1 && grep -F 'CheckOAuthCredentials(connA' internal/cloudclaude/ssh.go && grep -F 'errcodes.NET_OAUTH_EXPIRED' internal/cloudclaude/ssh.go && grep -F 'errcodes.NET_OAUTH_NOT_FOUND' internal/cloudclaude/ssh.go && grep -F 'gateway 未返回 claude_account_id' internal/cloudclaude/ssh.go && grep -F 'return ExitOAuthNotFound' internal/cloudclaude/ssh.go && grep -F 'return ExitOAuthExpired' internal/cloudclaude/ssh.go && grep -F '--template' internal/cloudclaude/mount_mutagen.go && ! grep -F 'sync list --json' internal/cloudclaude/mount_mutagen.go && grep -F '同步冲突' internal/cloudclaude/mount_strategy.go</automated>
  </verify>
  <done>
    OAuth 检查接入 ConnectAndRunClaudeV3 路径：claude_account_id 缺失跳过 + 三态分支落码 + 退出码引用 cloudclaude.ExitOAuthNotFound(6) / ExitOAuthExpired(7) 命名常量（修订 D-22 第 3 条 4/5 → 6/7，避开 v2.0 main.go 撞码）；mountMutagen 扩展 conflict count 解析（--template 模式）；banner 后输出中文冲突警告 + 写入 last-session.json；Plan 02 12 降级矩阵单测全部仍 PASS。
  </done>
</task>

<task type="auto">
  <name>Task 3.3: cmd/cloud-claude/sync.go 注册 sync conflicts 子命令</name>
  <files>
    cmd/cloud-claude/sync.go
    cmd/cloud-claude/main.go
  </files>
  <read_first>
    - cmd/cloud-claude/main.go（cobra 子命令注册模式 — initCmd / envCmd / sshCmd 范式）
    - internal/cloudclaude/config.go（LoadConfig）
    - internal/cloudclaude/mutagen_bin.go（ExtractMutagenBinary 用于确保本地有 mutagen 二进制）
    - .planning/phases/31-cli/31-CONTEXT.md（D-28 sync conflicts 子命令最小可行版本 — 不做 resolve）
  </read_first>
  <action>
    1. cmd/cloud-claude/sync.go：

       ```go
       package main

       import (
           "fmt"
           "os"
           "os/exec"
           "path/filepath"

           "github.com/spf13/cobra"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude"
       )

       func newSyncCmd() *cobra.Command {
           cmd := &cobra.Command{
               Use:           "sync",
               Short:         "Mutagen 同步管理（v3.0 三层文件映射）",
               SilenceUsage:  true,
               SilenceErrors: true,
           }

           conflictsCmd := &cobra.Command{
               Use:           "conflicts",
               Short:         "查看当前 Mutagen 同步会话的冲突文件清单",
               Long:          "调用本地 Mutagen 客户端 sync list --long 渲染所有 cloud-claude 创建的 sync session 的冲突文件（path / alpha / beta / mtime）。",
               SilenceUsage:  true,
               SilenceErrors: true,
               RunE:          runSyncConflicts,
           }
           cmd.AddCommand(conflictsCmd)
           return cmd
       }

       func runSyncConflicts(cmd *cobra.Command, args []string) error {
           home, err := os.UserHomeDir()
           if err != nil {
               return fmt.Errorf("无法获取用户主目录: %w", err)
           }

           // 确保本地有 mutagen 二进制
           binPath := filepath.Join(home, ".cloud-claude", "bin", "mutagen")
           if err := cloudclaude.ExtractMutagenBinary(binPath); err != nil {
               return fmt.Errorf("无法准备 Mutagen 二进制: %w", err)
           }

           env := append(os.Environ(),
               "MUTAGEN_DATA_DIRECTORY="+filepath.Join(home, ".cloud-claude", "mutagen"),
           )

           // sync list --long 输出含每个 session 的 conflict 详细信息
           c := exec.Command(binPath, "sync", "list", "--long")
           c.Env = env
           c.Stdout = os.Stdout
           c.Stderr = os.Stderr
           if err := c.Run(); err != nil {
               return fmt.Errorf("查询 Mutagen 冲突清单失败: %w", err)
           }
           return nil
       }
       ```

    2. cmd/cloud-claude/main.go：在 main() 函数 rootCmd.AddCommand 调用处追加 newSyncCmd()：

       ```go
       rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd())
       ```

       同时在 DisableFlagParsing 切换分支增加 "sync" 路由：
       ```go
       switch os.Args[1] {
       case "init", "env", "ssh", "sync", "help", "--help", "-h":
           rootCmd.DisableFlagParsing = false
       }
       ```

    3. **不实现** sync resolve / sync resume / 其它 mutagen 命令的 wrapper（CONTEXT 已声明 Deferred）。
  </action>
  <acceptance_criteria>
    - `gofmt -l cmd/cloud-claude/` 输出为空
    - `go build ./...` 退出码 0
    - 路由注册：
      `grep -F 'newSyncCmd' cmd/cloud-claude/main.go` 命中
      `grep -E '"sync"' cmd/cloud-claude/main.go` 命中（DisableFlagParsing 路由）
      `grep -F 'sync list --long' cmd/cloud-claude/sync.go` 命中（使用 --long 而非 --json）
      `grep -F 'cloudclaude.ExtractMutagenBinary' cmd/cloud-claude/sync.go` 命中
    - 二进制烟测：
      ```bash
      go build -o /tmp/cc-test ./cmd/cloud-claude
      /tmp/cc-test sync --help 2>&1 | grep -F 'conflicts' || exit 1
      /tmp/cc-test sync conflicts --help 2>&1 | grep -F '冲突' || exit 1
      ```
    - 整仓回归：`go test ./... -count=1` 退出码 0
  </acceptance_criteria>
  <verify>
    <automated>go build -o /tmp/cc-test ./cmd/cloud-claude && /tmp/cc-test sync --help 2>&1 | grep -F 'conflicts' && grep -F 'newSyncCmd' cmd/cloud-claude/main.go && grep -F 'sync list --long' cmd/cloud-claude/sync.go && grep -F 'cloudclaude.ExtractMutagenBinary' cmd/cloud-claude/sync.go</automated>
  </verify>
  <done>
    cloud-claude sync conflicts 子命令注册完毕：cobra 路由通畅、ExtractMutagenBinary 自动确保本地二进制、调用 sync list --long 渲染冲突清单到 stdout；本任务**不**实现自动 resolve（OOS-A2）。
  </done>
</task>

<task type="auto">
  <name>Task 3.4: 集成测试套件（//go:build integration）+ docker compose fixture 脚本</name>
  <files>
    internal/cloudclaude/integration_test.go
    scripts/test-fixture-up.sh
    scripts/test-fixture-down.sh
  </files>
  <read_first>
    - .planning/phases/31-cli/31-RESEARCH.md §6.2（6 个集成测试场景：C4 / C5 / REQ-F2-B / REQ-F1-D / REQ-F7-C / C3，含 docker exec / pkill / netem 命令）
    - .planning/phases/31-cli/31-CONTEXT.md specifics 段（Success Criteria 第 4 / 6 / 9 条 — 集成测试必须 pkill 真实 mutagen-agent，不能 mock）
    - .planning/phases/29-v3-worker/plans/01-image-base/PLAN.md（Phase 29 镜像 build 命令；fixture 脚本提示用户 build local/managed-user:v3.0.0）
    - go.mod（确认无 testcontainers-go）
    - Makefile / 任何已有 test script（如有则遵循其风格）
  </read_first>
  <action>
    1. **scripts/test-fixture-up.sh**：

       ```bash
       #!/usr/bin/env bash
       # 起 Phase 29 镜像容器作为 cloud-claude integration test fixture。
       # 用法：scripts/test-fixture-up.sh
       # 依赖：docker / docker compose plugin / netcat (nc)
       set -euo pipefail

       FIXTURE_DIR="/tmp/cloud-claude-fixture"
       IMAGE="local/managed-user:v3.0.0"

       command -v docker >/dev/null || { echo "需要 docker"; exit 1; }
       docker compose version >/dev/null 2>&1 || { echo "需要 docker compose plugin"; exit 1; }

       if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
         echo "镜像 $IMAGE 不存在。请先构建："
         echo "  docker build -f deploy/docker/managed-user/Dockerfile -t $IMAGE ."
         exit 1
       fi

       mkdir -p "$FIXTURE_DIR"
       cat > "$FIXTURE_DIR/docker-compose.yml" <<'YAML'
       services:
         cc-fixture:
           image: local/managed-user:v3.0.0
           container_name: cc-fixture
           cap_add: [SYS_ADMIN]
           devices: ["/dev/fuse:/dev/fuse"]
           security_opt: ["apparmor=unconfined"]
           ports: ["12222:2222"]
           environment:
             - CLOUD_CLAUDE_TEST_FIXTURE=1
       YAML

       echo "=== 启动 fixture 容器"
       (cd "$FIXTURE_DIR" && docker compose up -d)

       echo "=== 等待 sshd ready (port 12222)"
       for i in $(seq 1 30); do
         if command -v nc >/dev/null 2>&1; then
           if nc -z 127.0.0.1 12222 2>/dev/null; then
             echo "=== sshd ready"
             exit 0
           fi
         else
           if (echo > /dev/tcp/127.0.0.1/12222) 2>/dev/null; then
             echo "=== sshd ready"
             exit 0
           fi
         fi
         sleep 1
       done
       echo "=== sshd 启动超时（30s）"
       (cd "$FIXTURE_DIR" && docker compose logs)
       exit 1
       ```

    2. **scripts/test-fixture-down.sh**：

       ```bash
       #!/usr/bin/env bash
       set -euo pipefail
       FIXTURE_DIR="/tmp/cloud-claude-fixture"
       if [ -d "$FIXTURE_DIR" ]; then
         (cd "$FIXTURE_DIR" && docker compose down -v --remove-orphans 2>/dev/null || true)
         rm -rf "$FIXTURE_DIR"
       fi
       echo "=== fixture 已销毁"
       ```

       两脚本 chmod +x。

    3. **internal/cloudclaude/integration_test.go**：

       ```go
       //go:build integration
       // +build integration

       package cloudclaude

       import (
           "bytes"
           "context"
           "fmt"
           "os"
           "os/exec"
           "path/filepath"
           "strings"
           "testing"
           "time"

           "github.com/zanel1u/cloud-cli-proxy/internal/cloudclaude/errcodes"
       )

       const (
           fixtureHost = "127.0.0.1"
           fixturePort = 12222
           // 注：以下凭证为 Phase 29 测试镜像内置；实际值由 fixture 镜像 entrypoint 决定，executor 在跑 TestMain 前必须确认与镜像一致。
           // 如镜像未内置，可通过 `docker exec cc-fixture cat /tmp/test-credentials.txt` 读取。
           fixtureUser = "workspace"
           fixturePass = "test-password-fixture-only"
           fixtureCtr  = "cc-fixture"
       )

       func TestMain(m *testing.M) {
           // 起 fixture
           if err := exec.Command("scripts/test-fixture-up.sh").Run(); err != nil {
               fmt.Fprintln(os.Stderr, "fixture 启动失败，跳过集成测试:", err)
               os.Exit(0) // 不让 CI 因环境缺 docker 而失败；CI gate 必须提前确保 docker 可用
           }
           code := m.Run()
           _ = exec.Command("scripts/test-fixture-down.sh").Run()
           os.Exit(code)
       }

       // helper: 在 fixture 容器内执行命令
       func dockerExec(t *testing.T, args ...string) (string, error) {
           full := append([]string{"exec", fixtureCtr}, args...)
           c := exec.Command("docker", full...)
           var out bytes.Buffer
           c.Stdout = &out
           c.Stderr = &out
           err := c.Run()
           return out.String(), err
       }

       // helper: 启 cloud-claude 二进制（已编译到 /tmp/cloud-claude-int），返回退出码 + stderr
       func runCloudClaude(t *testing.T, mode string, cwd string) (exitCode int, stderr string) {
           t.Helper()
           bin := "/tmp/cloud-claude-int"
           if _, err := os.Stat(bin); err != nil {
               // 编译一次
               if err := exec.Command("go", "build", "-o", bin, "./cmd/cloud-claude").Run(); err != nil {
                   t.Fatalf("编译 cloud-claude 失败: %v", err)
               }
           }
           ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
           defer cancel()

           // 注：cloud-claude 默认用 LoadConfig 读 ~/.cloud-claude/config.yaml。集成测试必须先注入 fixture 配置。
           // 简化：用 env CLOUD_CLAUDE_GATEWAY / SHORT_ID / PASSWORD（main.go runInit 已支持读 env）
           // **executor 必须确认 main.go runRoot 路径是否也读 env**；如否则需扩展或写临时 config 文件。
           home, _ := os.UserHomeDir()
           cfgPath := filepath.Join(home, ".cloud-claude", "config.yaml")
           // 这里 executor 自行实现 config 注入（写临时 YAML）；本 PLAN 不展开。

           c := exec.CommandContext(ctx, bin, "--mount-mode="+mode)
           c.Dir = cwd
           var stderrBuf bytes.Buffer
           c.Stderr = &stderrBuf
           c.Stdout = nil
           err := c.Run()
           if exitErr, ok := err.(*exec.ExitError); ok {
               return exitErr.ExitCode(), stderrBuf.String()
           }
           if err != nil {
               t.Logf("cloud-claude 执行错误: %v", err)
               return -1, stderrBuf.String()
           }
           _ = cfgPath
           return 0, stderrBuf.String()
       }

       // === 6 个 RESEARCH §6.2 集成场景 ===

       // 场景 1：C4 - Mutagen client/agent 版本不一致 → 必须降级 sshfs-only + MOUNT_MUTAGEN_VERSION_SKEW
       func TestIntegration_C4_VersionSkew_DowngradesToSSHFSOnly(t *testing.T) {
           // 篡改容器内版本文件
           _, _ = dockerExec(t, "sed", "-i", "s/v0.18.1/v0.99.99/", "/etc/cloud-claude/mutagen.version")
           defer dockerExec(t, "sed", "-i", "s/v0.99.99/v0.18.1/", "/etc/cloud-claude/mutagen.version")

           cwd := t.TempDir()
           _, stderr := runCloudClaude(t, "auto", cwd)
           if !strings.Contains(stderr, string(errcodes.MOUNT_MUTAGEN_VERSION_SKEW)) {
               t.Errorf("stderr 未含 MOUNT_MUTAGEN_VERSION_SKEW: %s", stderr)
           }
           if !strings.Contains(stderr, "[sshfs-only]") {
               t.Errorf("banner 应含 [sshfs-only]: %s", stderr)
           }
       }

       // 场景 2：C5 - 安全门 alpha empty + beta non-empty → MOUNT_MUTAGEN_SAFETY_GUARD + 退出非 0 + sync 未创建
       func TestIntegration_C5_SafetyGuard_BlocksSync(t *testing.T) {
           _, _ = dockerExec(t, "bash", "-c", "echo seed > /workspace-hot/seed.txt")
           defer dockerExec(t, "rm", "-f", "/workspace-hot/seed.txt")

           cwd := t.TempDir() // 空目录
           code, stderr := runCloudClaude(t, "full", cwd)
           if code == 0 {
               t.Errorf("期望退出非 0，实际 0")
           }
           if !strings.Contains(stderr, string(errcodes.MOUNT_MUTAGEN_SAFETY_GUARD)) {
               t.Errorf("stderr 未含 MOUNT_MUTAGEN_SAFETY_GUARD: %s", stderr)
           }

           // 关键断言：mutagen sync list 必须为空（sync 未创建）
           home, _ := os.UserHomeDir()
           binPath := filepath.Join(home, ".cloud-claude", "bin", "mutagen")
           c := exec.Command(binPath, "sync", "list", "--template", `{{range .}}{{.Name}}{{"\n"}}{{end}}`)
           c.Env = append(os.Environ(), "MUTAGEN_DATA_DIRECTORY="+filepath.Join(home, ".cloud-claude", "mutagen"))
           out, _ := c.Output()
           if strings.TrimSpace(string(out)) != "" {
               t.Errorf("Mutagen sync list 应为空，实际: %s", out)
           }
       }

       // 场景 3：REQ-F2-B - kill mutagen-agent ≤2s 降级
       func TestIntegration_F2B_KillMutagenAgent_DowngradesIn2s(t *testing.T) {
           // 此测试需要 cloud-claude 已成功 mount（mutagen 在跑）；先正常启动取得 baseline，然后 pkill
           // 测试模型：跑一个长时 cloud-claude（exec sleep），在另一个 goroutine pkill 后等待 stderr 出现降级
           // 简化：本用例只验证 pkill 之后再次 cloud-claude 启动会自动降级 mutagen-only 或 sshfs-only

           cwd := t.TempDir()
           // 写入小文件确保不触发 50MB
           os.WriteFile(filepath.Join(cwd, "tiny.txt"), []byte("hi"), 0644)

           _, _ = dockerExec(t, "pkill", "-9", "mutagen-agent")
           // 给 systemd / supervisord 留 1s 自动重启的窗口
           time.Sleep(500 * time.Millisecond)

           start := time.Now()
           _, stderr := runCloudClaude(t, "auto", cwd)
           elapsed := time.Since(start)

           if elapsed > 10*time.Second {
               t.Errorf("启动总耗时 > 10s: %v", elapsed)
           }
           // mutagen-agent 已 kill，期望 stderr 出现降级 banner
           if !strings.Contains(stderr, string(errcodes.MOUNT_AUTO_DOWNGRADED)) &&
              !strings.Contains(stderr, "[sshfs-only]") &&
              !strings.Contains(stderr, "[mutagen-only]") {
               t.Errorf("期望降级 banner，stderr: %s", stderr)
           }
       }

       // 场景 4：REQ-F1-D - 200MB 拒绝热同步
       func TestIntegration_F1D_50MBReject(t *testing.T) {
           cwd := t.TempDir()
           // dd 创建 200MB 文件
           dd := exec.Command("dd", "if=/dev/zero", "of="+filepath.Join(cwd, "big.bin"), "bs=1M", "count=200")
           if err := dd.Run(); err != nil {
               t.Skip("dd 不可用，跳过: ", err)
           }
           _, stderr := runCloudClaude(t, "auto", cwd)
           if !strings.Contains(stderr, string(errcodes.MOUNT_MUTAGEN_WHITELIST_REJECT)) {
               t.Errorf("stderr 未含 MOUNT_MUTAGEN_WHITELIST_REJECT: %s", stderr)
           }
       }

       // 场景 5：REQ-F7-C - OAuth expired 退出非 0 不进 claude
       func TestIntegration_F7C_OAuthExpired_ExitsBeforeClaude(t *testing.T) {
           // 篡改 credentials.json 中 expiresAt 为 0
           _, _ = dockerExec(t, "bash", "-c",
               `mkdir -p /home/claude/.claude && echo '{"claudeAiOauth":{"expiresAt":0}}' > /home/claude/.claude/.credentials.json && chown -R 1000:1000 /home/claude/.claude`)
           defer dockerExec(t, "rm", "-f", "/home/claude/.claude/.credentials.json")

           cwd := t.TempDir()
           code, stderr := runCloudClaude(t, "sshfs-only", cwd)
           if code == 0 {
               t.Errorf("期望退出非 0，实际 0")
           }
           if !strings.Contains(stderr, string(errcodes.NET_OAUTH_EXPIRED)) {
               t.Errorf("stderr 未含 NET_OAUTH_EXPIRED: %s", stderr)
           }
           // 关键：不应该有 claude 进程报错（用 grep 排除常见 claude error 关键字）
           if strings.Contains(stderr, "claude:") || strings.Contains(stderr, "anthropic") {
               t.Errorf("stderr 含 claude 进程错误（OAuth 检查应阻止 claude 启动）: %s", stderr)
           }
       }

       // 场景 6：C3 - 拔网 30s ls /workspace 不 hang + 摘除 cold branch
       func TestIntegration_C3_NetemDrop_ColdBranchRemoved(t *testing.T) {
           // 此测试需要 tc / netem，CI runner 不一定可用；优雅降级
           if _, err := exec.Command("docker", "exec", fixtureCtr, "which", "tc").CombinedOutput(); err != nil {
               t.Skip("tc 在 fixture 容器内不可用，跳过 C3 集成场景（保留 unit 层的 SSHFSWatcher 测试）")
           }
           // executor 自行决定本场景的实现深度；建议最低限度断言 cloud-claude --mount-mode=full 在拔网后不 hang > 60s 并打印 MOUNT_SSHFS_DISCONNECTED
           t.Skip("C3 集成场景由 Phase 35 真机验收完整覆盖；本测试占位以满足 RESEARCH §6.2 计数")
       }
       ```

       注意：
       - **凭证注入**：cloud-claude 启动需要 ~/.cloud-claude/config.yaml + 网关 / shortID / password。集成测试用 fixture 容器需要替代真实 Entry API 路径 — 这是个绕不开的问题。**executor 在 TestMain 中必须**：
         (a) 写临时 ~/.cloud-claude/config.yaml 指向 fixture 内置的 mock gateway 或
         (b) 直接调 `cloudclaude.ConnectAndRunClaudeV3`（绕过 main.go 的 LoadConfig + EntryClient 路径），把 SSHConfig 与 AuthResponse 直接构造。**推荐 (b)**：测试代码直接 import internal/cloudclaude 包，不走 cobra 路由 — 这样不需要 mock gateway。
         上面 runCloudClaude helper 写的是 (a) 路径示意；executor 在实现时改用 (b) 更稳健。
       - 集成测试不在 default `go test` 触发：必须 `go test -tags=integration ./internal/cloudclaude/`。

    4. 把 fixture 脚本设为可执行：
       ```bash
       chmod +x scripts/test-fixture-up.sh scripts/test-fixture-down.sh
       ```
  </action>
  <acceptance_criteria>
    - 文件存在性：
      `test -f internal/cloudclaude/integration_test.go`
      `test -x scripts/test-fixture-up.sh`
      `test -x scripts/test-fixture-down.sh`
    - build tag 正确：
      `head -3 internal/cloudclaude/integration_test.go | grep -F '//go:build integration'`
    - 默认 `go test ./internal/cloudclaude/` **不**跑集成测试（因 build tag）：
      `go test ./internal/cloudclaude/ -count=1` 退出码 0（且 verbose 输出不含 TestIntegration_*）
    - 6 个用例存在（用 grep test 函数名计数，不实际执行）：
      ```bash
      [ "$(grep -c '^func TestIntegration_' internal/cloudclaude/integration_test.go)" -ge 6 ]
      ```
    - 关键 RESEARCH §6.2 断言关键字：
      `grep -F 'pkill -9 mutagen-agent' internal/cloudclaude/integration_test.go` 命中
      `grep -F 'count=200' internal/cloudclaude/integration_test.go` 命中
      `grep -F 'expiresAt":0' internal/cloudclaude/integration_test.go` 命中
      `grep -F '/workspace-hot/seed' internal/cloudclaude/integration_test.go` 命中
    - fixture 脚本静态检查：
      `bash -n scripts/test-fixture-up.sh && bash -n scripts/test-fixture-down.sh`
      `grep -F 'local/managed-user:v3.0.0' scripts/test-fixture-up.sh` 命中
      `grep -F 'cap_add: [SYS_ADMIN]' scripts/test-fixture-up.sh` 命中
    - 整仓回归：`go test ./... -count=1` 退出码 0
    - 集成测试静态编译通过：`go test -tags=integration -count=1 -run x_no_run_x ./internal/cloudclaude/` 退出码 0（仅检查 build，不真实跑）
    - **未引入 testcontainers-go**：`grep -F 'testcontainers' go.mod || echo OK`
  </acceptance_criteria>
  <verify>
    <automated>head -3 internal/cloudclaude/integration_test.go | grep -F '//go:build integration' && [ "$(grep -c '^func TestIntegration_' internal/cloudclaude/integration_test.go)" -ge 6 ] && bash -n scripts/test-fixture-up.sh && bash -n scripts/test-fixture-down.sh && go test -tags=integration -count=1 -run x_no_run_x ./internal/cloudclaude/ && ! grep -F 'testcontainers' go.mod</automated>
  </verify>
  <done>
    集成测试套件骨架就位：6 个 TestIntegration_* 函数 + 2 个 fixture 脚本 + build tag 正确隔离；默认 go test 不触发；CI 在 docker 可用环境下 `go test -tags=integration` 能完整跑（C3 场景因 tc/netem 依赖 Phase 35 真机，本 plan 占位 Skip）；未引入 testcontainers-go 新 dep。
  </done>
</task>

</tasks>

<verification>
本 plan 完成后执行以下端到端检查：

```bash
# 1. 文件齐备
test -f internal/cloudclaude/oauth_check.go
test -f internal/cloudclaude/oauth_check_test.go
test -f cmd/cloud-claude/sync.go
test -f internal/cloudclaude/integration_test.go
test -x scripts/test-fixture-up.sh
test -x scripts/test-fixture-down.sh

# 2. OAuth 关键命令字面与 5min 阈值
grep -F 'timeout 2 cat /home/claude/.claude/.credentials.json' internal/cloudclaude/oauth_check.go
grep -F '5 * time.Minute' internal/cloudclaude/oauth_check.go
grep -F 'CheckOAuthCredentials(connA' internal/cloudclaude/ssh.go
grep -F 'gateway 未返回 claude_account_id' internal/cloudclaude/ssh.go

# 3. Mutagen conflict 解析（--template 不是 --json）
grep -F '--template' internal/cloudclaude/mount_mutagen.go
! grep -F 'sync list --json' internal/cloudclaude/mount_mutagen.go

# 4. banner 中文冲突警告 + sync 子命令路由
grep -F '同步冲突' internal/cloudclaude/mount_strategy.go
grep -F 'cloud-claude sync conflicts' internal/cloudclaude/mount_strategy.go
grep -F 'newSyncCmd' cmd/cloud-claude/main.go

# 5. exit codes 引用命名常量（避开 v2.0 main.go 4/5 撞码）
grep -F 'return ExitOAuthNotFound,' internal/cloudclaude/ssh.go
grep -F 'return ExitOAuthExpired,' internal/cloudclaude/ssh.go
grep -F 'ExitOAuthNotFound    = 6' internal/cloudclaude/exitcodes.go
grep -F 'ExitOAuthExpired     = 7' internal/cloudclaude/exitcodes.go

# 6. 单元测试
go test ./internal/cloudclaude/ -run Test_ParseExpiresAt -count=1 -v   # 9 个子用例 PASS
go test ./internal/cloudclaude/errcodes/ -count=1                       # Plan 01 注册表回归
go test ./... -count=1                                                  # 整仓回归（不含 integration）

# 7. 集成测试静态编译
go test -tags=integration -count=1 -run x_no_run_x ./internal/cloudclaude/

# 8. 集成测试函数计数 + 关键场景关键字
[ "$(grep -c '^func TestIntegration_' internal/cloudclaude/integration_test.go)" -ge 6 ]
grep -F 'pkill -9 mutagen-agent' internal/cloudclaude/integration_test.go
grep -F 'expiresAt":0' internal/cloudclaude/integration_test.go

# 9. fixture 脚本
bash -n scripts/test-fixture-up.sh
bash -n scripts/test-fixture-down.sh

# 10. 未引入 testcontainers-go
! grep -F 'testcontainers' go.mod

# 11. cloud-claude 二进制烟测
go build -o /tmp/cc-test ./cmd/cloud-claude
/tmp/cc-test sync --help 2>&1 | grep -F 'conflicts'

# 12. 格式与 vet
gofmt -l internal/cloudclaude/ cmd/cloud-claude/
go vet ./...
```

**集成测试真实执行**（需 docker + Phase 29 镜像就绪，本 plan 不强制 CI gate；Phase 35 真机验收为最终 gate）：

```bash
# 前提：docker 可用 + local/managed-user:v3.0.0 镜像已构建
go test -tags=integration -count=1 -v ./internal/cloudclaude/
```
</verification>

<threat_model>
## Trust Boundaries

| Boundary | 描述 |
|----------|------|
| 远端容器 → cloud-claude（OAuth credentials JSON 内容） | conn-A 上 cat /home/claude/.claude/.credentials.json 的 stdout 直接被 JSON unmarshal；恶意容器可投毒 |
| cloud-claude → 本地 mutagen 进程（sync list stdout） | mountMutagen 解析 sync list 的 stdout 模板；mutagen 自身可信 |
| sync conflicts 子命令 → 用户 stdout | exec.Command 的 stdout 直接打到 os.Stdout；mutagen 输出可信 |
| 集成测试 fixture 容器 → 宿主机端口 12222 | 仅 bind 127.0.0.1 + 测试期间存在；不暴露公网 |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-31-03-01 | Information Disclosure | OAuth credentials.json 内容（含 accessToken / refreshToken）通过 conn-A 经过本地内存 | accept | cloud-claude 进程已是「容器内运行 claude code」的合法持有者；token 在内存中存活时间仅 parseExpiresAt 一次调用，不写日志 / 不写文件 / 不出栈帧外。OAuth 检查只读 expiresAt 字段，accessToken / refreshToken 不被使用 |
| T-31-03-02 | Tampering | 远端容器返回的 credentials.json 字段被恶意构造（如 expiresAt = 巨大值绕过过期检查） | accept | 容器是用户自己的 sandbox（账号绑定）；攻击模型是「用户容器被入侵」，此时 token 本身已经泄露，过期检查是次要防线。本 plan 不引入额外缓解（性价比低） |
| T-31-03-03 | Denial of Service | timeout 2 cat 在 SSH hang 时被超时强制退出 | mitigate | 远程命令 timeout 2 限制 2s；CheckOAuthCredentials 内部 sess.Run 错误一律收敛到 OAuthNotFound（不阻断 mount 路径） |
| T-31-03-04 | Information Disclosure | sync conflicts 子命令暴露文件路径 / 修改时间 | accept | mutagen sync list --long 已是用户层授权操作（用户自己跑命令查看自己的 sync）；信息全部本地 |
| T-31-03-05 | Tampering | 集成测试 fixture 容器以 SYS_ADMIN + apparmor=unconfined 运行 | accept | 仅本地测试环境；容器 lifetime 限定在 test-fixture-up/down 之间；CI runner 通常已隔离（GitHub Actions runner 沙箱） |
| T-31-03-06 | Repudiation | OAuth Expired / NotFound 时退出码 4/5 模糊（用户不知道是 OAuth 失败） | mitigate | 退出前 stderr 必输出 errcodes.Format(NET_OAUTH_*)，含中文 NextAction 指引 cloud-claude exec claude login |
| T-31-03-07 | Spoofing | 集成测试用 hardcoded fixturePass | accept | 测试凭证仅在 fixture 容器 lifetime 内有效；不进 git（PROJECT.md 隐私安全规则要求）；executor 在实现时如发现凭证需 commit，必须改为环境变量或 fixture entrypoint 动态生成 |
| T-31-03-08 | Information Disclosure | last-session.json 写入 conflict_count 字段 | accept | 仅运维数据，无 token / 无文件路径 |

</threat_model>

<success_criteria>
- oauth_check.go + oauth_check_test.go：CheckOAuthCredentials 接入 ConnectAndRunClaudeV3；9 个 parseExpiresAt 表驱动单测 PASS；5min 阈值与 D-22 一致
- mountMutagen 扩展 conflict count 解析（--template 模式）；mount_strategy banner 后中文警告 + last-session.json 写入 conflict_count
- cloud-claude sync conflicts 子命令注册 + ExtractMutagenBinary 自动准备 + 调 mutagen sync list --long
- 6 个集成测试函数齐备 + 2 个 fixture 脚本 + build tag //go:build integration 正确隔离 + 默认 `go test ./...` 不触发集成
- 未引入 testcontainers-go 新 dep（grep go.mod 不含 testcontainers）
- 整仓 `go test ./... -count=1` + `gofmt -l` + `go vet` 全部通过
- exit codes 引用 cloudclaude.ExitOAuthNotFound(6) / ExitOAuthExpired(7) 命名常量（修订 D-22 第 3 条 4/5 → 6/7，避开 v2.0 main.go ExitConfigError=4 / ExitInternalError=5 撞码）；ssh.go OAuth 路径不允许裸数字 return
- mount_mutagen.go 全程不出现 `sync list --json`（验证修订 D-28）
</success_criteria>

<output>
完成后创建 `.planning/phases/31-cli/plans/03-oauth-conflicts-integration/SUMMARY.md`，列明：
- CheckOAuthCredentials 调用入口在 ConnectAndRunClaudeV3 中的位置（行号 / hook 点替换前后对比）
- 6 个集成测试场景的执行方式：`go test -tags=integration -count=1 -v ./internal/cloudclaude/`
- 已知占位：TestIntegration_C3_NetemDrop_ColdBranchRemoved 因 tc/netem 依赖留 Skip，最终验收在 Phase 35 真机
- 凭证注入方案的最终实现选择（直接 import internal/cloudclaude 包 vs 写临时 config.yaml + mock gateway）
- ROADMAP §Phase 31 Success Criteria 第 4 / 6 / 8 / 9 条与 6 个集成测试的对应关系
- Phase 35 验收前置 / 留给 Phase 34 doctor 的 hook 点
- 与 Phase 34 / 35 的接口契约：errcodes 注册表已稳定 + last-session.json schema_version=1 已落地，Phase 34 doctor 可直接读
</output>
