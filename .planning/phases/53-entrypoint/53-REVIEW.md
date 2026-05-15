---
phase: 53-entrypoint
reviewed: 2026-05-16T03:05:00Z
depth: deep
files_reviewed: 7
files_reviewed_list:
  - deploy/docker/managed-user/Dockerfile
  - deploy/docker/managed-user/entrypoint.sh
  - deploy/docker/managed-user/default-deny.nft
  - tests/phase53/smoke.sh
  - tests/phase53/fixtures/test-singbox-config.json
  - tests/phase53/README.md
  - Makefile
findings:
  critical: 1
  high: 3
  medium: 6
  low: 5
  total: 15
status: partial-fixed
fixed:
  - CR-01
  - HI-01
  - HI-02
  - HI-03
deferred_to_tech_debt:
  - ME-01
  - ME-02
  - ME-03
  - ME-04
  - ME-05
  - ME-06
  - LO-01
  - LO-02
  - LO-03
  - LO-04
  - LO-05
tech_debt_doc: .planning/phases/53-entrypoint/53-TECH-DEBT.md
fixed_at: 2026-05-16T03:50:00Z
---

# Phase 53 Code Review Report

**Reviewed:** 2026-05-16T03:05:00Z
**Depth:** deep（含跨文件链路追踪：Dockerfile capability ↔ entrypoint runuser ↔ smoke fixture）
**Files Reviewed:** 7
**Status:** issues_found

## Summary

Phase 53 的"sing-box 同容器 + fail-closed entrypoint + nft default-deny"骨架完整、注释充分、anti-pattern 干净，shellcheck / bash -n / nft -c 静态校验全绿。但 **adversarial 审计发现一条 CRITICAL 启动阻塞 bug 与 3 条 HIGH 级别问题**，全部位于 verifier 已声明的"deferred-to-CI 运行时 oracle"覆盖范围之外，是真正的实现缺陷而非验收缺口：

1. **CRITICAL（CR-01）**：`start_singbox_or_die` 强制 config 必须为 `root:0600`，但 sing-box 通过 `runuser -u singbox` 以 uid=9000 跑，DAC 权限模型下 uid=9000 读 `root:0600` 文件必返回 EACCES → sing-box 启动即死 → tun0 30s waitFor 超时 → 容器永远起不来。**这是直接破坏 phase goal 的功能性 bug**，CI 跑 smoke.sh 的瞬间会全部 6 条断言一起失败。
2. **HIGH（HI-01）**：`CONTAINER_USER` 环境变量未做合法性校验直接拼进 `rm -f "/etc/sudoers.d/${RUN_USER}"`，存在路径穿越（`CONTAINER_USER='../../tmp/x'` 可让 root 进程删任意路径）。
3. **HIGH（HI-02）**：smoke fixture 的 sing-box config **没有 stub DNS inbound 监听 127.0.0.1:53**，但 entrypoint `lock_resolv_conf` 已把 `/etc/resolv.conf` 钉到 `nameserver 127.0.0.1` —— 任何依赖 libc resolver 的 DNS 查询（包括 smoke T-53-3 的 `curl https://api.ipify.org`）都会因为 connect 127.0.0.1:53 ECONNREFUSED 而失败。即便 CR-01 修复，T-53-3 仍会挂。
4. **HIGH（HI-03）**：`runuser` 默认带 PAM session 会 fork 父子两进程，`SING_BOX_PID=$!` 抓到的是 runuser parent（uid=0）的 PID，导致 `ps -o uid= -p $SING_BOX_PID` 输出 `0` 而不是 `9000`，启动日志严重误导（"sing-box 已成功跑在 uid=0"）。fail-closed 语义巧合保留（runuser parent 在 child 退出时会跟着退），但代码注释与日志承诺的"uid=9000 验证"落空。

剩余 6 条 MEDIUM 与 5 条 LOW/INFO 详见下文，包含 verifier 已识别的 2 项 gap。

