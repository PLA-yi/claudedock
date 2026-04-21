---
phase: 29-v3-worker
plan: 03-entrypoint-config
sub_scope: C
type: execute
wave: 3
depends_on: ["01-image-base", "02-binaries"]
files_modified:
  - deploy/docker/managed-user/entrypoint.sh
  - deploy/docker/managed-user/tmux.conf
  - deploy/docker/managed-user/profile.d-cloud-claude.sh
  - deploy/docker/managed-user/sshd_config
  - deploy/docker/managed-user/Dockerfile
autonomous: true
requirements:
  - C1
  - C2
  - C3
  - C5
  - C7
  - M4
  - M7
  - M8
  - M12
  - M17
  - Q10
must_haves:
  truths:
    - "entrypoint.sh 在启动 sshd 之前串行执行 prepare_v3_dirs / prepare_mutagen_agent / prepare_mergerfs_check / assert_tmux_version 四个阶段，任一失败 exit 1"
    - "容器内 /etc/tmux.conf 存在且包含 terminal-overrides \",*:RGB\" / window-size latest / aggressive-resize on / history-limit 50000"
    - "容器内 /etc/profile.d/cloud-claude.sh 存在且导出 CLAUDE_CODE_TMUX_TRUECOLOR=1"
    - "容器内 /etc/ssh/sshd_config 在现有字段末尾追加 ClientAliveInterval 15 / ClientAliveCountMax 8 / MaxSessions 30 / MaxStartups 60:30:120"
    - "entrypoint 对 /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist 做二次 chown 1000:1000；/workspace 不参与"
    - "entrypoint 在 /usr/local/libexec/mutagen/agents/ 下解压 /opt/mutagen-agents.tar.gz 并 touch .extracted 做幂等标记"
    - "entrypoint 运行时把 tmux 实际版本写入 /etc/cloud-claude/tmux.version 替代构建期占位"
  artifacts:
    - path: "deploy/docker/managed-user/entrypoint.sh"
      provides: "v3 串行编排阶段（prepare_* + assert_tmux_version）+ 尾行 exec sshd 不动"
    - path: "deploy/docker/managed-user/tmux.conf"
      provides: "容器级 tmux 默认配置（M7 / M8 防御）"
      contains: "terminal-overrides"
      min_lines: 4
    - path: "deploy/docker/managed-user/profile.d-cloud-claude.sh"
      provides: "CLAUDE_CODE_TMUX_TRUECOLOR 导出（M8）"
    - path: "deploy/docker/managed-user/sshd_config"
      provides: "v3 KeepAlive / MaxSessions / MaxStartups 字段"
      contains: "MaxStartups 60:30:120"
    - path: "deploy/docker/managed-user/Dockerfile"
      provides: "追加 COPY tmux.conf / profile.d；不改动 Plan 01/02 已写入的骨架"
  key_links:
    - from: "entrypoint.sh:prepare_mutagen_agent"
      to: "/usr/local/libexec/mutagen/agents/"
      via: "tar -xzf /opt/mutagen-agents.tar.gz"
      pattern: "tar -xzf.*mutagen-agents.tar.gz"
    - from: "entrypoint.sh:assert_tmux_version"
      to: "/etc/cloud-claude/tmux.version"
      via: "tmux -V | awk + 正则判 >=3.4 + echo > file"
      pattern: "tmux -V"
    - from: "Dockerfile COPY tmux.conf"
      to: "/etc/tmux.conf"
      via: "COPY ... && chmod 0644"
      pattern: "/etc/tmux.conf"
---

## Goal

把 v3.0 的 entrypoint 串行阶段、tmux 配置、profile.d 环境变量、sshd_config 服务端基线一次性落地。新增两个静态配置文件并通过 Dockerfile `COPY` 进镜像；原地改造 entrypoint.sh 在 `exec /usr/sbin/sshd -D -e`（当前 186 行）**之前**插入 v3 阶段；sshd_config 追加 4 行 KeepAlive 字段。**不**挂载 mergerfs（仅校验 `mergerfs --version`），**不**写 image.lock / CI / contracts / worker。

对应 Sub-scope：**C entrypoint & 配置**（29-RESEARCH.md §Sub-scope 映射）。

