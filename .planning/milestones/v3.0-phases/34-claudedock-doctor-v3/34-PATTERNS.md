# Phase 34 Patterns: claudedock doctor v3 + 错误码统一

**Mapped:** 2026-04-21

> **使用方式**：每个新文件给一个最近的「现网代码模板」 + 直接抄录的代码片段 + 「verbatim 抄哪部分 / 改哪部分」清单。所有片段均来自当前仓库（行号准确，可直接 `Grep` 复核）。Plan 01/02/03 在写代码前先按本文件的 verbatim/adapt 列表完成 80%，再针对差异做 20% 改造。

---

## 1. errcodes 包扩展（Plan 01 — Wave 1）

`errcodes/mount.go` 是五个新域文件（`state.go` / `system.go` / `ssh.go` / `auth.go` / `disk.go`）的**统一模板**。`session.go` / `net.go` 是同模板的两份样例。所有新文件**只需** `package errcodes` + `//nolint:lll` + 一个 `func init()` 调一串 `MustRegister`，**禁止**新增 helper / 全局变量。

### 1.1 `internal/claudedock/errcodes/state.go`

**Role:** STATE_* 错误码注册（持久化 / volume / 容器状态）  
**Closest analog:** `internal/claudedock/errcodes/mount.go`（init + 12 条 MOUNT_*） + `internal/claudedock/errcodes/codes.go:120-146` const 块

**Verbatim copy（直接抄）：**

```7:14:internal/claudedock/errcodes/mount.go
func init() {
	MustRegister(Entry{
		Code:       MOUNT_MUTAGEN_VERSION_SKEW,
		Severity:   SeverityError,
		Message:    "Mutagen 客户端版本 (%s) 与容器内 agent 版本 (%s) 不一致，已降级到 sshfs-only",
		NextAction: "升级容器镜像到 v3.0.0+ 或重装 claudedock",
	})
```

文件头三行注释 + `func init()` + 每条 `MustRegister(Entry{...})` 五字段缩进，**逐字符照抄**。文件顶部第二行 `//nolint:lll` 是设计要求（Message 中文长句不能折行，破坏 `errcodes/codes_test.go:34` 的字面量回归）。

**Adapt（按 CONTEXT D-21 表）：**
- `package errcodes` 后写注释 `// STATE_* 错误码注册（Phase 34 D-21）。`
- 3 条新码：
  | Code 常量名 | Severity | Message | NextAction |
  |-------------|----------|---------|-----------|
  | `STATE_LAST_SESSION_MISSING` | `SeverityInfo` | `"未找到上次会话快照（%s）"` | `"首次运行 claudedock 后再 doctor 即可看到"` |
  | `STATE_VOLUME_IN_USE_001` | `SeverityError` | `"持久化 volume %s 仍被容器持有，DELETE 拒绝"` | `"先停止容器：claudedock admin hosts stop <id>"` |
  | `STATE_CONTAINER_NOT_RUNNING` | `SeverityWarn` | `"主机 %s 状态为 %s，远端 doctor 检查跳过"` | `"运行 claudedock admin hosts start <id> 启动容器"` |
- **必须**先在 `errcodes/codes.go:120-146` 的 `const ( ... Code = "..." )` 块**末尾**追加这 3 个常量字面量（与变量名严格一致），否则 `MustRegister` 编译期 grep 不到。

### 1.2 `internal/claudedock/errcodes/system.go`

**Role:** SYSTEM_* 错误码注册（OS / kernel / FUSE / DNS / timeout）  
**Closest analog:** 同 `mount.go` 模板

**Adapt（CONTEXT D-21 + RESEARCH §3.4 / §4.5）：**
- 4 条：
  | 常量名 | Severity | Message | NextAction |
  |--------|----------|---------|-----------|
  | `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` | `SeverityError` | `"AppArmor 缺 fusermount3 override（%s）"` | `"按 host-preflight.sh 写入 capability dac_override 行"` |
  | `SYSTEM_FUSE_RESIDUAL_MOUNT` | `SeverityWarn` | `"发现 %d 个残留 FUSE 挂载: %s"` | `"运行 claudedock doctor mount --fix 自动解挂"` |
  | `SYSTEM_DNS_RESOLVE_FAILED` | `SeverityError` | `"DNS 解析 %s 失败: %s"` | `"运行 claudedock doctor network --fix 刷新 DNS 缓存"` |
  | `SYSTEM_CHECK_TIMEOUT` | `SeverityWarn` | `"检查 %s 超时（>%s）"` | `"加 --verbose 放宽到 30s，或检查远端容器状态"` |
- `NextAction` 严格 ≤ 80 runes（`codes_test.go:43-45` 强制断言）。

### 1.3 `internal/claudedock/errcodes/ssh.go`

**Role:** SSH_* 错误码注册（known_hosts / sshd 基线漂移）  
**Closest analog:** `errcodes/mount.go` + `errcodes/session.go`（`SESSION_KEEPALIVE_TOO_AGGRESSIVE` 已展示 SSH 相关码注册）

**Adapt：** 2 条 `SSH_KNOWN_HOSTS_CONFLICT` (Warn) + `SSH_SSHD_KEEPALIVE_DRIFT` (Warn)。Message 模板：
- `"~/.ssh/known_hosts 中 %s 的 fingerprint 与本次握手不一致"` / `"运行 claudedock doctor ssh --fix 自动 ssh-keygen -R"`
- `"远端 sshd ClientAlive 配置 (%s) 与基线 (15/8) 不一致"` / `"重建容器以恢复基线（参考 deploy/docker/managed-user/sshd_config）"`

### 1.4 `internal/claudedock/errcodes/auth.go`

**Role:** AUTH_* 错误码注册（CLI 配置 / Entry token / OAuth refresh）  
**Closest analog:** `errcodes/mount.go` + `errcodes/net.go:5-25`（NET_OAUTH_* 三态展示了与 auth 同语义的 Severity 选择）