> **Phase 53 不应在不修 CR-01 / HI-01 / HI-02 的情况下进入 Phase 54** —— Phase 55 CI 跑 smoke.sh 会立刻把这三条暴露成红绿灯，越早修越好。

## Structural Findings (fallow)

无外部 structural findings 输入（本次 review 由 `/gsd-code-review` 直接 spawn，未走 structural pre-pass）。

## Narrative Findings (AI reviewer)

## Critical

### CR-01: sing-box 进程 uid=9000 读不了 `root:0600` 的 config，启动必失败

**File:** `deploy/docker/managed-user/entrypoint.sh:175-186`
**Issue:**
`start_singbox_or_die` 显式校验 config 必须为 `root:0600`：

```169:209:deploy/docker/managed-user/entrypoint.sh
start_singbox_or_die() {
  if [ ! -f "$SING_BOX_CONFIG" ]; then
    echo "[entrypoint] FATAL: sing-box config 不存在: $SING_BOX_CONFIG" >&2
    exit 1
  fi

  perm="$(stat -c '%a' "$SING_BOX_CONFIG")"
  owner="$(stat -c '%U' "$SING_BOX_CONFIG")"
  if [ "$perm" != "600" ] || [ "$owner" != "root" ]; then
    echo "[entrypoint] FATAL: config 权限不对（want root:0600，got ${owner}:${perm}）" >&2
    exit 1
  fi

  echo "[entrypoint] starting sing-box as uid=9000 (file-cap based)"
  runuser -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
  SING_BOX_PID=$!
```

随后通过 `runuser -u singbox -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG"` 启动，sing-box 子进程的 `geteuid()=9000`。Linux DAC 模型下 mode `0600` 表示**仅 owner（root）可读**，group 与 other 都没有 read bit。sing-box 文件 cap 只授予 `cap_net_admin+eip` 与 `cap_net_bind_service+eip`，**没有 `cap_dac_read_search` / `cap_dac_override`**，因此 sing-box `open(config, O_RDONLY)` 必然返回 EACCES，进程立即退出。

后续链路：sing-box 死 → `start_singbox_or_die` 30s `tun0` waitFor 超时 → entrypoint `exit 1` → tini 关停容器 → docker `--restart=on-failure:3` 重试 3 次后 give up。**容器永远起不来，phase goal 全部失守**。

这条 bug 没被 verifier 抓到的原因：53-VERIFICATION 把所有 docker run / docker exec 类 oracle 都标 deferred-to-CI（D-53-PRE-1 阻塞 build），53-02 SUMMARY 也只跑了 `bash -n` + `shellcheck`。CI 跑 smoke.sh 的瞬间 T-53-1 ~ T-53-6 会一起红。

**Fix:**
config 必须让 singbox 用户可读。两条路径选其一（推荐 A）：

A. **改 ownership 为 `root:singbox 0640`**（最小权限暴露）：

```bash
# entrypoint.sh L179 改：
if [ "$perm" != "640" ] || [ "$owner" != "root" ]; then
  echo "[entrypoint] FATAL: config 权限不对（want root:singbox 0640，got ${owner}:${perm}）" >&2
  exit 1
fi
local group
group="$(stat -c '%G' "$SING_BOX_CONFIG")"
if [ "$group" != "singbox" ]; then
  echo "[entrypoint] FATAL: config group 不对（want singbox，got ${group}）" >&2
  exit 1
fi
```

并在 host-agent 注入侧（Phase 54）确保 `chown root:singbox && chmod 0640`。smoke.sh L31-33 同步改为 `chmod 640 && sudo chown root:9000 "$TMP_CONFIG"`（host 上没 singbox group，用 numeric gid）。

B. **给 sing-box binary 加 `cap_dac_read_search+ep`**（不推荐 —— 扩大攻击面，sing-box 进程能读容器内任何 root-only 文件）。

接受标准：
- entrypoint.sh 校验逻辑允许 `root:singbox 0640`
- smoke.sh fixture 准备步骤同步更新
- 53-CONTEXT D-V4-2 改为 "config root:singbox 0640 + 启动后 rm"（保留 D-V4-2 的"rm 后 fs 不可见"语义不变）