---

## Scope

### In
1. 新建 `deploy/docker/managed-user/tmux.conf`（4 行 + 注释）
2. 新建 `deploy/docker/managed-user/profile.d-cloud-claude.sh`（单行 export + 注释）
3. 原地修改 `deploy/docker/managed-user/sshd_config`：末尾追加 4 行 KeepAlive / MaxSessions / MaxStartups
4. 原地修改 `deploy/docker/managed-user/entrypoint.sh`：在现有 ssh-keygen / 用户同步 / FUSE chmod 之后、`exec /usr/sbin/sshd -D -e` 之前插入 v3 阶段函数定义与调用
5. 原地修改 `deploy/docker/managed-user/Dockerfile`：新增 `COPY tmux.conf → /etc/tmux.conf` + `COPY profile.d-cloud-claude.sh → /etc/profile.d/cloud-claude.sh` + `chmod 0644`，插入位置紧邻现 sshd_config / entrypoint COPY 附近

### Out
- Plan 01 的 apt/BuildKit/预建目录/ENTRYPOINT tini 改造 → 不动
- Plan 02 的 mergerfs/mutagen 下载 + `/etc/cloud-claude/*.version` 写入 → 不动
- Plan 04 的 contracts/worker → 不动
- Plan 05 的 host-preflight.sh / 运维文档 → 不动
- Plan 06 的 image.lock / CI → 不动
- entrypoint.sh 中 KasmVNC / Fluxbox / Chromium / Xvnc 逻辑（当前 103-183 行）→ 不动（AP2 明确禁止把 v3 逻辑挤进 KasmVNC 私有函数）

---

## Dependencies

- **Wave 3**，`depends_on: ["01-image-base", "02-binaries"]`
- 原因：
  - 依赖 Plan 01 的预建目录（entrypoint `prepare_v3_dirs` 的 chown 需要目录已存在）
  - 依赖 Plan 02 的 `/opt/mutagen-agents.tar.gz`（entrypoint `prepare_mutagen_agent` 需要 tarball 在场）
  - 依赖 Plan 02 的 `/etc/cloud-claude/tmux.version`（assert_tmux_version 回填时文件需存在）

---

## Tasks

### Task 3.1 — 新建 tmux.conf + profile.d-cloud-claude.sh 两份配置文件

**文件：**
- `deploy/docker/managed-user/tmux.conf`（新建）
- `deploy/docker/managed-user/profile.d-cloud-claude.sh`（新建）

**`deploy/docker/managed-user/tmux.conf` 内容：**
```tmux
# v3.0 baseline — PITFALLS M7 (多端 resize) / M8 (Claude Code truecolor) 防御
set -ga terminal-overrides ",*:RGB"
set -g  window-size latest
set -g  aggressive-resize on
set -g  history-limit 50000
```

**`deploy/docker/managed-user/profile.d-cloud-claude.sh` 内容：**
```sh
#!/bin/sh
# v3.0 baseline — Claude Code truecolor hint (PITFALLS M8)
export CLAUDE_CODE_TMUX_TRUECOLOR=1
```

**对应：** D-13（tmux.conf 硬编码 4 行 + profile.d 导出）
**PATTERNS：** N1 + N2（无现有 analog，按 RESEARCH §Code Examples 骨架）；D4（COPY + chmod 模式由 Task 3.4 完成）
**Pitfalls 防御：** M7（window-size latest / aggressive-resize） / M8（terminal-overrides RGB + CLAUDE_CODE_TMUX_TRUECOLOR）

### Task 3.2 — sshd_config 末尾追加 4 行 v3 基线字段

**文件：** `deploy/docker/managed-user/sshd_config`

**改动要点：**
- 在现第 14 行 `Subsystem sftp /usr/lib/openssh/sftp-server` **之前**追加 4 行：
  ```
  ClientAliveInterval 15
  ClientAliveCountMax 8
  MaxSessions 30
  MaxStartups 60:30:120
  ```
  （追加位置在 Subsystem 之前是常规 SSH 配置顺序；追加到最后一行之后也可；本 plan 推荐在 Subsystem 之前以保持全局指令集中）