**Adapt（CONTEXT D-21）：** 4 条
- `AUTH_CONFIG_MISSING` (Fatal) — Message `"~/.claudedock/config.yaml 不存在或解析失败: %s"`，NextAction `"运行 claudedock init 重新配置网关与凭证"`
- `AUTH_GATEWAY_UNREACHABLE` (Error) — Message `"网关 %s 不可达: %s"`，NextAction `"检查网络与 gateway 配置，或运行 claudedock init"`
- `AUTH_TOKEN_EXPIRED` (Warn) — Message `"Entry API token 已过期或 401: %s"`，NextAction `"运行 claudedock doctor auth --fix 自动刷新"`
- `AUTH_OAUTH_REFRESH_FAILED` (Error) — Message `"Claude OAuth 刷新失败: %s"`，NextAction `"在容器内运行 claudedock exec claude login 重新登录"`（**字面量与 `errcodes/net.go:10` `NET_OAUTH_EXPIRED.NextAction` 完全一致**，RESEARCH §9 第 7 条）

### 1.5 `internal/claudedock/errcodes/disk.go`

**Role:** DISK_* 错误码注册（本地 / 容器 disk usage）  
**Closest analog:** `errcodes/mount.go`

**Adapt：** 3 条 `DISK_LOCAL_LOW` (Warn) / `DISK_CONTAINER_LOW` (Warn) / `DISK_MUTAGEN_DATA_BLOAT` (Warn)。阈值（500MB / 100MB / 1GB）按 CONTEXT Discretion §5 硬编码到 `doctor/disk.go`，**不**进 errcodes 文案。

---

### 1.6 `internal/claudedock/errcodes/explanations.go`

**Role:** `claudedock explain` 长说明数据源 + ExplainExempt 豁免集合  
**Closest analog:** **无完美对应**，但**结构必须复刻 `mount.go` 的 `func init()` 防御机制**，避免重复 key panic。

**Verbatim 模式（自创但模仿 `MustRegister` 的两段式 panic）：**

```go
// internal/claudedock/errcodes/explanations.go (Plan 01 新建模板)
package errcodes

import (
	"fmt"
	"sync"
)

// ExplainExempt 是登记的「不需要长说明」码集合（CONTEXT D-02 第 7 项）：
// 仅限 informational 类（M13 降级提示 / APFS 识别 / *_BACKOFF / *_NOTIFIED）。
var (
	explainMu            sync.RWMutex
	ExtendedExplanations = map[Code]string{}
	ExplainExempt        = map[Code]struct{}{
		MOUNT_APFS_CASE_INSENSITIVE: {},
		MOUNT_AUTO_DOWNGRADED:       {},
		SESSION_TAKEOVER_NOTIFIED:   {},
		NET_RECONNECT_BACKOFF:       {},
	}
)

// registerExplanation 与 MustRegister 同语义防御重复注册；
// init() 调用方式与 mount.go:8 一致。
func registerExplanation(c Code, text string) {
	if text == "" {
		panic(fmt.Sprintf("errcodes: code %q ExtendedExplanations 不能为空", c))
	}
	explainMu.Lock()
	defer explainMu.Unlock()
	if _, exists := ExtendedExplanations[c]; exists {
		panic(fmt.Sprintf("errcodes: 重复注册 ExtendedExplanations %q", c))
	}
	ExtendedExplanations[c] = text
}

func init() {
	registerExplanation(MOUNT_MUTAGEN_VERSION_SKEW, `触发场景：claudedock 客户端 embed 的 Mutagen 二进制版本与容器内 /etc/claudedock/mutagen.version 不匹配。
根本原因：Mutagen agent / client 协议版本必须严格一致，否则 sync session 创建会握手失败。
复现方式：docker exec <ctr> sed -i 's/v0.18.1/v0.99.99/' /etc/claudedock/mutagen.version
修复路径：升级容器镜像到 v3.0.0+（含 mutagen v0.18.1 agent），或重装 claudedock；也可运行 claudedock doctor mount --fix 自动重启 daemon 复测。
关联文档：.planning/research/PITFALLS.md C4`)
	// ... 其它 ≥17 条
}
```

**Verbatim**：
- 防御性 panic 三段式（`text==""` / 重复 key）= 直接抄 `codes.go:60-77` `MustRegister` 框架
- 用模板 5 段（触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档）= **CONTEXT D-18 字面格式**
- `init()` 注册风格与 `mount.go:7` 完全一致

**Adapt：**
- 全部 17 条新码 + 既有 25 条非 informational 码 = 至少 ~30 条注册（CONTEXT D-29 Plan 01 要求遍历 Registry 覆盖）
- 每条 ≥ 200 中文字符（CONTEXT D-18 + RESEARCH §6.2 `TestAllCodesHaveExplanations`）
- `ExplainExempt` 初始 4 条（如上代码块），其它新增 `*_TIMEOUT` / `*_BACKOFF` 类按 `Severity == SeverityInfo` 显式登记

---

### 1.7 `internal/claudedock/errcodes/explanations_test.go`

**Role:** map 完备性 + Registry 覆盖断言  
**Closest analog:** `internal/claudedock/errcodes/codes_test.go:10-47` `TestErrcodesRegistry`

**Verbatim copy（结构 + 遍历 Registry 写法）：**

```10:35:internal/claudedock/errcodes/codes_test.go
func TestErrcodesRegistry(t *testing.T) {
	reg := Registry()

	if len(reg) < 15 {
		t.Fatalf("注册表条目不足：want >= 15, got %d", len(reg))
	}

	re := regexp.MustCompile(`^[A-Z]+_[A-Z]+_[A-Z0-9]+(_[A-Z0-9]+)*$`)

	seen := map[Code]struct{}{}
	for code, e := range reg {
		if _, dup := seen[code]; dup {
			t.Errorf("发现重复 code: %s", code)
		}
		seen[code] = struct{}{}
		...
	}
}
```

**Adapt：** 新建 `explanations_test.go`，以同样的 `for code, e := range Registry()` 骨架写 RESEARCH §6.2 的 3 个新断言：
- `TestAllCodesHaveExplanations` — 每个 Code 在 `ExtendedExplanations` 或 `ExplainExempt` 中至少出现一次；非豁免码 `utf8.RuneCountInString(exp) >= 200`
- `TestNoLegacyLowercaseCodes` — 5 个 v2.0 lower-case 字面量（`auth_failed` / `auth_expired` / `entry_token_expired` / `host_action_failed` / `entry_config_missing`）禁出现
- `TestAllDomainsClosed` — 8 域闭合 `{MOUNT,SESSION,NET,STATE,SYSTEM,SSH,AUTH,DISK}`