---

## High

### HI-01: `CONTAINER_USER` 未做输入校验，路径穿越可让 root 删任意 sudoers 路径

**File:** `deploy/docker/managed-user/entrypoint.sh:282-305`
**Issue:**
`CONTAINER_USER` 来自外部环境变量，entrypoint 直接拼进若干 root 进程命令，其中 `rm -f` 路径未做 sanitize：

```297:305:deploy/docker/managed-user/entrypoint.sh
RUN_USER="${CONTAINER_USER:-workspace}"

# v4.0 (D-53-4): 删除 v3.x 的 sudoers NOPASSWD 写入。
# 用户拿到 shell 后不再有任何 sudo / root 提权路径。
# 兜底清理（防御历史镜像残留 / volume 挂载的 sudoers.d）：
rm -f /etc/sudoers.d/workspace 2>/dev/null || true
if [ "${RUN_USER}" != "workspace" ]; then
  rm -f "/etc/sudoers.d/${RUN_USER}" 2>/dev/null || true
fi
```

恶意 / 误配 `CONTAINER_USER='../../etc/shadow'` 会让 root 进程执行 `rm -f /etc/sudoers.d/../../etc/shadow` = `rm -f /etc/shadow`。同样 `CONTAINER_USER='*'` 会触发 `rm -f /etc/sudoers.d/*` 删除整个目录里的所有兜底文件。

虽然 v4.0 已删除 sudoers，删 shadow 也只影响该容器实例（容器是一次性的），但 **principle of least surprise + defense in depth** 仍要求 entrypoint 在使用 `CONTAINER_USER` **之前** 显式校验合法性。

副线风险：L292 `usermod -l "$CONTAINER_USER" workspace` 与 L313 `echo "${CONTAINER_SSH_AUTHORIZED_KEY}" > /workspace/.ssh/authorized_keys` 也都依赖 `CONTAINER_USER` 的合法性，usermod 自带名字校验是 defense-in-depth，但 entrypoint 不该把责任推给下游工具。

**Fix:**
在 `RUN_USER=` 赋值后立刻校验：

```bash
RUN_USER="${CONTAINER_USER:-workspace}"

# 仅允许 POSIX portable username（与 useradd NAME_REGEX 对齐）
if ! [[ "$RUN_USER" =~ ^[a-z_][a-z0-9_-]{0,30}$ ]]; then
  echo "[entrypoint] FATAL: CONTAINER_USER 非法（仅允许 POSIX portable username）: $RUN_USER" >&2
  exit 1
fi
```

接受标准：
- 任何带 `/` `..` `*` `\0` `\n` 的 `CONTAINER_USER` 都让 entrypoint 在 sing-box 启动**之前** exit 1
- 与 D-53-4 的"用户输入不可信"前提一致

---

### HI-02: smoke fixture 缺 stub DNS inbound，但 `lock_resolv_conf` 已把 resolver 钉到 127.0.0.1:53 → libc DNS 必失败

**File:** `tests/phase53/fixtures/test-singbox-config.json:10-22`（fixture）+ `deploy/docker/managed-user/entrypoint.sh:211-224`（lock_resolv_conf）
**Issue:**
fixture 仅声明 `tun` inbound（地址段 `172.19.0.1/30`），**没有任何 inbound 在 `127.0.0.1:53` 监听**：

```10:22:tests/phase53/fixtures/test-singbox-config.json
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "tun0",
      "address": ["172.19.0.1/30"],
      "mtu": 1500,
      "auto_route": true,
      "strict_route": true,
      "stack": "system",
      "endpoint_independent_nat": true
    }
  ],
```

但 entrypoint `lock_resolv_conf` 强制 `/etc/resolv.conf` → `nameserver 127.0.0.1`：