- **不**修改现有字段（Port/Protocol/AddressFamily/PermitRootLogin/PasswordAuthentication/PermitEmptyPasswords/ChallengeResponseAuthentication/UsePAM/X11Forwarding/PrintMotd/AuthorizedKeysFile/PidFile/Subsystem）
- **不**添加注释（现文件全扁平 key-value，无注释分组；PATTERNS Sub-scope C 子项已确认）

**对应：** D-14（sshd_config 追加 4 字段） / M12（ControlMaster MaxSessions=10 防御）
**PATTERNS：** C 子项行（扁平 key-value，追加至 Subsystem 之前）
**Pitfalls 防御：** M12（MaxSessions 30 + MaxStartups 60:30:120 打开并发门） / REQ-F3-A 的服务端基线（KeepAlive 15s）

### Task 3.3 — entrypoint.sh 插入 v3 阶段函数定义 + 调用

**文件：** `deploy/docker/managed-user/entrypoint.sh`

**改动要点：**

**（a）在文件头部、现 `wait_for_x_display` 函数（45-58 行）**之后**插入 4 个新函数（保持 `snake_case + 动词前缀` 风格，PATTERNS S3 / Sub-scope C 汇总行）：**

```bash
# ===== v3.0 stages — D-09 / PITFALLS M4 串行快速失败 =====

prepare_v3_dirs() {
  echo "[entrypoint] v3: chown /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist"
  chown -R 1000:1000 \
    /home/claude \
    /workspace-hot \
    /workspace-cold \
    /var/lib/claude-persist 2>/dev/null || true
}

prepare_mutagen_agent() {
  local src=/opt/mutagen-agents.tar.gz
  local dest=/usr/local/libexec/mutagen/agents
  if [[ ! -f "$src" ]]; then
    echo "[entrypoint] v3: FATAL missing $src" >&2
    exit 1
  fi
  mkdir -p "$dest"
  if [[ ! -f "$dest/.extracted" ]]; then
    tar -xzf "$src" -C "$dest"
    touch "$dest/.extracted"
  fi
  echo "[entrypoint] v3: mutagen agents ready at $dest"
}

prepare_mergerfs_check() {
  if ! command -v mergerfs >/dev/null 2>&1; then
    echo "[entrypoint] v3: FATAL mergerfs binary missing" >&2
    exit 1
  fi
  local ver
  ver="$(mergerfs --version 2>&1 | head -n1 || true)"
  echo "[entrypoint] v3: mergerfs available ($ver) — mount deferred to cloud-claude (Phase 31)"
  # SC1 / C1 / C2 mergerfs mount params (Phase 31 cloud-claude consumes; locked here
  # only as docstring so SC1/SC2 静态 grep 可断言；本阶段不执行 mount)：
  #   cache/readdir:  func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off
  #   branch policy:  category.create=ff,inodecalc=path-hash
  #   2-way branches: /workspace-hot=RW:/workspace-cold=NC,RO
  # Q10：2 路 branch 锁定，3 路扩展通过 CLOUD_CLAUDE_MERGERFS_BRANCHES env 预留
  # （读取位置在 Phase 31 cloud-claude，本阶段仅登记 env 名称，不读取）
  echo "[entrypoint] v3: expected mergerfs params (documented for Phase 31): func.readdir=cor:4 category.create=ff"
}

assert_tmux_version() {
  local tmux_ver
  tmux_ver="$(tmux -V 2>/dev/null | awk '{print $2}' || true)"
  case "$tmux_ver" in
    3.4*|3.5*|3.6*|3.7*|3.8*|3.9*|[4-9].*)
      echo "[entrypoint] v3: tmux ${tmux_ver} >= 3.4 ok"
      echo "$tmux_ver" > /etc/cloud-claude/tmux.version
      ;;
    *)
      echo "[entrypoint] v3: FATAL tmux ${tmux_ver} < 3.4" >&2
      exit 1
      ;;
  esac
}
```

**（b）在 `exec /usr/sbin/sshd -D -e`（现 186 行）之前、所有 KasmVNC / Fluxbox / Chromium 启动逻辑（155-183 行）之后插入串行调用：**

