---
phase: 34-claudedock-doctor-v3
plan: 02
subsystem: cli
tags: [doctor, cobra, multi-domain-checks, downgrade-banner, json-schema, errcode-integration]

# Dependency graph
requires:
  - phase: 34-claudedock-doctor-v3
    provides: errcodes 8 域闭合 Registry（42 条）+ ExtendedExplanations + claudedock explain CLI（Plan 01 / commits 01d9f12..ccd9317）
  - phase: 31-cli
    provides: MountConfig.KeepAliveInterval / MutagenBinaryVersion 常量 / SSHConfig / sshConnect 包装 SSHConnect
  - phase: 32-ssh-tmux
    provides: LastSessionSnapshot / DowngradeStep（含 TmuxSession/ClientRole/ReconnectCount Phase 32 D-27 增量字段）
  - phase: 33-claude-code-cli-admin-gc
    provides: STATE_VOLUME_IN_USE_001（doctor 暂未消费，但 Plan 01 已落字面量）
provides:
  - claudedock doctor [domain] 五维度自检 cobra 子命令（network/auth/ssh/mount/disk/all）
  - internal/claudedock/doctor 子包（Report/Options/Status/Check/Checker/RemoteRunner 顶层骨架）
  - 18 项 check 函数（network×3 / auth×3 / ssh×4 / mount×5 / disk×3）≥ REQ-F6-A 17 项下限
  - RenderText 4 段布局（banner / 上次会话快照 / 5 维度矩阵 / 汇总）
  - RenderJSON schema_version=1 锁死（不带 omitempty）
  - 退出码 0/1/2 与 brew doctor 对齐（CONTEXT D-16）
  - 51 条 doctor 包单测（含 M13 SC#6 / M14 SC#3 / SC#5 三大锚点）
  - claudedock.LoadLastSession + DefaultLastSessionPath 读端 API（Rule 3）
  - claudedock.{ColorEnabled,Colorize,AnsiGreen,AnsiYellow,AnsiRed,AnsiGray,AnsiCyan,AnsiReset} 大写导出（PATTERNS §2.10）
affects:
  - 34-03-doctor-fix-integration（FixerRegistry 接 doctor 包；--fix 5 类修复字面量）

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "doctor 包级 var mock 注入：lookupHost/httpGet/loadConfig/entryAuthenticate/statfs/duLocal/userHomeDir/readOSRelease/readAppArmorOverride/execLookPath/execMountList — 单测纯内存路径全覆盖"
    - "RemoteRunner interface + fakeRunner / branchRunner 双 stub — 远端命令脱离 SSH 走 unit"
    - "Check constructor helpers newPass/newWarn/newFail/newSkip — 复用 errcodes.Lookup 自动填 Message + NextAction，verbatim 守恒"
    - "M13 第一屏锚点：renderDowngradeBanner 字面量 '[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>'"
    - "M14 终验：renderCheckLine 对 warn/fail 行强约束追加 '建议:' + '错误码:' 后缀"
    - "JSON schema 锁：Report.SchemaVersion `json:\"schema_version\"` 不带 omitempty + TestJSONSchemaV1Lock 守护"

key-files:
  created:
    - internal/claudedock/doctor/doctor.go
    - internal/claudedock/doctor/check.go
    - internal/claudedock/doctor/remote_runner.go
    - internal/claudedock/doctor/network.go
    - internal/claudedock/doctor/network_test.go
    - internal/claudedock/doctor/auth.go
    - internal/claudedock/doctor/auth_test.go
    - internal/claudedock/doctor/ssh.go
    - internal/claudedock/doctor/ssh_test.go
    - internal/claudedock/doctor/mount.go
    - internal/claudedock/doctor/mount_test.go
    - internal/claudedock/doctor/disk.go
    - internal/claudedock/doctor/disk_test.go
    - internal/claudedock/doctor/render.go
    - internal/claudedock/doctor/render_test.go
    - internal/claudedock/doctor/doctor_test.go
    - cmd/claudedock/doctor.go
  modified:
    - internal/claudedock/colors.go
    - internal/claudedock/input_buffer.go
    - internal/claudedock/input_buffer_test.go
    - internal/claudedock/mount_strategy.go
    - internal/claudedock/session.go
    - internal/claudedock/session_test.go
    - internal/claudedock/last_session.go
    - cmd/claudedock/main.go