### 1.8 （扩展）`internal/claudedock/errcodes/codes_test.go`

**Role:** 既有断言不动（`>= 15` 数下限放宽到 `>= 30`）+ 追加 1.7 中三个测试到同文件或独立 `explanations_test.go`（推荐独立，单测文件不与既有 PR diff 噪音耦合）。

**Verbatim**：保留 `TestErrcodesRegistry` / `TestFormat_Render` / `TestFormat_UnknownCode` / `TestLookup_Hit` / `TestLookup_Miss` 不动；只把 `len(reg) < 15` 的 `15` 改成 `30`（CONTEXT D-21 末句「17 条新 + 25 条既有 = 42 条」）。

---

## 2. doctor 子包（Plan 02 + Plan 03 — Wave 2/3）

### 2.1 `internal/claudedock/doctor/doctor.go`

**Role:** 顶层入口 / Options struct / Report struct / Status enum / RunDoctor  
**Closest analog:** `internal/claudedock/ssh_doctor.go:74-94` `RunSSHDoctor`

**Verbatim copy（控制流框架）：**

```74:94:internal/claudedock/ssh_doctor.go
func RunSSHDoctor(cfg SSHConfig, opts SSHDoctorOptions) (*SSHDoctorResult, error) {
	conn, err := sshConnect(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	raw, err := runSSHSession(conn, scanScript)
	if err != nil {
		return nil, fmt.Errorf("远端扫描失败: %w", err)
	}

	result := parseScanOutput(raw)
	result.FixMode = opts.Fix
	if opts.Fix {
		applyFixes(conn, result)
	}
	return result, nil
}
```

**Adapt：**
- 改名为 `RunDoctor(ctx context.Context, opts Options) (*Report, error)`，签名增加 `context.Context`（CONTEXT Established Patterns「`context.WithTimeout` + errgroup」）
- SSH conn **lazy** 建立（CONTEXT D-20）：第一次需要远端的 check 之前才 `sshConnect`，连不上时所有 `RequiresRemote=true` 的 check 标 `StatusSkip`，**不**让一次远端断开拖死整轮
- `Options` struct 字段（CONTEXT D-01）：`Domain string` / `Fix bool` / `Verbose bool` / `JSON bool` / `NoColor bool` / `Yes bool` / `CheckTimeout time.Duration`
- `Report` struct 字段（RESEARCH §5.1）：`SchemaVersion int` 硬编码 1（不带 omitempty）+ `StartedAt time.Time` + `DurationMS int64` + `CloudClaudeVer/RemoteImageVer string` + `DowngradeHistory *DowngradeBanner` + `Summary Summary` + `Checks []Check`，**JSON tag 严格按 RESEARCH §5.1 表逐字符照抄**
- `Status` 是 string 枚举（不是 int），值 `"pass"|"warn"|"fail"|"skip"`（RESEARCH §5.1 + CONTEXT D-15）

### 2.2 `internal/claudedock/doctor/check.go`

**Role:** Check struct + Checker interface + runWithTimeout helper  
**Closest analog:** `internal/claudedock/ssh_doctor.go:18-30` `FileReport` struct

**Verbatim copy（FixApplied/FixFailed 字段）：**

```18:30:internal/claudedock/ssh_doctor.go
type FileReport struct {
	Path       string
	Kind       string
	Owner      string
	Mode       string
	FirstLine  string
	OwnerOK    bool
	ModeOK     bool
	PEMEndsNL  *bool
	FixApplied []string
	FixFailed  []string
}
```

**Verbatim**：`FixApplied []string` / `FixFailed []string` 两字段语义 + 命名 + 类型完全保留，render 阶段 `printGroup` 用同样的循环（见 §2.10）。

**Adapt：**
- 新 struct `Check{Domain, Name string; Status string; Code errcodes.Code; Message, NextAction string; Details map[string]any; FixApplied, FixFailed []string; DurationMS int64}`，JSON tag 按 RESEARCH §5.1 第二张表
- 新 interface `Checker interface { Run(ctx context.Context, runner RemoteRunner) Check; Fix(ctx context.Context, opts Options) Check }`
- 新 helper `runWithTimeout(ctx, name string, timeout time.Duration, fn func(context.Context) Check) Check` — 用 `context.WithTimeout` 包装；命中 timeout 返回 `Status="fail"` + `Code=SYSTEM_CHECK_TIMEOUT`（CONTEXT D-08）

### 2.3 `internal/claudedock/doctor/network.go`

**Role:** dns_resolve / gateway_reachable / egress_ip_visible 三 check  
**Closest analog:** `internal/claudedock/envcheck.go:30-65` `RunEnvCheck`（已实现远端 `cat /etc/os-release` / DNS / IP 等命令的 `run` helper 模式）

**Verbatim copy（envcheck 的 run helper 模式 + bytes.Buffer 收集 stdout）：**

```37:53:internal/claudedock/envcheck.go
run := func(cmd string) string {
	sess, err := conn.NewSession()
	if err != nil { return "(session error)" }
	defer sess.Close()
	var buf bytes.Buffer
	sess.Stdout = &buf
	if err := sess.Run(cmd); err != nil {
		out := strings.TrimSpace(buf.String())
		if out != "" { return out }
		return "(unavailable)"
	}
	return strings.TrimSpace(buf.String())
}
```

**Adapt：**
- `dns_resolve`：纯本地 `net.LookupHost(host)` + 包级 var `var lookupHost = net.LookupHost` 便于单测注入（包级 var mock 模式 §2.9）
- `gateway_reachable`：`http.NewRequestWithContext` + 2s timeout（**不**复用 `EntryClient.AuthenticateAndWait`，避免误报 token 过期成网络问题，RESEARCH §3.1）
- `egress_ip_visible`：远端 `curl -sS --max-time 5 https://ifconfig.io` 走 `RemoteRunner.RunScript`（§2.8）

### 2.4 `internal/claudedock/doctor/auth.go`