```211:224:deploy/docker/managed-user/entrypoint.sh
lock_resolv_conf() {
  echo "[entrypoint] locking /etc/resolv.conf to sing-box stub"
  cat > /etc/resolv.conf <<'EOF'
# v4.0: DNS 强制走 sing-box stub resolver (D-V4-3)
nameserver 127.0.0.1
options edns0 trust-ad
EOF
```

容器内任何走 libc resolver 的 DNS 查询：

1. 客户端读 `/etc/resolv.conf` → nameserver=127.0.0.1
2. 客户端 `connect(127.0.0.1, 53, UDP)` → kernel 路由表查 127.0.0.1 → `lo` 接口
3. `lo` 上没人监听 53 → ICMP port unreachable / EHOSTUNREACH（`tun` inbound 不会接管 lo 流量，`auto_route` + `strict_route` 只接管 default route，不影响 lo loopback）
4. `getaddrinfo` 返回 EAI_AGAIN

smoke T-53-3 `curl https://api.ipify.org` 会因 DNS 失败而退出 6 (Could not resolve host)，T-53-3 必红。生产路径同样 broken（任何应用 `apt update` / `curl example.com` 都不通）。

**Fix:**
fixture 需追加 stub DNS server + DNS hijack route：

```json
{
  "log": { "level": "info", "timestamp": true },
  "dns": {
    "servers": [
      { "type": "udp", "tag": "upstream-dns", "server": "1.1.1.1" },
      { "type": "local", "tag": "local-dns" }
    ],
    "rules": [],
    "final": "upstream-dns"
  },
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "tun0",
      "address": ["172.19.0.1/30"],
      "mtu": 1500,
      "auto_route": true,
      "strict_route": true,
      "stack": "system",
      "endpoint_independent_nat": true
    }
  ],
  "outbounds": [
    { "type": "direct", "tag": "direct-out" }
  ],
  "route": {
    "rules": [
      { "action": "sniff" },
      { "action": "hijack-dns", "ip_cidr": ["127.0.0.0/8"], "port": 53 }
    ],
    "final": "direct-out",
    "auto_detect_interface": true
  }
}
```

注意 sing-box 1.13.x 的正确字段名是 `hijack-dns`（命中后让 sing-box 内置 DNS 模块直接应答），具体语法以 `sing-box check` 为准。Phase 54 host-agent 注入的生产 config 也必须满足该约束 —— 这是 entrypoint `lock_resolv_conf` 的隐式契约，应在 53-CONTEXT D-V4-3 文档化。

接受标准：
- fixture 在容器内 `getent hosts api.ipify.org` 能返回 IP
- smoke T-53-3 在 CI 跑通

---

### HI-03: `runuser` 会 fork PAM session，`SING_BOX_PID=$!` 抓到的是 wrapper PID，启动日志谎报 uid

**File:** `deploy/docker/managed-user/entrypoint.sh:184-198`
**Issue:**
`runuser` 来自 util-linux，默认会调用 `pam_open_session` / `pam_close_session`，因此实现上必须 `fork()`：parent 等 child 退出后再 `pam_close_session`，child `setresuid` + `execvp(sing-box)`。这意味着：

```184:198:deploy/docker/managed-user/entrypoint.sh
  echo "[entrypoint] starting sing-box as uid=9000 (file-cap based)"
  runuser -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
  SING_BOX_PID=$!

  # WaitFor tun0 ready（替代裸 sleep）
  echo "[entrypoint] waiting for tun0 (timeout=${TUN_READY_TIMEOUT_S}s)"
  local deadline=$((SECONDS + TUN_READY_TIMEOUT_S))
  while (( SECONDS < deadline )); do
    if ip link show tun0 >/dev/null 2>&1; then
      if kill -0 "$SING_BOX_PID" 2>/dev/null; then
        local sb_uid
        sb_uid="$(ps -o uid= -p "$SING_BOX_PID" 2>/dev/null | tr -d ' ' || echo 'unknown')"
        echo "[entrypoint] tun0 ready (sing-box pid=$SING_BOX_PID, uid=$sb_uid)"
        return 0
      fi
    fi
    sleep 0.5
  done
```