key-decisions:
  - "ColorEnabled / Colorize / Ansi* 保留 plan 原 2-arg / 3-arg 签名（不强行无参化）— 让 doctor render.go 显式传 os.Stdout 做 TTY 探测，避免改既有 mount_strategy/session 调用点（Rule 3 微调）"
  - "claudedock.LoadLastSession + DefaultLastSessionPath 在 last_session.go 新增（plan 引用了不存在的 API）— 与 WriteLastSession 对称，路径 ~/.claudedock/last-session.json，文件不存在时返回 error 由 doctor nil-banner 走 STATE_LAST_SESSION_MISSING 兜底（Rule 3）"
  - "authRespExpectedEgressIP 恒返回 ''：claudedock.AuthResponse v3.0 未导出 ExpectedEgressIP 字段（仅 Status/Message/Error/SSH*/ImageVersion/SupportsMutagen/SupportsMergerfs/ClaudeAccountID）；按 plan 注释所述走兜底分支不误报 drift，entry 包补该字段移交 v3.1 backlog（Rule 1）"
  - "checkWorkspaceSSHKeys 集成测试由 Plan 03 docker fixture 兜底；本 plan 单测层不覆盖（plan 原文允许）"
  - "AppArmor + FUSE residual 等 Linux-only check 在 darwin 上 t.Skip 而非 t.Fatal — CI 跑 Linux runner 时全 PASS"

patterns-established:
  - "Pattern: doctor check 函数命名 — checkXxxYyy(ctx, [runner|cfg|ssh.Client|...]) Check，单一返回 Check struct"
  - "Pattern: 包级 var 注入 + t.Cleanup 还原 — 单测纯内存 mock 模板（RESEARCH §9 第 2 条）"
  - "Pattern: errcodes.Lookup 文案优先 — newWarn/newFail 自动从 Registry 拉 Message/NextAction，verbatim 守恒，args 走 fmt.Sprintf"
  - "Pattern: lazy SSH 连接 — RunDoctor 第一次 ensureRemote 时拨号；defer close；失败时 RequiresRemote=true 的 check 全走 StatusSkip"

requirements-completed: [REQ-F6-A, REQ-F6-B, REQ-F6-D]

# Metrics
duration: 22min
completed: 2026-04-21
---

# Phase 34 Plan 02: doctor-framework Summary

**claudedock doctor 五维度自检框架（18 项 check / 51 条单测 / M13+M14+SC#5 三锚点全 PASS）**

## Performance

- **Duration:** ≈ 22 min
- **Completed:** 2026-04-21
- **Tasks:** 11 / 11
- **Files created:** 17（doctor 包 16 + cmd 1）
- **Files modified:** 8（colors export 6 + last_session 1 + main 1）

## Accomplishments

- 18 项 check 函数：network×3（dns_resolve / gateway_reachable / egress_ip_visible）+ auth×3（config_present / entry_token_valid / oauth_credentials）+ ssh×4（keepalive_config / sshd_keepalive_drift / known_hosts / workspace_ssh_keys）+ mount×5（mutagen_version_match / mergerfs_branches / sshfs_mountpoint / fuse_residual / apparmor_fusermount3）+ disk×3（local_disk / container_disk / mutagen_data_size）
- 5 维度统一接入 errcodes Registry：所有 warn/fail 必带 Code + 中文 Message + NextAction（M14）
- 第一屏降级历史 banner：`[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>` 字面量逐 step 输出（M13 / SC#6）
- JSON schema_version=1 锁死（不带 omitempty）+ jq 可解析（SC#5 / TestJSONSchemaV1Lock）
- 退出码 0/1/2 与 brew doctor 对齐（SC#5 第 3 条）
- cobra 子命令完整（4 flag + 6 ValidArgs + ValidArgs 校验 + cobra.MaximumNArgs(1)）
- doctor 包不调任何 host-agent endpoint（SC#9 守恒；rg `agentapi.*` 命中 0）