**Role:** config_present / entry_token_valid / oauth_credentials 三 check  
**Closest analog:** `internal/claudedock/oauth_check.go:44-63` `CheckOAuthCredentials`

**Verbatim copy（oauth_check 已就位的三态 switch）：**

```44:63:internal/claudedock/oauth_check.go
func CheckOAuthCredentials(connA *ssh.Client, claudeAccountID string) (*OAuthStatus, error) {
	_ = claudeAccountID
	if connA == nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	sess, err := connA.NewSession()
	if err != nil {
		return &OAuthStatus{State: OAuthNotFound}, nil
	}
	defer sess.Close()
	var stdout bytes.Buffer
	sess.Stdout = &stdout
	cmd := "timeout 2 cat /home/claude/.claude/.credentials.json 2>/dev/null"
	_ = sess.Run(cmd)
	return parseExpiresAt(stdout.String(), time.Now()), nil
}
```

**Verbatim**：直接 `import "github.com/claudedock/claudedock/internal/claudedock"` 后调 `claudedock.CheckOAuthCredentials(connA, accountID)`，**绝对禁止重写 OAuth 解析逻辑**（C-7 防御）。

**Adapt：**
- `config_present`：调 `claudedock.LoadConfig()` 失败 → `Status="fail"` + `Code=AUTH_CONFIG_MISSING`
- `entry_token_valid`：调 `EntryClient.AuthenticateAndWait(ctx, ..., func(string){})`（dry-run callback），HTTP 200 = pass；写 `Check.Details["image_version"] = authResp.ImageVersion`（render banner 复用）
- `oauth_credentials`：switch `OAuthStatus.State` 映射码：
  - `OAuthValid` → pass
  - `OAuthExpiringSoon` → warn + `NET_OAUTH_EXPIRING_SOON`
  - `OAuthExpired` → fail + `NET_OAUTH_EXPIRED`
  - `OAuthNotFound` → fail + `NET_OAUTH_NOT_FOUND`

### 2.5 `internal/claudedock/doctor/ssh.go`

**Role:** keepalive_config / sshd_keepalive_drift / known_hosts / workspace_ssh_keys 四 check  
**Closest analog:** `internal/claudedock/ssh_doctor.go:74-94` `RunSSHDoctor`（直接调用，**不**重写）

**Verbatim**（workspace_ssh_keys check 的实现 = 一行 wrap 调用）：

```go
// internal/claudedock/doctor/ssh.go (Plan 02 实现模板)
func checkWorkspaceSSHKeys(sshCfg claudedock.SSHConfig) Check {
	res, err := claudedock.RunSSHDoctor(sshCfg, claudedock.SSHDoctorOptions{Fix: false})
	if err != nil {
		return Check{Domain: "ssh", Name: "workspace_ssh_keys", Status: "fail",
			Message: fmt.Sprintf("RunSSHDoctor 失败: %v", err)}
	}
	// 无错误码（D-19 字面：本阶段不强迁移）；details 携带摘要供 --verbose / JSON 消费
	return Check{Domain: "ssh", Name: "workspace_ssh_keys",
		Status:  summarizeSSHDoctor(res),  // pass/warn/fail by problemCount
		Details: map[string]any{"files": len(res.Files), "missing": res.Missing}}
}
```

**Adapt：**
- `keepalive_config`：纯本地读 `MountConfig.KeepAliveInterval` ≥ 15s（CONTEXT D-19）
- `sshd_keepalive_drift`：远端 `sshd -T | grep -E '^(clientalive(interval|countmax))\b'`（RESEARCH §3.3 + Phase 29 D-14 基线 15/8）
- `known_hosts`：用 `import "golang.org/x/crypto/ssh/knownhosts"` 的 `New(files...) → HostKeyCallback` 做 fake handshake 判冲突（RESEARCH §3.3 + 关键 gotcha：HASHED hostname 不能字符串匹配）

### 2.6 `internal/claudedock/doctor/mount.go`

**Role:** mutagen_version_match / mergerfs_branches / sshfs_mountpoint / fuse_residual / apparmor_fusermount3 五 check  
**Closest analog:** `internal/claudedock/mount_mutagen.go:230-237`（版本握手已现网） + `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3`（shell 改 Go）

**Verbatim copy（Mutagen 版本握手三行）：**

```231:237:internal/claudedock/mount_mutagen.go
// 3) 版本握手
if connA != nil {
	remoteVer, _ := deps.remoteVersion(connA)
	if remoteVer != "" && !strings.Contains(remoteVer, strings.TrimPrefix(MutagenBinaryVersion, "v")) {
		return nil, MutagenSyncStatus{}, newMutagenErr(errcodes.MOUNT_MUTAGEN_VERSION_SKEW, MutagenBinaryVersion, remoteVer)
	}
}
```

**Verbatim**：`TrimPrefix("v") + Contains` 双保险（RESEARCH §8.2 — 远端文件无 `v` 前缀），把 newMutagenErr 替换成 `Check{Status:"fail", Code: errcodes.MOUNT_MUTAGEN_VERSION_SKEW, Details: {...}}`。

**Adapt（mergerfs branches）：**
- 走 `RemoteRunner.RunScript("getfattr_branches", "getfattr --only-values -n user.mergerfs.branches /workspace/.mergerfs 2>/dev/null")`
- 用 RESEARCH §8.1 的 6 字面量 `strings.Contains` 循环（**禁止** 正则匹配整行）：
  ```go
  want := []string{
      "func.readdir=cor:4", "cache.attr=30", "cache.entry=30",
      "cache.readdir=true", "cache.files=off", "category.create=ff",
  }
  ```

**Adapt（apparmor_fusermount3）：** Go 改写 `deploy/scripts/host-preflight.sh:check_apparmor_fusermount3`（RESEARCH §8.3 已给出 5-Gate 伪代码）；纯文件读取，**不** exec `apparmor_parser`。

### 2.7 `internal/claudedock/doctor/disk.go`

**Role:** local_disk / container_disk / mutagen_data_size 三 check  
**Closest analog:** **无**（Statfs 是新引入的）；最近的远端命令 wrap 模式见 `envcheck.go:55-65`

**Verbatim 模式（用 `golang.org/x/sys/unix` 抹平 Linux/Darwin Statfs_t 签名差异）：**