- `SING_BOX_PID=$!` 抓到的是 **runuser parent**（uid=0，等待 child）
- `ps -o uid= -p $SING_BOX_PID` 输出 `0`
- 启动日志会打印 `tun0 ready (sing-box pid=N, uid=0)` —— **谎报 sing-box 跑在 uid=0**
- D-53-2 / EP-02 断言"sing-box uid=9000"在日志层面无可观测信号

**fail-closed 语义巧合 OK**（runuser parent 会在 child 退出时自动退出，`monitor_singbox_fail_closed` 的 `kill -0 $SING_BOX_PID` 仍能感知），但 `verifier` 在 53-VERIFICATION 把"runuser → uid=9000"标 ✓ VERIFIED 也是基于这个误导日志。

**Fix:**
两条路径选其一（推荐 A）：

A. **runuser 加 `--no-pam`**（避免 fork，要求 util-linux ≥ 2.36，Ubuntu 24.04 自带）：

```bash
runuser --no-pam -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
SING_BOX_PID=$!
```

B. **改用 `setpriv`**（util-linux 自带，不走 PAM，无 fork）：

```bash
setpriv --reuid="$SING_BOX_USER" --regid="$SING_BOX_USER" --clear-groups \
  /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
SING_BOX_PID=$!
```

C. 或在 wait_for_tun0 中**显式查 sing-box 真实 PID**：

```bash
sb_real_pid=$(pgrep -u "$SING_BOX_USER" -nx sing-box || echo "")
sb_uid=$(ps -o uid= -p "$sb_real_pid" 2>/dev/null | tr -d ' ' || echo 'unknown')
[ "$sb_uid" = "9000" ] || { echo "FATAL: sing-box uid=$sb_uid != 9000" >&2; exit 1; }
```

C 方案额外能在启动序列里把"sing-box 实际跑在 uid=9000"做成硬断言（弥补 53-VERIFICATION T2 oracle 的误判）。

接受标准：
- entrypoint 启动日志能正确显示 `sing-box pid=<N>, uid=9000`
- `monitor_singbox_fail_closed` 的 `kill -0` polling 仍能在 sing-box 真实进程死时触发 fail-closed

---

## Medium

### ME-01: smoke.sh `sudo chown` 没有 `-n`，开发期会卡密码 prompt

**File:** `tests/phase53/smoke.sh:32-33`
**Issue:**

```32:33:tests/phase53/smoke.sh
# config 必须由 root 拥有以模拟生产 host-agent 注入语义
sudo chown root:root "$TMP_CONFIG" 2>/dev/null || true
```

`sudo` 没带 `-n`（non-interactive），如果 sudo timestamp 已过期，会触发 TTY 密码 prompt 卡住整个脚本。`2>/dev/null || true` 兜底无效（prompt 是直接读 `/dev/tty`，不走 stderr）。在 CI 上 sudo 一般 NOPASSWD，OK；在开发者本地 `bash tests/phase53/smoke.sh` 时会卡。

而且 `|| true` 把 sudo 失败也吞掉了 —— 失败后 fixture 仍是当前 host user 拥有，进容器后 `start_singbox_or_die` L179 校验 `owner=root` 失败 → exit 1 → smoke T-53-1 直接红，但失败信息被掩盖。

**Fix:**

```bash
if ! sudo -n chown root:root "$TMP_CONFIG" 2>/dev/null; then
  echo "[smoke] FATAL: 需要 NOPASSWD sudo 以将 fixture chown 为 root（CI 环境正常；本地请配置 sudoers）" >&2
  exit 1
fi
```

---

### ME-02: SC4 第二条 oracle `ip link set tun0 down` EPERM 断言缺失

**File:** `tests/phase53/smoke.sh:76-86`
**Issue:**
verifier 已识别（53-VERIFICATION gap #1）。SC4 显式要求"`ip link set tun0 down` 必须返回 `Operation not permitted`"，smoke T-53-4 仅断言空 cap + sudo 拒绝，未覆盖第二条 oracle。