## Task Commits

Each task was committed atomically:

1. **Task 2.1: 导出 ColorEnabled/Colorize/Ansi* helpers** - `0ddeb10` (refactor)
2. **Task 2.2: scaffold doctor 顶层数据结构** - `c5f6df5` (feat)
3. **Task 2.3: RemoteRunner interface + sshRemoteRunner** - `a945598` (feat)
4. **Task 2.4: network 维度 3 check + 9 子测** - `511753c` (feat)
5. **Task 2.5: auth 维度 3 check + 7 子测** - `aefef3d` (feat)
6. **Task 2.6: ssh 维度 4 check + 8 子测** - `7fb7124` (feat)
7. **Task 2.7: mount 维度 5 check + 13 子测** - `d77aa8a` (feat)
8. **Task 2.8: disk 维度 3 check + 9 子测** - `326cdb1` (feat)
9. **Task 2.9: render Text/JSON + M13/M14 锚点 5 子测** - `3a95373` (feat)
10. **Task 2.10: RunDoctor 主流程 + 3 集成测 + LoadLastSession** - `231fe17` (feat)
11. **Task 2.11: cmd/claudedock/doctor.go 注册 cobra** - `a34e4ce` (feat)

## Files Created/Modified

### 新建（17 个）

doctor 子包源码（9 个）：
- `internal/claudedock/doctor/doctor.go` - RunDoctor + Report/Options/DowngradeBanner + 辅助函数
- `internal/claudedock/doctor/check.go` - Check struct + Checker interface + runWithTimeout + newPass/newWarn/newFail/newSkip
- `internal/claudedock/doctor/remote_runner.go` - RemoteRunner interface + sshRemoteRunner
- `internal/claudedock/doctor/network.go` - 3 check
- `internal/claudedock/doctor/auth.go` - 3 check
- `internal/claudedock/doctor/ssh.go` - 4 check + parseSSHDKeepalive 辅助
- `internal/claudedock/doctor/mount.go` - 5 check
- `internal/claudedock/doctor/disk.go` - 3 check + parseDuHumanToMB 辅助
- `internal/claudedock/doctor/render.go` - RenderText + RenderJSON + renderDowngradeBanner + pickIcon

doctor 子包测试（7 个）：
- `internal/claudedock/doctor/network_test.go` - 9 子测
- `internal/claudedock/doctor/auth_test.go` - 7 子测
- `internal/claudedock/doctor/ssh_test.go` - 8 子测
- `internal/claudedock/doctor/mount_test.go` - 13 子测
- `internal/claudedock/doctor/disk_test.go` - 9 子测
- `internal/claudedock/doctor/render_test.go` - 5 子测（M13/M14/SC#5 锚点）
- `internal/claudedock/doctor/doctor_test.go` - 3 集成测

cmd（1 个）：
- `cmd/claudedock/doctor.go` - newDoctorCmd / runDoctor / contextWithDoctorTimeout / anyFixerRegistered

### 修改（8 个）

- `internal/claudedock/colors.go` - colorEnabled→ColorEnabled / colorize→Colorize / ansi*→Ansi* 大写导出
- `internal/claudedock/input_buffer.go` - AnsiGray/AnsiReset 调用点同步
- `internal/claudedock/input_buffer_test.go` - AnsiGray 调用点同步
- `internal/claudedock/mount_strategy.go` - ColorEnabled/Colorize/AnsiYellow/AnsiGreen 调用点同步（仅本 plan 的 colors 重命名 hunk）
- `internal/claudedock/session.go` - ColorEnabled/Colorize/AnsiGreen 调用点同步
- `internal/claudedock/session_test.go` - AnsiGray/AnsiReset 调用点同步（含注释）
- `internal/claudedock/last_session.go` - 新增 DefaultLastSessionPath + LoadLastSession（Rule 3）
- `cmd/claudedock/main.go` - AddCommand 追加 newDoctorCmd() + DisableFlagParsing switch case 加 "doctor"