```bash
# ===== v3.0 stages (serialized fail-fast, D-09 order) =====
prepare_v3_dirs
prepare_mutagen_agent
prepare_mergerfs_check
assert_tmux_version

# Foreground: sshd
exec /usr/sbin/sshd -D -e
```

- **严格顺序**：`prepare_fuse` 的职责已由现 99-101 行的 `if [ -c /dev/fuse ]; then chmod 666 /dev/fuse; fi` 承担（D-09 第 1 步已在 v2.0 entrypoint 就位，不再另起函数）；v3 新增从 `prepare_v3_dirs` 开始
- **不并行化**（AP2 + M4）；**不**在任一函数里启动 mergerfs mount（AP4）
- **不**把任何 v3 阶段挤进 `write_desktop_config` / `wait_for_x_display`（AP2）
- **不**对 `/workspace` 做第二次 `chown`（AP10：现 90 行已负责）

**对应：** D-09（串行阶段） / D-10（tini PID 1，由 Plan 01 保障） / D-11（mergerfs 参数只文档化，不挂载） / D-12（2 路 branch + env 预留；本 plan 只在注释登记 env 名） / M4（串行 + 快速失败） / Q10
**PATTERNS：** 汇总行 C 列 + S3（串行编排） + S4（轮询超时，本阶段未用但保持风格） + AP2 / AP4 / AP10

### Task 3.4 — Dockerfile 新增 COPY tmux.conf + profile.d-cloud-claude.sh + chmod 0644

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 插入位置：**紧随现 `COPY deploy/docker/managed-user/sshd_config /etc/ssh/sshd_config`**（当前 89 行）**之后**、现 `COPY deploy/docker/managed-user/entrypoint.sh /usr/local/bin/entrypoint.sh`（当前 90 行）**之前**（或 `RUN chmod +x ...` 当前 95 行之前均可；保持 COPY 成段、chmod 成段的既有风格）
- 追加 2 行 COPY + 1 个 chmod RUN（可合并到现有 chmod RUN）：
  ```dockerfile
  COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf
  COPY deploy/docker/managed-user/profile.d-cloud-claude.sh /etc/profile.d/cloud-claude.sh
  ```
- 现 `RUN chmod +x /usr/local/bin/entrypoint.sh ...`（95 行）保持不变；新增一条 chmod RUN 或扩展现有 RUN：
  ```dockerfile
  RUN chmod 0644 /etc/tmux.conf /etc/profile.d/cloud-claude.sh
  ```
  （`profile.d` 脚本 0644 即可被 `/bin/sh` source；不需要可执行位；PATTERNS D4 略有差异：原 D4 是 `chmod +x`，本 task 故意不加 +x）

**对应：** D-13（COPY 配置文件进镜像）
**PATTERNS：** D4（COPY + chmod 模式，调整 mode 到 0644）
**Anti-pattern 回避：** 不用 entrypoint 内 heredoc 生成 tmux.conf（PATTERNS S5 仅作 fallback；COPY 静态文件更可审计）

---

## Verification

### 静态断言（文件 / grep / shellcheck）