**Fix:**

```bash
log "[T-53-4b] workspace 不能 ip link set tun0 down"
if ! docker exec -u workspace "$CONTAINER_NAME" ip link set tun0 down 2>&1 | grep -q "Operation not permitted"; then
  fail "T-53-4b workspace 居然能 ip link set tun0 down（NET_ADMIN cap inheritance broken）"
fi
```

注意：smoke.sh 用 `docker exec -u workspace`，而 docker exec 默认会 keep CAP set，需要在 docker run 时 `--cap-drop ALL --cap-add NET_ADMIN` 才能让 NET_ADMIN 仅授予 sing-box（已经做了，L39）；docker exec 进程默认不继承 NET_ADMIN（除非 docker 版本异常），但仍应显式校验。

---

### ME-03: Dockerfile apt install 缺 `dnsutils`（IMG-03 REQ 不达标）

**File:** `deploy/docker/managed-user/Dockerfile:25-57`
**Issue:**
verifier 已识别（53-VERIFICATION gap #2）。IMG-03 显式列 `dig` 为镜像必装工具，NET-02 SC `dig @8.8.8.8 example.com` 验证用例无可执行 binary。

**Fix:**

```dockerfile
# Dockerfile L25-57 apt install 段追加：
        bind9-host \      # 提供 host 命令（足够运维查 DNS，体积比 dnsutils 小）
        dnsutils \        # 或这个，提供 dig + nslookup
```

推荐 `bind9-host`（单一工具 ~200KB，足够 NET-02 oracle）；如果想给运维更全工具集用 `dnsutils`（~10MB）。

---

### ME-04: `apply_nft_or_die` 二次 verify 不能保证规则真在 hook 上生效

**File:** `deploy/docker/managed-user/entrypoint.sh:238-243`
**Issue:**

```238:243:deploy/docker/managed-user/entrypoint.sh
  if ! nft list table inet cloud_proxy_v4 >/dev/null 2>&1; then
    echo "[entrypoint] FATAL: nft table cloud_proxy_v4 未生效" >&2
    if [ -n "$SING_BOX_PID" ]; then kill "$SING_BOX_PID" 2>/dev/null || true; fi
    exit 1
  fi
```

`nft list table` 仅证明 table 对象在 nft userspace 可见，不证明 chain 上的 hook 已绑定到 netfilter。如果 kernel 缺 `nf_tables_inet` 模块（极罕见，但 docker 主机 host 可能 selectively unload），`nft -f` 会成功 add 但 hook 不 active —— 流量不会被 drop，**default-deny 静默失效**。

**Fix:**
显式探测一条 drop 规则的 counter，发一个测试包再 verify counter 增长：

```bash
# 简化：dummy verify — 探测无主机的 IP（10.255.255.1）
local before after
before=$(nft -j list table inet cloud_proxy_v4 | jq '.. | objects | select(.counter? != null) | .counter.packets' 2>/dev/null | head -1 || echo 0)
timeout 1 curl -s http://10.255.255.1/ >/dev/null 2>&1 || true
after=$(nft -j list table inet cloud_proxy_v4 | jq '.. | objects | select(.counter? != null) | .counter.packets' 2>/dev/null | head -1 || echo 0)
if [ "$after" -le "$before" ]; then
  echo "[entrypoint] FATAL: nft hook 未生效（counter 未增长）" >&2
  exit 1
fi
```

可选 fix（不强求，但要在 53-CONTEXT 标 known limitation）。

---

### ME-05: `lock_resolv_conf` 没有先 `chattr -i`，旧 immutable bit 会让 `cat >` 失败

**File:** `deploy/docker/managed-user/entrypoint.sh:212-223`
**Issue:**