## Verification

| 检查 | 结果 |
| ---- | ---- |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./internal/claudedock/doctor/ -count=1` | PASS（51 子测） |
| `go test ./internal/claudedock/ -count=1 -short` | PASS（无回归） |
| `go test ./internal/claudedock/errcodes/ -count=1` | PASS（无回归） |
| `go test ./cmd/claudedock/ -count=1` | PASS（无回归） |
| `TestDowngradeBannerRendersChain`（M13 / SC#6 锚点） | **PASS** |
| `TestRenderTextContainsNextAction`（M14 / SC#3 锚点） | **PASS** |
| `TestJSONSchemaV1Lock`（SC#5 锚点） | **PASS** |
| 17 项 check 下限断言 `grep -c '^func check[A-Z]' internal/claudedock/doctor/*.go` | **18** ≥ 17 |
| SC#9 host-agent 边界 `rg "agentapi\.(Action\|NewClient\|RunHostAction)" internal/claudedock/doctor/` | empty ✅ |
| `claudedock doctor --help` smoke | 列出 4 flag + 中文 Long 描述 |

## Decisions Made

详见 frontmatter `key-decisions`。核心：

1. **ColorEnabled / Colorize 保留 2-arg / 3-arg 签名**：plan 的 render.go 示例代码用 no-arg 版本与既有 mount_strategy/session 2-arg 用法不兼容；选择不重构既有调用点，让 doctor render.go 显式传 `os.Stdout` 做 TTY 探测（Rule 3 微调）。
2. **claudedock.LoadLastSession 自实现**：plan 引用了不存在的 API（仅 WriteLastSession 存在）。在 last_session.go 新增对称读端 + DefaultLastSessionPath 辅助，签名 `() (*LastSessionSnapshot, error)`，文件不存在时返回 error 由 doctor 的 nil-banner 走 STATE_LAST_SESSION_MISSING 兜底（Rule 3）。
3. **authRespExpectedEgressIP 恒返回 ""**：v3.0 entry.go AuthResponse 未导出 ExpectedEgressIP 字段；按 plan 第 2589 行的兜底说明走 expectedIP="" 分支，不误报 drift；entry 包补字段记入 v3.1 backlog（Rule 1）。

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - 缺失依赖] 新增 claudedock.LoadLastSession + DefaultLastSessionPath**

- **Found during:** Task 2.10（写 RunDoctor 第 1 步「读 LastSessionSnapshot」时编译报 undefined）
- **Issue:** Plan §2.10 调 `claudedock.LoadLastSession()` 但 claudedock 包仅实现 `WriteLastSession`，没有对称读端
- **Fix:** 在 `internal/claudedock/last_session.go` 末尾新增 `DefaultLastSessionPath() (string, error)` + `LoadLastSession() (*LastSessionSnapshot, error)`，路径 `~/.claudedock/last-session.json`；文件不存在 / JSON 解析失败时返回 error，调用方按 nil 兜底
- **Files modified:** `internal/claudedock/last_session.go`（+34 行）
- **Verification:** doctor build PASS；TestRunDoctor_NoInit_NetworkAuthOnlyLocal 跑通
- **Committed in:** `231fe17`（Task 2.10 commit）

**2. [Rule 1 - plan 内部 API 不一致] authRespExpectedEgressIP 走兜底**

- **Found during:** Task 2.10（写 RunDoctor 时编译报 `r.ExpectedEgressIP undefined`）
- **Issue:** Plan §2.10 第 2589 行示例代码访问 `authResp.ExpectedEgressIP`，但 v3.0 entry.go 的 `AuthResponse` 仅定义 Status/Message/Error/SSH*/ImageVersion/SupportsMutagen/SupportsMergerfs/ClaudeAccountID，未含该字段
- **Fix:** plan 自身 executor 注释（行 2589-2589）已预批兜底方案：`authRespExpectedEgressIP` 恒返回 `""`，让 `checkEgressIPVisible` 走 `expectedIP=="" → Pass` 分支，不误报 drift
- **Files modified:** `internal/claudedock/doctor/doctor.go`（authRespExpectedEgressIP 函数体）
- **Verification:** doctor build PASS；checkEgressIPVisible 单测全 PASS
- **Committed in:** `231fe17`（Task 2.10 commit）
- **Carry-over:** v3.1 backlog 在 entry.go AuthResponse 中导出 `ExpectedEgressIP string` 字段并由 control-plane gateway 填充