```bash
# tmux.conf 新建文件 + 内容
test -f deploy/docker/managed-user/tmux.conf
grep -F 'terminal-overrides ",*:RGB"' deploy/docker/managed-user/tmux.conf
grep -F 'window-size latest' deploy/docker/managed-user/tmux.conf
grep -F 'aggressive-resize on' deploy/docker/managed-user/tmux.conf
grep -F 'history-limit 50000' deploy/docker/managed-user/tmux.conf

# profile.d
test -f deploy/docker/managed-user/profile.d-cloud-claude.sh
grep -F 'CLAUDE_CODE_TMUX_TRUECOLOR=1' deploy/docker/managed-user/profile.d-cloud-claude.sh

# sshd_config 4 行
grep -E '^ClientAliveInterval 15$'    deploy/docker/managed-user/sshd_config
grep -E '^ClientAliveCountMax 8$'     deploy/docker/managed-user/sshd_config
grep -E '^MaxSessions 30$'            deploy/docker/managed-user/sshd_config
grep -E '^MaxStartups 60:30:120$'     deploy/docker/managed-user/sshd_config

# entrypoint.sh 函数定义 + 调用
grep -E '^prepare_v3_dirs\(\) \{'       deploy/docker/managed-user/entrypoint.sh
grep -E '^prepare_mutagen_agent\(\) \{' deploy/docker/managed-user/entrypoint.sh
grep -E '^prepare_mergerfs_check\(\) \{' deploy/docker/managed-user/entrypoint.sh
grep -E '^assert_tmux_version\(\) \{'   deploy/docker/managed-user/entrypoint.sh

# R2 / SC1 / SC2 / C1 / C2 — mergerfs 关键 mount 参数必须作为字符串出现在 prepare_mergerfs_check 内，
# 供 Phase 31 cloud-claude 上游溯源、供 SC1/SC2 的 Phase 29 静态面直接 grep 断言。
grep -F 'func.readdir=cor:4'  deploy/docker/managed-user/entrypoint.sh
grep -F 'category.create=ff'  deploy/docker/managed-user/entrypoint.sh
grep -F 'inodecalc=path-hash' deploy/docker/managed-user/entrypoint.sh
# 调用顺序：必须都在 exec sshd 之前
awk '/^prepare_v3_dirs$/{a=NR} /^prepare_mutagen_agent$/{b=NR} /^prepare_mergerfs_check$/{c=NR} /^assert_tmux_version$/{d=NR} /^exec \/usr\/sbin\/sshd /{e=NR} END{exit !(a<b && b<c && c<d && d<e)}' deploy/docker/managed-user/entrypoint.sh

# /workspace 不在 v3 chown（AP10）
! grep -E 'chown.*1000:1000.*\/workspace[^-]' deploy/docker/managed-user/entrypoint.sh | grep -v 'already-handled-by-v2'

# Dockerfile COPY tmux / profile.d
grep -F 'COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf'                deploy/docker/managed-user/Dockerfile
grep -F 'COPY deploy/docker/managed-user/profile.d-cloud-claude.sh /etc/profile.d/cloud-claude.sh' deploy/docker/managed-user/Dockerfile
grep -E 'chmod 0644.*\/etc\/tmux\.conf.*\/etc\/profile\.d\/cloud-claude\.sh' deploy/docker/managed-user/Dockerfile

# shellcheck entrypoint.sh（可选，但推荐）
shellcheck -S warning deploy/docker/managed-user/entrypoint.sh || true
bash -n deploy/docker/managed-user/entrypoint.sh
```

### 运行时断言（需 Plan 01+02+03 合成的镜像）

```bash
DOCKER_BUILDKIT=1 docker build \
  --build-arg MERGERFS_SHA256_AMD64=<实测值> \
  -f deploy/docker/managed-user/Dockerfile \
  -t local/managed-user:p03-test .

docker run -d --name mu-p03 --cap-add SYS_ADMIN --device /dev/fuse \
  --security-opt apparmor=unconfined \
  -e CONTAINER_USER=workspace -e CONTAINER_SSH_PASSWORD=workspace \
  local/managed-user:p03-test

# 等待 entrypoint v3 阶段完成（最多 10s）
for i in {1..20}; do
  docker exec mu-p03 test -f /usr/local/libexec/mutagen/agents/.extracted && break
  sleep 0.5
done

# SC3 & SC5 断言
docker exec mu-p03 stat -c '%u:%g' /home/claude/.claude         | grep -Fq '1000:1000'
docker exec mu-p03 stat -c '%u:%g' /workspace-hot               | grep -Fq '1000:1000'
docker exec mu-p03 stat -c '%u:%g' /workspace-cold              | grep -Fq '1000:1000'
docker exec mu-p03 stat -c '%u:%g' /var/lib/claude-persist      | grep -Fq '1000:1000'
docker exec mu-p03 ps -o pid=,comm= -p 1 | awk '{print $2}' | grep -q '^tini$'
! docker exec mu-p03 pgrep -x systemd-logind

# SC4 断言（tmux.conf + profile.d）
docker exec mu-p03 test -f /etc/tmux.conf
docker exec mu-p03 grep -F ',*:RGB' /etc/tmux.conf
docker exec mu-p03 grep -F 'window-size latest' /etc/tmux.conf
docker exec mu-p03 bash -lc 'echo $CLAUDE_CODE_TMUX_TRUECOLOR' | grep -Fq '1'
docker exec mu-p03 cat /etc/cloud-claude/tmux.version | grep -Eq '^3\.[4-9]'

# V-04 断言（sshd_config 追加）
docker exec mu-p03 grep -E '^MaxSessions 30$'            /etc/ssh/sshd_config
docker exec mu-p03 grep -E '^MaxStartups 60:30:120$'     /etc/ssh/sshd_config
docker exec mu-p03 grep -E '^ClientAliveInterval 15$'    /etc/ssh/sshd_config
docker exec mu-p03 grep -E '^ClientAliveCountMax 8$'     /etc/ssh/sshd_config

# mutagen agents 解压完成 + mergerfs 校验通过
docker exec mu-p03 test -d /usr/local/libexec/mutagen/agents
docker exec mu-p03 test -f /usr/local/libexec/mutagen/agents/.extracted
docker exec mu-p03 mergerfs --version 2>&1 | grep -F '2.41.1'

docker rm -f mu-p03
```