```go
// internal/claudedock/doctor/disk.go (RESEARCH §3.5 模板)
import "golang.org/x/sys/unix"

func checkLocalDisk(path string) Check {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return Check{Domain: "disk", Name: "local_disk", Status: "fail",
			Code: errcodes.DISK_LOCAL_LOW, Message: err.Error()}
	}
	availMB := int64(stat.Bavail) * int64(stat.Bsize) / 1024 / 1024
	switch {
	case availMB < 100:
		return failWith(errcodes.DISK_LOCAL_LOW, availMB)
	case availMB < 500:
		return warnWith(errcodes.DISK_LOCAL_LOW, availMB)
	}
	return pass("disk", "local_disk")
}
```

**Adapt：** 阈值硬编码 500MB / 100MB / 1GB（CONTEXT Discretion §5），不进 config。`golang.org/x/sys` 已在间接依赖（`go.sum` via `crypto/ssh`），无需 `go get`。

### 2.8 `internal/claudedock/doctor/remote_runner.go`

**Role:** SSH 远端命令执行包装 + 单测 mock 注入点  
**Closest analog:** `internal/claudedock/ssh_doctor.go:96-117` `runSSHSession`

**Verbatim copy（runSSHSession 的 stdout/stderr 收集 + error 包装）：**

```96:117:internal/claudedock/ssh_doctor.go
func runSSHSession(conn *ssh.Client, script string) (string, error) {
	sess, err := conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(script); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("%w (%s)", err, msg)
	}
	return stdout.String(), nil
}
```

**Adapt：**
- 包装为 `RemoteRunner` interface（CONTEXT D-20 + RESEARCH §9 第 1 条）：
  ```go
  type RemoteRunner interface {
      RunScript(name, script string) (stdout, stderr string, err error)
  }
  type sshRemoteRunner struct{ conn *ssh.Client }
  func (r *sshRemoteRunner) RunScript(name, script string) (string, string, error) {
      // 把上面 runSSHSession 的 stdout/stderr/err 三元组返回，name 仅用于 --verbose 日志
  }
  ```
- 单测里 mock：`type fakeRunner struct{ scripts map[string]struct{ out, errs string; err error }}` — 与 `ssh_doctor_test.go` 风格一致

### 2.9 `internal/claudedock/doctor/fix.go`

**Role:** FixerRegistry + 5 类自动修复 + confirmDestructive helper  
**Closest analog:** 包级 var mock 模板 = `internal/runtime/tasks/worker.go:752-760`（execInContainer）+ `:957-984`（dockerVolumeRunner / ensureDockerVolume）

**Verbatim copy（包级 var mock 模板，逐字符照搬）：**

```752:760:internal/runtime/tasks/worker.go
// execInContainer 在目标容器中以 `docker exec -i <container> bash -c <script>` 执行，
// 支持可选 stdin。暴露为 package-level 变量以便单元测试注入 fake。
var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}
```

```957:969:internal/runtime/tasks/worker.go
// dockerVolumeRunner 抽象 docker volume 子命令的实际执行；包级 var 便于单元测试注入 mock。
// 与 var execInContainer = ... 模式一致。
var dockerVolumeRunner = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"volume"}, args...)...)
	return cmd.CombinedOutput()
}

var ensureDockerVolume = realEnsureDockerVolume

func realEnsureDockerVolume(ctx context.Context, name string, labels map[string]string) error {
```

**Verbatim**：包级 `var XxxRunner = realXxxRunner` + `realXxxRunner(...) error` 一对的模式 = Phase 29.1 / Phase 33 验证过的官方模板。**注释里必须写「暴露为 package-level 变量以便单元测试注入 fake」字面量** —— 与 worker.go:753 / :957 一致，便于 grep。

**Adapt：** 5 个包级 var（与 RESEARCH §2.5 + §4.1-§4.5 一一对应）：

```go
// internal/claudedock/doctor/fix.go (Plan 03 模板)
var execMutagenDaemon     = realExecMutagenDaemon      // §4.1 daemon stop/start
var execFusermountUnmount = realExecFusermountUnmount  // §4.2 跨 OS fusermount/umount
var execSSHKeygenRemove   = realExecSSHKeygenRemove    // §4.3 ssh-keygen -R
var execEntryRefresh      = realExecEntryRefresh       // §4.4 wraps EntryClient.AuthenticateAndWait
var execDNSFlush          = realExecDNSFlush           // §4.5 macOS dscacheutil / Linux resolvectl
```

**复用既有幂等容错**（mount_mutagen.go:227 已实现）：

```226:229:internal/claudedock/mount_mutagen.go
out, derr := deps.runLocal(binPath, []string{"daemon", "start"}, env)
if derr != nil && !strings.Contains(out, "daemon already started") && !strings.Contains(out, "already running") {
	return nil, MutagenSyncStatus{}, newMutagenErr(errcodes.MOUNT_MUTAGEN_DAEMON_UNAVAILABLE, derr.Error())
}
```

**Adapt（confirmDestructive）：** RESEARCH §9 第 2 条 — `term.IsTerminal(int(os.Stdin.Fd()))` 走非 TTY 拒绝（`golang.org/x/term` 已在 main.go:10 import）；JSON 模式直接拒绝（CONTEXT D-10 第 2 条）；`opts.Yes==true` 直接 return true（CI 友好）。

### 2.10 `internal/claudedock/doctor/render.go`

**Role:** Text + JSON 双输出，Banner / printGroup / 状态符号 / NO_COLOR  
**Closest analog:** `internal/claudedock/ssh_doctor.go:370-453` `Print` + `printGroup` + `colors.go:25-48` `colorEnabled`/`colorize`

**Verbatim copy（banner 框 + section + 状态符号循环）：**

```370:409:internal/claudedock/ssh_doctor.go
func (r *SSHDoctorResult) Print() {
	fmt.Println("╭─────────────────────────────────────────╮")
	fmt.Println("│    Cloud Claude SSH 密钥体检报告        │")
	fmt.Println("╰─────────────────────────────────────────╯")
	fmt.Println()
	section("目标")
	row("远端用户", r.User)
	row("SSH 目录", r.SSHDir)
	...
	printGroup("私钥", r.Files, "private")
	printGroup("公钥", r.Files, "public")
	...
	summarize(r)
}
```