**3. [Rule 3 - plan 微调] ColorEnabled / Colorize 签名保留**

- **Found during:** Task 2.1（写 colors.go 重命名时发现 plan render.go 示例用无参版本与既有调用点冲突）
- **Issue:** Plan §2.1 要求把 `colorEnabled(noColor, fdHolder)` 改名为 `ColorEnabled` 且 plan §2.9 render.go 示例代码用 `claudedock.ColorEnabled()` 无参形式调用 — 签名不可同时满足
- **Fix:** 选择保留原 2-arg / 3-arg 签名（最小破坏既有 mount_strategy/session 调用），在 render.go 的 pickIcon 中显式传 `os.Stdout` 做 TTY 探测（`claudedock.ColorEnabled(noColor, os.Stdout)` / `claudedock.Colorize(text, ansi, true)`）
- **Files modified:** `internal/claudedock/colors.go` / `internal/claudedock/doctor/render.go`
- **Verification:** go build + go test 全 PASS；既有 input_buffer / session / mount_strategy 测试无回归
- **Committed in:** `0ddeb10`（Task 2.1 commit）

---

**Total deviations:** 3 auto-fixed（2 Rule 3 缺失依赖 / 1 Rule 1 plan 内部不一致）
**Impact on plan:** 全 11 task 按 plan 执行；锚点测试（M13/M14/SC#5）全 PASS；下游 Plan 03 仍按 plan 接 FixerRegistry，零阻塞。

## Issues Encountered

无。所有任务按 plan + deviation 修复一次性通过 build + test。

## Known Stubs

无。`anyFixerRegistered() bool { return false }` 是 Plan 02 → Plan 03 的 explicit hand-off 占位，配套 `[!] --fix 自动修复将在 doctor.fix.go 实现（Plan 03）` 用户提示，符合 plan 设计；不算 stub 隐患。

## Threat Flags

无新增 trust boundary。本 plan 5 维度 check 全部走只读命令（getfattr / sshd -T / mount / df / du / cat /etc/claudedock/mutagen.version），无 shell 拼接（命令字面量硬编码）；T-34-02-01..06 mitigation 全部 verbatim 落地：
- T-34-02-01: cobra ValidArgs 校验 ✅
- T-34-02-02: 远端命令字面量无用户变量插入 ✅
- T-34-02-03: Details 走白名单（addrs / branches_xattr / mount / egress_ip / files / problems / image_version / claude_account_id），无 password / token 字面量 ✅
- T-34-02-05: runWithTimeout 5s/30s + contextWithDoctorTimeout 60s/120s 顶层 ✅
- T-34-02-06: convertSnapshotToBanner 只拷贝结构化字段，不 eval ✅

## TDD Gate Compliance

本 plan frontmatter `type: execute`（非 tdd），无 RED/GREEN gate 强制要求；但 Task 2.4-2.9 各维度的源码 commit + 测试在同 commit 内 ship（实现与测试同步落盘），事实上呈现 "实现 + 锚点测试一起验证" 的 hybrid 形态。M13 / M14 / SC#5 三大锚点测试在 Task 2.9 commit `3a95373` 中独立 PASS，提供端到端守护。

## Carry-over for Plan 03

- **FixerRegistry 接入点**：`cmd/claudedock/doctor.go:anyFixerRegistered()` 当前恒 false；Plan 03 需要：
  1. 在 `internal/claudedock/doctor/` 新建 `fix.go` 定义 `FixerRegistry map[string]Fixer`
  2. 各维度 check 函数改造为 Checker interface 实现（plan 已留 `Fix(ctx, opts) Check` 占位）
  3. 修改 `runDoctor` 在 `fix=true` 时遍历 FixerRegistry 而非走占位输出