```212:223:deploy/docker/managed-user/entrypoint.sh
  cat > /etc/resolv.conf <<'EOF'
# v4.0: DNS 强制走 sing-box stub resolver (D-V4-3)
nameserver 127.0.0.1
options edns0 trust-ad
EOF
  chmod 0644 /etc/resolv.conf
  if ! chattr +i /etc/resolv.conf 2>/dev/null; then
    echo "[entrypoint] WARN: chattr +i /etc/resolv.conf 失败（可能是 overlayfs），依赖 nft drop 兜底"
  fi
```

如果上次容器实例正常关停后，`/etc/resolv.conf` 被 `chattr +i` 持久化在某些 overlay backend / 持久化 volume 上（非 default 配置但可能），重启容器时 `cat > /etc/resolv.conf` 会因 immutable 失败 → `set -e` 触发 → entrypoint exit 1 → 但此时 sing-box 已经在跑（start_singbox_or_die 已完成）→ orphan。

**Fix:**

```bash
chattr -i /etc/resolv.conf 2>/dev/null || true
cat > /etc/resolv.conf <<'EOF'
...
EOF
```

`chattr -i` 在文件无 immutable bit 时无 side effect，加上一行成本极低，避免 corner case 被 `set -e` 直接搞死 entrypoint。

---

### ME-06: `start_singbox_or_die` waitFor 期间不验证 sing-box 实际 uid，仅验 pid alive

**File:** `deploy/docker/managed-user/entrypoint.sh:191-202`
**Issue:**
即使 HI-03 的 runuser fork 问题修了，waitFor 仍只验证 `kill -0 $SING_BOX_PID` 成功 + tun0 link existence。**没有断言 sing-box 真在 uid=9000 跑**。如果 capability 配置错（例如 setcap 失败但 Dockerfile 没卡住，setcap 命令 grep 兜底 OK 不出错），sing-box 可能因为 NET_ADMIN missing 而 fallback 用 root 跑（如果 entrypoint 不小心 uid=0 起 sing-box）—— 但这个场景已被 runuser 切断。真正剩下的风险是 D-V4-1 SC2 失守：sing-box 跑起来了，但不是 uid=9000。

**Fix:**
在 waitFor 成功后追加 hard-assert（结合 HI-03 修复方案 C）：

```bash
local sb_real_pid sb_uid
sb_real_pid=$(pgrep -u "$SING_BOX_USER" -nx sing-box 2>/dev/null || echo "")
if [ -z "$sb_real_pid" ]; then
  echo "[entrypoint] FATAL: 找不到 uid=9000 的 sing-box 进程" >&2
  exit 1
fi
sb_uid=$(ps -o uid= -p "$sb_real_pid" | tr -d ' ')
if [ "$sb_uid" != "9000" ]; then
  echo "[entrypoint] FATAL: sing-box uid=$sb_uid（want 9000）" >&2
  exit 1
fi
```

---

## Low / Info

### LO-01: `shred -u` 在 overlay2 上无法保证物理擦除（D-V4-2 语义弱化）

**File:** `deploy/docker/managed-user/entrypoint.sh:247-256`
**Issue:**
`shred` 通过原地多次覆写再 unlink 来防止恢复，但 overlay2 / overlayfs 是 copy-on-write —— `shred` 的覆写写到 upper layer 的新 inode，旧 inode（lower layer 或被 shred 之前的 upper page）可能仍可恢复（取决于 storage driver 与 host filesystem）。生产是 docker overlay2 + host ext4 的组合，shred 实际只达到 `unlink + 一次额外写`，不能等同于裸盘的"3-pass overwrite"语义。

**Impact:**
攻击者拿到容器物理磁盘后**理论上可能恢复 config**。但威胁模型里"拿物理磁盘"已经超出 v4.0 范围（host 运维方可信）。建议在 53-CONTEXT D-V4-2 显式注明"shred 在 overlay2 上仅作 best-effort，secure deletion 不在 v4.0 威胁模型内"。

**Fix:**
文档化即可，代码不动。

---

### LO-02: 镜像保留 `sudo` 包，扩大攻击面

**File:** `deploy/docker/managed-user/Dockerfile:32`
**Issue:**
v4.0 已删除所有 sudoers 写入（D-53-4），但 `sudo` binary 仍在镜像里：