```412:454:internal/claudedock/ssh_doctor.go
func printGroup(title string, files []FileReport, kind string) {
	var subset []FileReport
	for _, f := range files { ... }
	section(title)
	for _, f := range subset {
		icon := "✓"
		if !fileOverallOK(f) { icon = "✗" }
		fmt.Printf("  %s  %s  %s\n", icon, f.Path, strings.Join(parts, "  "))
		for _, fx := range f.FixApplied {
			fmt.Printf("       ✓ 已修复: %s\n", fx)
		}
		for _, fx := range f.FixFailed {
			fmt.Printf("       ✗ 修复失败: %s\n", fx)
		}
	}
}
```

**Verbatim**：
- Banner 框 `╭─╮│╰─╯` 字符 = ssh_doctor.go:371-373，标题改为 `"  Cloud Claude Doctor v3.0 体检报告  "`
- `section()` / `row()` helper 内联调用模式不动
- `       ✓ 已修复: %s\n` / `       ✗ 修复失败: %s\n` 缩进字面量（7 空格）= ssh_doctor.go:447-451 完全保留
- 颜色 helper：直接 `import` `claudedock` 内的 `colorEnabled`/`colorize` —— 但因为是新包，需要把这俩 helper **导出**（提交 Plan 02 task：把 `colors.go:29` `colorEnabled` 改首字母大写 `ColorEnabled`，同时改全部调用点）；**或者**在 doctor 包内重新声明同名包内 helper（CONTEXT 要求「不引入新颜色 helper」推荐前者）

**Adapt（4 状态符号 + 颜色）：**

```go
// internal/claudedock/doctor/render.go (CONTEXT D-14)
const (
	iconPass = "[✓]"  // ansiGreen 32
	iconWarn = "[!]"  // ansiYellow 33
	iconFail = "[✗]"  // ansiRed 31
	iconSkip = "[~]"  // ansiGray 90
)
// NO_COLOR=1 或非 TTY → 退回 [ok]/[warn]/[fail]/[skip]（claudedock.colors.go 已自动处理）
```

**Adapt（D-13 第一屏降级历史 banner）：** 新增 `renderDowngradeBanner(snap *claudedock.LastSessionSnapshot) string`，每条 DowngradeStep 输出一行 `[降级] <from> → <to> | 原因 [<reason_code>] <reason_message>`（M13 验收锚点字面量，RESEARCH §8.5 测试断言依赖此字串）。

**Adapt（JSON 输出 schema_version=1 锁死，RESEARCH §5.1）：**
- `RenderJSON(*Report) []byte` 用 `json.MarshalIndent(r, "", "  ")` 默认 indent 2（CONTEXT Discretion §6）
- `SchemaVersion int` JSON tag **不带 omitempty** —— jq 探针稳定 assert
- 必有测试 `TestJSONSchemaV1Lock`（RESEARCH §5.1 已给出伪代码）

### 2.11 `internal/claudedock/doctor/explain_index.go`（optional）

**Role:** `RelatedChecks map[Code][]string` — explain --verbose 关联 doctor check 名  
**Closest analog:** `errcodes/explanations.go`（同包级 map + init 注册）

**Verbatim 模式：** 与 1.6 节 `ExtendedExplanations` 完全镜像 —— 包级 `var RelatedChecks = map[Code][]string{}` + `init()` 内 `RelatedChecks[MOUNT_MUTAGEN_VERSION_SKEW] = []string{"mount.mutagen_version_match"}` 等。**Plan 02 视实现复杂度决定是否落地（CONTEXT 标 optional）；不实现时 explain --verbose 段 3 静默跳过即可**。

### 2.12 `internal/claudedock/doctor/{network,auth,ssh,mount,disk,render,check,fix}_test.go`

**Role:** 每文件独立单测，远端 check 走 RemoteRunner mock  
**Closest analog:**
- 每维度 check 测试 = `internal/claudedock/oauth_check_test.go`（纯函数 + 三态 switch 已展示）
- fix_test.go 包级 var mock = `internal/runtime/tasks/worker_test.go`（搜 `execInContainer = func(...)` 注入点）
- render_test.go = `internal/claudedock/ssh_doctor_test.go`（`Print()` 风格测试可参考其 stdout 捕获）

**Verbatim 模式（包级 var mock 注入）：**

```go
// internal/claudedock/doctor/fix_test.go (Plan 03)
func TestFixMutagenDaemon_Idempotent(t *testing.T) {
	calls := 0
	original := execMutagenDaemon
	execMutagenDaemon = func(ctx context.Context, action string) error {
		calls++
		return nil  // 模拟成功
	}
	defer func() { execMutagenDaemon = original }()
	// ... 调 fixMutagenDaemonUnavailable(...) 两次，断言 calls==2 且都返回 nil（idempotent）
}
```

**Adapt：** RemoteRunner mock 与生产同接口：

```go
type fakeRunner struct{ out, errs string; err error }
func (f *fakeRunner) RunScript(name, script string) (string, string, error) {
	return f.out, f.errs, f.err
}
```

### 2.13 `internal/claudedock/doctor/integration_test.go`

**Role:** docker fixture 1 happy + 1 fail injection  
**Closest analog:** `internal/claudedock/integration_test.go:1-119`（build tag + TestMain + dockerExec helper）

**Verbatim copy（build tag + TestMain + dockerExec 三段式必须照抄）：**

```1:14:internal/claudedock/integration_test.go
//go:build integration
// +build integration

// Phase 31 Plan 03 集成测试套件。
//
// 默认 `go test ./...` 不触发本文件（受 build tag `integration` 隔离）；
// 完整执行需：
//
//	bash scripts/test-fixture-up.sh   # 起 Phase 29 镜像
//	go test -tags=integration -count=1 -v ./internal/claudedock/doctor/
//	bash scripts/test-fixture-down.sh
```