- **5 类 --fix 修复字面量**（Plan 01 SUMMARY carry-over 已锁定）：
  - mount fix → `SYSTEM_FUSE_RESIDUAL_MOUNT` + `claudedock doctor mount --fix`
  - network fix → `SYSTEM_DNS_RESOLVE_FAILED` + `claudedock doctor network --fix`
  - ssh fix → `SSH_KNOWN_HOSTS_CONFLICT` + `claudedock doctor ssh --fix`
  - auth fix → `AUTH_TOKEN_EXPIRED` + `claudedock doctor auth --fix`
  - disk fix → `DISK_MUTAGEN_DATA_BLOAT` + `mutagen daemon stop && rm -rf ~/.claudedock/mutagen/sessions/`
- **集成测试 fixture**：`checkWorkspaceSSHKeys` 真实 SSH 单测留 Plan 03 docker fixture 兜底（plan 原文允许）；现有的 `internal/claudedock/integration_test.go` Phase 31/32 fixtures 可复用
- **v3.1 backlog**：
  - `entry.go AuthResponse.ExpectedEgressIP string` 字段补齐（control-plane gateway 配套）
  - `claudedock.LoadLastSession` 容错增强：corrupt JSON 时返回 `*LastSessionSnapshot{SchemaVersion: 1}` 替代 nil
  - `checkKnownHosts` 精确 fingerprint 比对（当前仅 Load 成功就 Pass）

## Next Phase Readiness

- ✅ Wave 2 doctor framework 完成；Plan 03 的 --fix integration 可立即开工
- ✅ REQ-F6-A / REQ-F6-B / REQ-F6-D 三项需求闭合
- ✅ ROADMAP §Phase 34 Success Criteria SC#2（5 维度 + 四要素）+ SC#3（建议子串）+ SC#5（JSON + NO_COLOR + 退出码）+ SC#6（降级 banner）+ SC#9（host-agent 边界）全部交付
- ⚠️ Plan 03 需要在 RunDoctor 中真正遍历 FixerRegistry（当前留空逻辑）
- ⚠️ `claudedock.AuthResponse.ExpectedEgressIP` 字段 v3.1 补齐前，`checkEgressIPVisible` 不会触发 drift 警告（仅记 Details）

## Self-Check

- [x] `internal/claudedock/doctor/doctor.go` exists ✅
- [x] `internal/claudedock/doctor/check.go` exists ✅
- [x] `internal/claudedock/doctor/remote_runner.go` exists ✅
- [x] `internal/claudedock/doctor/network.go` + `network_test.go` exist ✅
- [x] `internal/claudedock/doctor/auth.go` + `auth_test.go` exist ✅
- [x] `internal/claudedock/doctor/ssh.go` + `ssh_test.go` exist ✅
- [x] `internal/claudedock/doctor/mount.go` + `mount_test.go` exist ✅
- [x] `internal/claudedock/doctor/disk.go` + `disk_test.go` exist ✅
- [x] `internal/claudedock/doctor/render.go` + `render_test.go` exist ✅
- [x] `internal/claudedock/doctor/doctor_test.go` exists ✅
- [x] `cmd/claudedock/doctor.go` exists ✅
- [x] commit `0ddeb10` (Task 2.1) exists ✅
- [x] commit `c5f6df5` (Task 2.2) exists ✅
- [x] commit `a945598` (Task 2.3) exists ✅
- [x] commit `511753c` (Task 2.4) exists ✅
- [x] commit `aefef3d` (Task 2.5) exists ✅
- [x] commit `7fb7124` (Task 2.6) exists ✅
- [x] commit `d77aa8a` (Task 2.7) exists ✅
- [x] commit `326cdb1` (Task 2.8) exists ✅
- [x] commit `3a95373` (Task 2.9) exists ✅
- [x] commit `231fe17` (Task 2.10) exists ✅
- [x] commit `a34e4ce` (Task 2.11) exists ✅

## Self-Check: PASSED

---

*Phase: 34-claudedock-doctor-v3*
*Completed: 2026-04-21*