```32:32:deploy/docker/managed-user/Dockerfile
        sudo \
```

虽然没有 sudoers 配置，sudo 进程拿不到提权，但 sudo binary 本身是 SUID root，**任何 sudo 历史 CVE（如 CVE-2021-3156 baron samedit）都会变成镜像内的潜在提权路径**，而 v4.0 已宣告"用户不再有任何 sudo / root 提权路径"。

**Fix:**

```dockerfile
# Dockerfile L25-57 删除 sudo 行
# 或单独：
RUN apt-get purge -y sudo && rm -rf /var/lib/apt/lists/*
```

接受标准：`docker run --rm managed-user:v4-dev which sudo` 返回非零。

---

### LO-03: sing-box stdout/stderr 没有重定向到日志文件

**File:** `deploy/docker/managed-user/entrypoint.sh:186`
**Issue:**

```186:186:deploy/docker/managed-user/entrypoint.sh
  runuser -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" &
```

sing-box 输出会和 entrypoint 自身日志、sshd 日志全部混在容器 stdout，排障时难以区分。

**Fix:**

```bash
mkdir -p /var/log/sing-box
chown singbox:singbox /var/log/sing-box
runuser -u "$SING_BOX_USER" -- /usr/local/bin/sing-box run -c "$SING_BOX_CONFIG" \
  >>/var/log/sing-box/stdout.log 2>>/var/log/sing-box/stderr.log &
```

或在 fixture / 生产 config 里走 `log.output` 字段。

---

### LO-04: nft 显式 drop 仅覆盖 IPv4 DNS，IPv6 DNS 仅靠 policy drop 兜底

**File:** `deploy/docker/managed-user/default-deny.nft:25-26`
**Issue:**

```25:26:deploy/docker/managed-user/default-deny.nft
        meta l4proto udp udp dport 53 ip daddr != 127.0.0.0/8 counter drop comment "extdns-udp-drop"
        meta l4proto tcp tcp dport { 53, 853 } ip daddr != 127.0.0.0/8 counter drop comment "extdns-tcp-drop"
```

`ip daddr` 只匹配 IPv4。IPv6 DNS 流量（dport 53/853）会落到 `policy drop` 兜底，但**没有专属 counter**，排障时看不到"有 IPv6 DNS 尝试"。entrypoint L333-334 已 sysctl disable_ipv6，OK，但兜底层缺乏可观测性。

**Fix:**

```
meta l4proto udp udp dport 53 ip6 daddr != ::1 counter drop comment "extdns6-udp-drop"
meta l4proto tcp tcp dport { 53, 853 } ip6 daddr != ::1 counter drop comment "extdns6-tcp-drop"
```

可选 fix。

---

### LO-05: `assert_tmux_version` case 模式 `[4-9].*` 与 `3.4*..3.9*` 共存，意图正确但易误读

**File:** `deploy/docker/managed-user/entrypoint.sh:140-153`
**Issue:**

```140:153:deploy/docker/managed-user/entrypoint.sh
  case "$tmux_ver" in
    3.4*|3.5*|3.6*|3.7*|3.8*|3.9*|[4-9].*)
```

`[4-9].*` 实际匹配 `4.x..9.x`（OK，意图是 ≥3.4 全放行），但混在 `3.4*|3.5*|...` 之间易让 reviewer 误读为只覆盖 4.x..9.x 第一位。建议改为更清晰的写法：

```bash
# 提取 major.minor，与 3.4 数字比较
local major minor
major="${tmux_ver%%.*}"
minor_full="${tmux_ver#*.}"
minor="${minor_full%%.*}"
if (( major < 3 )) || { (( major == 3 )) && (( minor < 4 )); }; then
  echo "[entrypoint] v3: FATAL tmux ${tmux_ver} < 3.4" >&2
  exit 1
fi
```

非 phase 53 主线，可选 fix。

---

_Reviewed: 2026-05-16T03:05:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