### Coverage contribution

> **Coverage contribution:** SC3（chown + mutagen 版本） / SC4（tmux / profile.d） / SC5（PID 1 = tini，配合 Plan 01 完成） / V-04（sshd_config 4 行追加） / V-05（C5/M17 chown 运行时兜底）→ 本 plan 负责 entrypoint 与配置层面的全部运行时断言落地。SC1 / SC2 的真正 mount 断言由 Phase 31 消费。
>
> **Pitfall coverage:** C1（mergerfs 参数文档化） / C2（category.create=ff 文档化，仅 version 校验） / C3（sshfs 参数文档化，不改 sshfs） / C5（entrypoint 二次 chown） / C7（不启动 systemd-logind，由 Plan 01 tini + 本 plan 不启 systemd 共同保障） / M4（串行 + fail-fast） / M7 + M8（tmux.conf + profile.d） / M12（sshd_config 4 行） / M17（二次 chown） → 本 plan 是 v3 坑位防御的主战场。

---

## Atomic Commit Strategy

4 个原子 commit：

1. `feat(29-03): add tmux.conf and profile.d cloud-claude.sh for v3 baseline`
   - Task 3.1（新增两份静态配置文件）
2. `feat(29-03): sshd_config append KeepAlive, MaxSessions, MaxStartups`
   - Task 3.2（4 行追加）
3. `feat(29-03): entrypoint.sh insert v3 serial stages (prepare/* + assert tmux)`
   - Task 3.3（entrypoint 函数定义 + 调用）
4. `feat(29-03): Dockerfile COPY tmux.conf and profile.d cloud-claude.sh`
   - Task 3.4（Dockerfile COPY 行 + chmod）

---

## Pitfalls 防御

| Pitfall | 防御手段 | 本 plan 对应任务 |
|---------|---------|-----------------|
| **C1** mergerfs readdir 串行 | 参数字符串 `func.readdir=cor:4` 硬编码到 `prepare_mergerfs_check` 注释 + echo 日志（Phase 29 静态 grep 可断言；真正挂载由 Phase 31 消费） | Task 3.3（注释 + echo） |
| **C2** category.create=pfrd 随机落盘 | 参数字符串 `category.create=ff` + `inodecalc=path-hash` 硬编码到 `prepare_mergerfs_check` 注释 + echo 日志；Phase 31 mount 时消费 | Task 3.3（注释 + echo） |
| **C3** sshfs 抖动级联 mergerfs 挂死 | entrypoint 不改 sshfs 参数；镜像层只保证 `sshfs --version` 可用；Phase 31 传入稳定参数 | Task 3.3（注释说明） |
| **C5** Mutagen 首次同步清空 /workspace | 二次 chown 兜底（named volume 首挂权限差异修复） | Task 3.3 prepare_v3_dirs |
| **C7** systemd-logind 杀 tmux | entrypoint 不启动 systemd；PID 1 tini（Plan 01） | Task 3.3（反向：不添加 systemd 调用） |
| **M4** entrypoint 顺序错误 | 严格串行：prepare_v3_dirs → prepare_mutagen_agent → prepare_mergerfs_check → assert_tmux_version → exec sshd；任一失败 exit 1 | Task 3.3 |
| **M7** tmux 多端尺寸 | tmux.conf `window-size latest` + `aggressive-resize on` | Task 3.1 |
| **M8** Claude Code 颜色灰 | tmux.conf `terminal-overrides ",*:RGB"` + profile.d `CLAUDE_CODE_TMUX_TRUECOLOR=1` | Task 3.1 |
| **M12** ControlMaster MaxSessions=10 | sshd_config `MaxSessions 30` + `MaxStartups 60:30:120` | Task 3.2 |
| **M17** named volume UID 差异 | entrypoint 二次 chown（Plan 01 预建 + 本 plan 运行时兜底） | Task 3.3 prepare_v3_dirs |