```60:80:internal/claudedock/integration_test.go
func TestMain(m *testing.M) {
	if err := exec.Command("scripts/test-fixture-up.sh").Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture 启动失败，跳过集成测试:", err)
		os.Exit(0)
	}
	code := m.Run()
	_ = exec.Command("scripts/test-fixture-down.sh").Run()
	os.Exit(code)
}

func dockerExec(t *testing.T, args ...string) (string, error) {
	t.Helper()
	full := append([]string{"exec", fixtureCtr}, args...)
	c := exec.Command("docker", full...)
	var out bytes.Buffer
	c.Stdout = &out
	c.Stderr = &out
	err := c.Run()
	return out.String(), err
}
```

**Verbatim**：
- 1-2 行 build tag `//go:build integration` + `// +build integration` 必须**精确双行**（go 旧编译器兼容）
- TestMain 跳过 fixture 失败的 `os.Exit(0)` 模式 = 本地 dev 无 docker 不阻塞 unit test（**CONTEXT D-24 第 3 条 + RESEARCH §6.1 CI 假设**）
- 复用现有 `scripts/test-fixture-up.sh` / `test-fixture-down.sh`（RESEARCH §7.1）—— **禁止**新建 docker compose fixture

**Adapt：** 2 个测试用例（CONTEXT D-29 Plan 03 + Specifics §3 / §4）：
- `TestIntegration_DoctorMountHappy`：v3 镜像 + claudedock doctor mount → 5 项全 pass，退出码 0
- `TestIntegration_DoctorMountFail_MergerfsTampered`：`docker exec <ctr> umount /workspace && mount -o cor:1 ...` 篡改 readdir 参数 → doctor mount 输出含 `MOUNT_MERGERFS_FAILED` + next_action 含字面量 `claudedock doctor mount --fix`（SC#7 锚点）

---

## 3. cobra 子命令（Plan 01 / Plan 02 — Wave 1/2）

### 3.1 `cmd/claudedock/doctor.go`

**Role:** `doctor [domain]` cobra 子命令 + `--fix|--verbose|--json|--yes` flag + 退出码 0/1/2  
**Closest analog:** `cmd/claudedock/sessions.go:22-47` `newSessionsCmd`（三段式 outer + sub + RunE）

**Verbatim copy（cobra 三段式 + SilenceUsage/SilenceErrors）：**

```22:47:cmd/claudedock/sessions.go
func newSessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "sessions",
		Short:         "tmux 会话管理（v3.0 SSH 会话可靠性）",
		Long:          "查看 / attach 容器内由 claudedock 创建的 tmux 会话；零控制面改造，纯客户端逻辑。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	lsCmd := &cobra.Command{
		Use:           "ls",
		Short:         "列出当前 tmux 会话",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runSessionsLs,
	}
	...
	cmd.AddCommand(lsCmd, attachCmd)
	return cmd
}
```

**Verbatim**：`SilenceUsage: true` + `SilenceErrors: true` 是 claudedock 全 cobra cmd 的统一字段（避免 error 时打印 cobra default usage，干扰 --json 输出）—— 必须保留。

**Adapt：**
- `Use: "doctor [domain]"` + `Args: cobra.MaximumNArgs(1)` + `ValidArgs: []string{"network","auth","ssh","mount","disk","all"}`（CONTEXT D-05）
- 4 flag 注册：`cmd.Flags().Bool("fix", false, "...")` 等 4 个
- 文件级 const：`doctorExitOK=0 / doctorExitWarn=1 / doctorExitFail=2`（RESEARCH §5.2 — **不**新增 `exitcodes.go` 常量，子命令各自 `os.Exit`）
- `RunE` 内调 `claudedock/doctor.RunDoctor(ctx, opts)` → 按 `Report.Summary` 计算 exit code → `os.Exit(code)`

**main.go 注册（同时 Plan 02 task）：**
- `cmd/claudedock/main.go:93` 的 `rootCmd.AddCommand(...)` 行追加 `newDoctorCmd()`：
  ```go
  rootCmd.AddCommand(initCmd, envCmd, sshCmd, newSyncCmd(), newSessionsCmd(), newDoctorCmd(), newExplainCmd())
  ```
- `cmd/claudedock/main.go:99` 的 `switch os.Args[1]` case 追加 `"doctor", "explain"`（CONTEXT D-03）

### 3.2 `cmd/claudedock/explain.go`

**Role:** `explain <code>` cobra 子命令 + 三段输出 + exit 4 未找到  
**Closest analog:** `cmd/claudedock/sessions.go:30-44`（attachCmd 三字段 + Args ExactArgs(1)）

**Verbatim copy（ExactArgs(1) + RunE 模板）：**

```37:44:cmd/claudedock/sessions.go
attachCmd := &cobra.Command{
	Use:           "attach <name>",
	Short:         "attach 到指定 tmux 会话",
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runSessionsAttach,
}
```

**Adapt：**
- `Use: "explain <code>"` + `Args: cobra.ExactArgs(1)`
- 大小写敏感匹配 `errcodes.Lookup(errcodes.Code(args[0]))`（CONTEXT D-17 + Pitfall §8.4 — 禁止自动大小写转换）
- 三段输出（CONTEXT D-17）：
  1. `errcodes.Format(code)` 两段（沿用现网字面量，与 doctor 输出严格对齐）
  2. 空行 + `"详细说明："` + `errcodes.ExtendedExplanations[code]` or fallback 字符串
  3. `--verbose` 时追加 RelatedChecks（如有）
- 未找到 → `fmt.Fprintln(os.Stderr, "未找到错误码 ...")` + `os.Exit(exitConfigError)` (= 4)

### 3.3 `cmd/claudedock/explain_test.go`

**Role:** os/exec 子进程级测试（构建 claudedock 二进制后跑 explain 命令）  
**Closest analog:** `internal/claudedock/integration_test.go:86-111` `runCloudClaude` helper

**Verbatim copy（go build + exec.CommandContext + ExitError 断言）：**

```86:111:internal/claudedock/integration_test.go
func runCloudClaude(t *testing.T, mode string, cwd string) (exitCode int, stderr string) {
	t.Helper()
	bin := "/tmp/claudedock-int"
	if _, err := os.Stat(bin); err != nil {
		if err := exec.Command("go", "build", "-o", bin, "./cmd/claudedock").Run(); err != nil {
			t.Fatalf("编译 claudedock 失败: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
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
		t.Logf("claudedock 执行错误: %v", err)
		return -1, stderrBuf.String()
	}
	return 0, stderrBuf.String()
}
```