---

## Risks / Unknowns

1. **mutagen tarball 内部目录结构影响 `prepare_mutagen_agent` 解压路径**
   - 假设：`/opt/mutagen-agents.tar.gz` 解开后顶层是 `linux_amd64/mutagen-agent` 等架构子目录（由 Mutagen 官方打包约定）
   - Fallback：若解压后 `/usr/local/libexec/mutagen/agents/` 下多了一层（例如 `agents/agents/linux_amd64/...`），调整 `tar -xzf` 的 `--strip-components=N`
   - Executor 首次构建后应 `docker exec <c> ls /usr/local/libexec/mutagen/agents/` 核对目录布局与 Phase 31 Mutagen daemon 的期望路径一致

2. **`assert_tmux_version` 正则覆盖范围**
   - 当前 case 覆盖 `3.4-3.9` 与 `4.x+`；若未来 Ubuntu 24.04 更新到 tmux `3.10` 或更大版本号位数，需要再扩展正则
   - Fallback：改用 `awk -F. '{ if ($1 >= 4) exit 0; if ($1 == 3 && $2 >= 4) exit 0; exit 1 }'` 做纯数字比较，避免 case 漏匹配

2.1 **`window-size latest` / `aggressive-resize on` 与 tmux apt 版本兼容性（R4）**
   - `window-size latest` 引入于 **tmux 2.9**，`aggressive-resize on` 自 tmux 1.8 起可用；Ubuntu 24.04 apt tmux 当前版本 = **3.4**，已远超 2.9 下限，理论上无兼容性风险
   - 若未来 apt 仓库回滚至 < 2.9（极不可能），`tmux -f /etc/tmux.conf` 会在启动时报 `unknown option` → interactive session 可能退化但不致命；本 plan 的 `assert_tmux_version` fail-fast 会在 < 3.4 时直接 `exit 1`，提前拦截此类回滚场景
   - Fallback：无需处理；`assert_tmux_version` 的 ≥ 3.4 硬 gate 是该风险的最终兜底

3. **`/etc/profile.d/cloud-claude.sh` 的读取时机**
   - 仅 interactive login shell 会 source `/etc/profile.d/*.sh`；`ssh <host> <command>` 非 login 模式不读取
   - cloud-claude 的 `tmux new-session -A` 会通过 tmux login shell 触发 profile.d 读取；但若 Phase 32 改为 `ssh -t container claude ...` 之类非 tmux 路径，`CLAUDE_CODE_TMUX_TRUECOLOR` 不会被 export
   - Fallback：Phase 32 可以在 tmux start 命令前显式 `export CLAUDE_CODE_TMUX_TRUECOLOR=1`；本 plan 已做镜像默认，Phase 32 按需加强

4. **`prepare_mutagen_agent` 幂等标记 `.extracted`**
   - 使用 `[[ ! -f "$dest/.extracted" ]]` 作为幂等门；若 tarball 版本被升级（比如 Phase 35 回流升级到 Mutagen 0.19），`.extracted` 存在会导致新版本不被解压
   - Fallback：Phase 35 升级 Mutagen 版本时，entrypoint 应改为根据 `/etc/cloud-claude/mutagen.version` 与 `.extracted-v0.18.1` 这种带版本的 marker 判断；本 plan 不前置处理，只在注释留 TODO

---

*End of Plan 03-entrypoint-config*