**Verbatim**：
- `go build -o /tmp/claudedock-explain-test ./cmd/claudedock` 缓存复用模式
- `ExitError.ExitCode()` 类型断言 = exit code 提取标准写法
- `t.TempDir()` cwd 切换 + `c.Dir = cwd` 隔离

**Adapt：** 测试用例（**不需要** docker fixture，纯 explain 子命令）：
- `TestExplain_KnownCode_Exit0_StdoutContainsCode` — 跑 `claudedock explain MOUNT_MUTAGEN_VERSION_SKEW`，断言 exit 0 + stdout 含错误码字面量 + 含 `"建议:"`
- `TestExplain_UnknownCode_Exit4` — 跑 `claudedock explain FAKE_CODE_X`，断言 exit 4 + stderr 含「未找到」
- 用 `exec.Command(bin, "explain", code)` —— **不带** `--mount-mode` flag

---

## 4. CI 脚本（Plan 03 — Wave 3 收尾）

### 4.1 `scripts/ci-doctor-grep.sh`

**Role:** M14 终验脚本 — `claudedock doctor --json` jq 校验 + 文本 grep 三段断言  
**Closest analog:** `scripts/test-fixture-up.sh`（bash 头部 + set -euo pipefail + 错误退出码模式）

**Verbatim copy（bash header / 退出码 / 依赖检查模式）：**

```1:23:scripts/test-fixture-up.sh
#!/usr/bin/env bash
# Phase 31 Plan 03 集成测试 fixture：
# 起 Phase 29 镜像容器作为 claudedock integration test fixture。
#
# 用法：scripts/test-fixture-up.sh
#
# 退出码：
#   0  → fixture 已起且 sshd 就绪
#   1  → 缺少依赖、镜像未构建、或 30s 内 sshd 未就绪
#
# 幂等：重复执行会先 docker compose up -d；docker compose 自身保证 idempotent。
set -euo pipefail

FIXTURE_DIR="/tmp/claudedock-fixture"
IMAGE="local/managed-user:v3.0.0"

command -v docker >/dev/null || { echo "需要 docker"; exit 1; }
```

**Verbatim**：
- `#!/usr/bin/env bash` shebang + 5-10 行注释（用法 / 退出码 / 幂等）= scripts/ 全脚本统一头部
- `set -euo pipefail`（行 17）必须保留
- `command -v jq >/dev/null || { echo "需要 jq"; exit 1; }` 依赖检查模式 = test-fixture-up.sh:22 同款

**Adapt：** 完整脚本已在 RESEARCH §6.1 给出（70 行），3 段 jq/grep 断言 + `mktemp -d` + `trap rm` 清理。**直接复用**那一份脚本作为 Plan 03 task 的实现，无需重新设计。

**CI 挂载位置：** Plan 03 收尾 task 把 `bash scripts/ci-doctor-grep.sh ./claudedock` 加入 `.github/workflows/ci.yml`（如有）或 Makefile target；exit code 必须传播（**不**加 `|| true`）。

---

## 5. 测试模式总览

| 测试类型 | Pattern Source | 适用文件 |
|---------|---------------|---------|
| 包级 var mock | `internal/runtime/tasks/worker.go:752-760`（execInContainer）+ `:957-984`（dockerVolumeRunner / ensureDockerVolume） | `doctor/fix_test.go`（5 个 `var execXxx`）|
| Interface mock | `RemoteRunner interface{ RunScript(...)... }` 自定义 + 单测 `fakeRunner{out, errs string, err error}` | `doctor/{network,auth,ssh,mount,disk}_test.go` |
| 纯函数 + 三态 switch | `internal/claudedock/oauth_check_test.go`（`parseExpiresAt` 三态） | `doctor/auth_test.go` oauth_credentials check |
| 子进程级测试（go build + exec.CommandContext） | `internal/claudedock/integration_test.go:86-111` `runCloudClaude` | `cmd/claudedock/explain_test.go` |
| docker fixture + build tag + t.Skip | `internal/claudedock/integration_test.go:1-80`（`//go:build integration` + TestMain + dockerExec） | `doctor/integration_test.go`（1 happy + 1 mergerfs 篡改 fail） |
| Registry 遍历断言 | `internal/claudedock/errcodes/codes_test.go:10-47` `TestErrcodesRegistry` | `errcodes/explanations_test.go`（3 个新断言：覆盖 / lower-case 禁入 / 8 域闭合） |
| stdout 捕获 + 字面量断言 | `internal/claudedock/ssh_doctor_test.go`（参考 `Print()` 输出） | `doctor/render_test.go`（含 SC#6 `TestDowngradeBannerRendersChain` + RESEARCH §5.1 `TestJSONSchemaV1Lock`） |
| CI shell 脚本（jq + grep + exit code） | `scripts/test-fixture-up.sh:17-22` `set -euo pipefail` + 依赖检查 | `scripts/ci-doctor-grep.sh`（脚本完整版见 RESEARCH §6.1）|

---

## PATTERN MAPPING COMPLETE

- 22 个新文件全部找到现网模板，每个给出 verbatim 抄写片段（带行号）+ adapt 列表（按 CONTEXT D-XX 决策点定位）。
- 5 个 errcodes 新文件 = `mount.go` 模板逐字符复刻（init + MustRegister + nolint:lll）；`explanations.go` 自创但镜像 `MustRegister` 防御 panic 模式。
- doctor 子包核心控制流 = `ssh_doctor.go` RunSSHDoctor / runSSHSession / Print / printGroup / applyFixes 四块直接借用；fix.go 包级 var mock 严格复刻 `worker.go:752-760` 与 `:957-984` 模板。
- cobra 子命令 = `sessions.go:22-47` newSessionsCmd 三段式；explain_test.go 子进程测试 = `integration_test.go:86-111` runCloudClaude；doctor 集成测试 = `integration_test.go:1-80` build tag + TestMain。
- ci-doctor-grep.sh 脚本头部 = `scripts/test-fixture-up.sh:1-23`，主体 70 行 bash 已在 RESEARCH §6.1 完成可直接复用。
