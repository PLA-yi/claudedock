---
phase: 29-v3-worker
plan: 05-host-preflight-docs
sub_scope: E
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/scripts/host-preflight.sh
  - deploy/README.md
autonomous: true
requirements:
  - D-23
  - D-24
  - C6
must_haves:
  truths:
    - "host-preflight.sh 扩展新增 check_apparmor_fusermount3 检测函数（不新建文件）"
    - "AppArmor override 路径统一为 /etc/apparmor.d/local/fusermount3（D-23 已修正；非 docker-default）"
    - "检测仅在 Ubuntu 25.04+（/etc/os-release ID=ubuntu + VERSION_ID >= 25.04）执行；其他发行版 skip"
    - "触发条件：aa-status 输出含 fusermount3 且无 capability dac_override 例外规则"
    - "脚本只提供修复命令提示，不自动执行 apparmor_parser -r / 不自动写文件（D-24 AUTO_FIX 延后）"
    - "deploy/README.md 新增或扩展 v3.0 AppArmor override 章节（中文）"
  artifacts:
    - path: "deploy/scripts/host-preflight.sh"
      provides: "check_apparmor_fusermount3 shell 函数 + 调用入口"
      contains: "check_apparmor_fusermount3"
    - path: "deploy/README.md"
      provides: "v3.0 AppArmor override 运维章节"
      contains: "/etc/apparmor.d/local/fusermount3"
  key_links:
    - from: "main() 调用入口"
      to: "check_apparmor_fusermount3"
      via: "bash 函数调用"
      pattern: "check_apparmor_fusermount3"
---

## Goal

**扩展**（不是新建）现有 `deploy/scripts/host-preflight.sh`，追加一个 Ubuntu 25.04+ AppArmor `fusermount3` `capability dac_override` 检测函数，当检测到缺失时打印清晰的中文修复指引（写文件 + `apparmor_parser -r`），并补充 `deploy/README.md` 的 v3.0 运维章节。**不自动修复**（D-24 AUTO_FIX 延后），**不**覆盖其他发行版（Debian / CentOS / WSL skip）。

对应 Sub-scope：**E host-preflight & 运维文档**（29-RESEARCH.md §Sub-scope 映射）。

---

## Scope

### In
1. `deploy/scripts/host-preflight.sh`（**现网 44 行，扁平 top-level 命令结构；仅有 `require_cmd` 一个 helper；无 `log_*` / `main()` / `check_*` 函数**）：
   - 在 `require_cmd` helper 函数定义（现 4-9 行）**之后**、第一条 `require_cmd docker`（现 11 行）**之前** 追加 `check_apparmor_fusermount3()` 函数定义
   - 函数内部遵循现网风格（`echo ... >&2` + `return 1` / `return 0`，非 `exit`；`set -euo pipefail` 语境下用 `return` 传递失败信号）：
     - OS 门禁：非 Ubuntu → `echo ... >&2` 后 `return 0`（函数级 skip，不打断后续检查）
     - 版本门禁：Ubuntu < 25.04 → `echo ... >&2` 后 `return 0`
     - 工具门禁：无 `aa-status` 命令 → `echo ... >&2` 提示 `apt-get install apparmor-utils` 后 `return 0`（工具缺失视为跳过，advisory）
     - Profile 门禁：`aa-status` 无 `fusermount3` profile → `echo ... >&2` 后 `return 0`（内核禁用 AppArmor 或早于 25.04 内核的场景）
     - 例外规则检测：`grep -qE '^\s*capability\s+dac_override' /etc/apparmor.d/local/fusermount3 2>/dev/null` → 若缺失则 `echo ... >&2` 打印修复指引（heredoc 3 行 shell snippet 让 operator 手动粘贴）后 `return 1`
   - 调用入口插在 `require_cmd curl`（现 41 行）**之后**、`mkdir -p /var/lib/cloud-cli-proxy`（现 43 行）**之前**，形式为 `check_apparmor_fusermount3 || true`
   - **`|| true` 必要性**：本检测是 advisory（宿主侧 AppArmor 策略不通过不应阻断 docker / nft / fuse 等核心 preflight 的 `mkdir` 初始化）；若未来引入 `AUTO_FIX=1` 模式再考虑升级为 hard fail
   - **严禁**自动执行 `sudo tee` / `apparmor_parser -r`（D-24 延后）
   - **严禁**引入 `log_info` / `log_warn` / `log_fail` / `log_ok` / `log_skip` / `main()` / `PREFLIGHT_HAD_WARN` 等现网不存在的 helper（会引发 `command not found`；若未来需要统一日志格式应作为独立 refactor 提交，不在本 plan scope）
2. `deploy/README.md` 或（若 README 过短）新建 `deploy/README.md` 的 `## v3.0 AppArmor override 部署` 章节：
   - 描述现象（`mergerfs/fusermount3` mount 在 Ubuntu 25.04+ 报 `permission denied`）
   - 修复步骤（3 行命令 + `apparmor_parser -r`）
   - 何时需要（宿主 Ubuntu ≥ 25.04；Ubuntu 24.04/Debian/CentOS 跳过）
   - 关联：`host-preflight.sh check_apparmor_fusermount3` 检测项

### Out（属于其他 plan 的职责 / 跨 phase defer，本 plan 禁止触碰）
- AppArmor 自动修复 / `AUTO_FIX=1` 环境变量（D-24 明确 deferred；未来阶段引入）
- `host-preflight.sh` 现有 `require_cmd docker/ip/systemctl/nsenter/nft/curl` + FUSE 设备检查 + `mkdir -p /var/lib/cloud-cli-proxy` 等既有逻辑（已存在，本 plan 不动；**不引入** `log_*` / `main()` / `check_*` 统一包装结构）
- C6 防御的 `docker/daemon.json fuse` 配置 → 后续阶段按需补（本 plan 只做 AppArmor 宿主检测）

**其余 5 plan 归属（cross-plan boundary 显式列出，traceability 闭合）：**
- **Plan 01 — Sub-scope A 镜像构建基线 — 负责** `deploy/docker/managed-user/Dockerfile`：BuildKit cache mount / `tini` 安装 / 预建 `/home/claude` 家族 + `chown 1000:1000` / ENTRYPOINT tini 化（与本 plan 无文件交集）
- **Plan 02 — Sub-scope B 二进制预置 — 负责** `deploy/docker/managed-user/Dockerfile`：mergerfs 2.41.1 `.deb` 下载 + dpkg 安装 / Mutagen agent tarball 预放 `/opt/` / 版本元数据 `/etc/cloud-claude/*.version`（与本 plan 无文件交集）
- **Plan 03 — Sub-scope C entrypoint & 配置 — 负责** `deploy/docker/managed-user/{entrypoint.sh,tmux.conf,profile.d-cloud-claude.sh,sshd_config,Dockerfile}`：v3 串行阶段函数 / tmux + profile.d 静态配置 / sshd KeepAlive + MaxSessions（与本 plan 无文件交集）
- **Plan 04 — Sub-scope D Worker volumes contract — 负责** `internal/agentapi/contracts.go` + `internal/runtime/tasks/worker.go` + `internal/runtime/tasks/worker_volume_test.go`：`VolumeMount` 结构 + `HostActionRequest.Volumes` 字段 + `--mount type=volume` args 拼接 + 测试（与本 plan 无文件交集）
- **Plan 06 — Sub-scope F image.lock + CI gate — 负责** `deploy/docker/managed-user/image.lock` + `.github/workflows/build-images.yml`：6 字段能力清单追加 + `docker image inspect` ≤ 700 MiB 硬 gate（与本 plan 无文件交集）

本 plan 仅改 `deploy/scripts/host-preflight.sh` + `deploy/README.md`，与其他 5 plan 零文件重叠，可 Wave 1 并行。

---

## Dependencies

- **None**（Wave 1，可与 Plan 01 / 04 / 06 并行）
- 本 plan 不依赖任何 v3 镜像产物；是"宿主侧运维工具"，完全正交

---

## Tasks

### Task 5.1 — host-preflight.sh 追加 check_apparmor_fusermount3 函数

**文件：** `deploy/scripts/host-preflight.sh`（现网 44 行）

**改动要点：**

**插入点（严格）：**
- 函数定义：紧随 `require_cmd` helper 结束行（现 9 行 `}`）**之后**、空行保留，再紧邻第一条 `require_cmd docker`（现 11 行）**之前** 追加一个空行 + 函数定义
- 调用入口：在 `require_cmd curl`（现 41 行）**之后**、`mkdir -p /var/lib/cloud-cli-proxy`（现 43 行）**之前** 插入单行调用 `check_apparmor_fusermount3 || true`（加注释说明 advisory）

**函数体（严格遵循现网 `echo >&2` + `exit/return` 风格，不引入任何 `log_*` helper）：**

  ```bash
  # check_apparmor_fusermount3 — Ubuntu 25.04+ AppArmor override advisory check.
  # D-23: override path is /etc/apparmor.d/local/fusermount3 (not docker-default).
  # D-24: detect + print manual fix instructions only; never auto-apply.
  # Style: matches existing `require_cmd` / inline `echo ... >&2` pattern in this file;
  #        intentionally NOT introducing log_* helpers (would diverge from current script).
  check_apparmor_fusermount3() {
    # --- OS gate: only Ubuntu 25.04+ is affected ---
    if [ ! -f /etc/os-release ]; then
      echo "host-preflight: /etc/os-release missing; skipping AppArmor fusermount3 check" >&2
      return 0
    fi
    # shellcheck source=/dev/null
    . /etc/os-release
    if [ "${ID:-}" != "ubuntu" ]; then
      echo "host-preflight: non-Ubuntu host (${ID:-unknown}); skipping AppArmor fusermount3 check" >&2
      return 0
    fi
    # VERSION_ID 示例：25.04 / 25.10 / 24.04 / 26.04
    local ubuntu_major ubuntu_minor
    ubuntu_major=${VERSION_ID%%.*}
    ubuntu_minor=${VERSION_ID#*.}
    ubuntu_minor=${ubuntu_minor%%.*}   # 去掉潜在的 patch 段（如 25.04.1）
    if [ "${ubuntu_major}" -lt 25 ] || { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor}" -lt 4 ]; }; then
      echo "host-preflight: Ubuntu ${VERSION_ID} < 25.04; skipping AppArmor fusermount3 check" >&2
      return 0
    fi

    # --- Tool gate: aa-status 缺失视为 advisory skip（不强制安装 apparmor-utils）---
    if ! command -v aa-status >/dev/null 2>&1; then
      echo "host-preflight: aa-status not installed; install with: apt-get install -y apparmor-utils" >&2
      echo "host-preflight: skipping AppArmor fusermount3 check (advisory)" >&2
      return 0
    fi

    # --- Profile gate: fusermount3 profile 未加载（内核禁用 AppArmor / 早于 25.04 内核）---
    if ! aa-status 2>/dev/null | grep -q 'fusermount3'; then
      echo "host-preflight: AppArmor fusermount3 profile not loaded; nothing to override" >&2
      return 0
    fi

    # --- Override rule detection ---
    local override=/etc/apparmor.d/local/fusermount3
    if [ ! -f "${override}" ] || ! grep -qE '^[[:space:]]*capability[[:space:]]+dac_override' "${override}" 2>/dev/null; then
      echo "host-preflight: FAIL AppArmor override missing — ${override} lacks 'capability dac_override,'" >&2
      cat >&2 <<'FIX_INSTRUCTIONS'

  To fix on Ubuntu 25.04+ (run as root on the host):

      sudo tee /etc/apparmor.d/local/fusermount3 >/dev/null <<'APPARMOR'
      # Cloud CLI Proxy v3.0 — allow mergerfs DAC override for multi-branch readdir
      capability dac_override,
      APPARMOR

      sudo apparmor_parser -r /etc/apparmor.d/fusermount3

  See deploy/README.md §v3.0 AppArmor override 部署 for rationale.

  FIX_INSTRUCTIONS
      return 1
    fi

    echo "host-preflight: AppArmor fusermount3 override OK" >&2
    return 0
  }
  ```

**调用行（插入点见上）：**

  ```bash
  # Phase 29: Ubuntu 25.04+ AppArmor advisory check (D-23 / D-24; advisory — non-blocking).
  check_apparmor_fusermount3 || true
  ```

**严禁清单：**
- **严禁**引入 `log_info` / `log_warn` / `log_fail` / `log_ok` / `log_skip` / `main()` / `PREFLIGHT_HAD_WARN` 等现网脚本不存在的 helper（会引发 `command not found` 直接 `set -e` 中断）
- **严禁**把调用改为 `check_apparmor_fusermount3` 裸调（不加 `|| true`）—— 本检测失败不应阻断后续 `mkdir -p /var/lib/cloud-cli-proxy` 等核心初始化
- **严禁**执行 `sudo tee` / `apparmor_parser -r` 的自动修复逻辑（D-24：AUTO_FIX=1 明确 defer）
- **严禁**硬编码绝对路径到 `/Users/` / `/home/zaneliu` 等（CLAUDE.md 规范）
- **严禁**把非 Ubuntu / 版本不足分支写成 `exit 0` —— 必须是 `return 0`，否则会终止整个宿主 preflight 流程

**对应：** D-23（AppArmor 路径 `/etc/apparmor.d/local/fusermount3`）/ D-24（仅检测 + 打印）
**PATTERNS：** S1（`#!/usr/bin/env bash` + `set -euo pipefail` 骨架继承）、S2（复用 `require_cmd` helper 风格 —— 失败统一 `echo ... >&2` + `exit/return` 传递；本 plan **不引入** `log_*` helper）、S5 fallback（heredoc 输出修复指引，执行体外部，仅 stderr）、AP3（脚本只检测不执行 `apparmor_parser -r`）、AP8（不使用 `docker-default` 路径 → `fusermount3`）、AP9（扩展现有 `deploy/scripts/host-preflight.sh` 而非新建 `deploy/host-preflight.sh`；与 CONTEXT D-23 原文路径的修正点）
**参考：** RESEARCH §8 研究结论（AppArmor 路径修正）+ §Code Examples §host-preflight.sh 骨架；DISCUSSION-LOG 2026-04-18 D-23 路径确认

### Task 5.2 — deploy/README.md 新增 v3.0 AppArmor override 章节

**文件：** `deploy/README.md`

**改动要点：**
- 先 `ls deploy/README.md` 确认是否存在；若存在读最后一段，在文档**末尾**追加本章节（不覆盖现有章节）；若不存在则创建最小化 README 骨架：
  ```
  # Cloud CLI Proxy — Deploy 运维手册

  本目录收纳宿主侧部署与运维脚本、配置样例与故障处置指引。

  ## v3.0 AppArmor override 部署
  ...（见下）
  ```
- 追加的 `## v3.0 AppArmor override 部署` 章节固定包含 4 小节：

  ```markdown
  ## v3.0 AppArmor override 部署

  ### 适用范围
  仅 Ubuntu 25.04 及以上宿主机需要执行本节；Ubuntu 24.04 / Debian / CentOS / RHEL 跳过。

  ### 背景
  Ubuntu 25.04 起默认加载了针对 `fusermount3` 的 AppArmor profile，拒绝 `capability dac_override`。
  受管镜像 v3.0 在容器内以 root 身份挂载 `mergerfs`，需要此能力写入 workspace 数据盘。
  若未追加 override，容器启动时 `mergerfs` 会报 `permission denied`，受管镜像 entrypoint 会 fail-fast 失败。

  ### 修复步骤
  在宿主执行（需要 root 权限）：

      sudo tee /etc/apparmor.d/local/fusermount3 >/dev/null <<'APPARMOR'
      # Cloud CLI Proxy v3.0 — allow mergerfs DAC override for multi-branch readdir
      capability dac_override,
      APPARMOR

      sudo apparmor_parser -r /etc/apparmor.d/fusermount3

  修复后建议再执行 `deploy/scripts/host-preflight.sh` 验收，直到 AppArmor fusermount3 检测项输出 OK。

  ### 自动检测
  运行 `deploy/scripts/host-preflight.sh` 会调用 `check_apparmor_fusermount3`：
  - Ubuntu 25.04+ 且 override 缺失 → 输出 FAIL + 修复指引
  - Ubuntu 24.04 / 非 Ubuntu → 自动跳过
  - AppArmor 未启用 / profile 未加载 → 自动跳过

  > 注：v1 版本不做自动修复（避免未经授权修改宿主安全策略）；`AUTO_FIX=1` 由后续阶段引入。
  ```

- **严禁**：README 中出现 `/Users/` / `/home/zaneliu` 等绝对路径；所有路径用相对仓库根（CLAUDE.md 规范）
- **严禁**：README 中出现真实 IP / 密码 / token（CLAUDE.md 规范）

**对应：** D-23 / D-24 文字化；CLAUDE.md §文档（中文）/ §隐私与安全
**PATTERNS：** 文档条目复用（中文正文 + 英文路径/命令）

---

## Verification

### 静态断言

```bash
# Task 5.1 — host-preflight.sh
grep -F 'check_apparmor_fusermount3' deploy/scripts/host-preflight.sh
grep -F '/etc/apparmor.d/local/fusermount3' deploy/scripts/host-preflight.sh
grep -F 'capability dac_override' deploy/scripts/host-preflight.sh
grep -F 'apparmor_parser -r' deploy/scripts/host-preflight.sh
# D-24 反向断言：不自动执行修复（修复代码仅位于 heredoc 内部 stderr 输出，不在可执行路径上）
! grep -E '^[[:space:]]*sudo[[:space:]]+tee[[:space:]]+/etc/apparmor\.d/local/fusermount3' deploy/scripts/host-preflight.sh
! grep -E '^[[:space:]]*sudo[[:space:]]+apparmor_parser[[:space:]]+-r' deploy/scripts/host-preflight.sh
# OS 门禁
grep -F '/etc/os-release' deploy/scripts/host-preflight.sh
grep -F 'ubuntu' deploy/scripts/host-preflight.sh
# 工具门禁
grep -F 'aa-status' deploy/scripts/host-preflight.sh
# 调用入口断言：调用行存在且带 `|| true`（advisory 非阻断）
grep -F 'check_apparmor_fusermount3 || true' deploy/scripts/host-preflight.sh
# 调用位置断言：必须位于 `require_cmd curl` 之后、`mkdir -p /var/lib/cloud-cli-proxy` 之前
awk '
  /^require_cmd curl$/             { curl=NR }
  /^check_apparmor_fusermount3[[:space:]]*\|\|[[:space:]]*true/ { call=NR }
  /^mkdir -p \/var\/lib\/cloud-cli-proxy/ { mkdir=NR }
  END { exit !(curl>0 && call>curl && mkdir>call) }
' deploy/scripts/host-preflight.sh
# R1 反向断言：严禁引入现网不存在的 log_* / main / PREFLIGHT_HAD_WARN helper
! grep -Eq '^(log_info|log_warn|log_fail|log_ok|log_skip)[[:space:]]*\(' deploy/scripts/host-preflight.sh
! grep -Eq '^main[[:space:]]*\(\)' deploy/scripts/host-preflight.sh
! grep -F 'PREFLIGHT_HAD_WARN' deploy/scripts/host-preflight.sh
# R1 反向断言：非 Ubuntu / 版本不足分支用 `return 0` 而非 `exit 0`（否则会中断后续 preflight）
! awk '/check_apparmor_fusermount3\(\)/{in_fn=1} in_fn && /^}/{in_fn=0} in_fn && /exit 0/' deploy/scripts/host-preflight.sh

# shellcheck 无新增 error（仅关心 error，warning 按现脚本基线）
command -v shellcheck >/dev/null && shellcheck -S error deploy/scripts/host-preflight.sh

# bash -n 语法检查
bash -n deploy/scripts/host-preflight.sh

# Task 5.2 — README.md
test -f deploy/README.md
grep -F 'v3.0 AppArmor override' deploy/README.md
grep -F '/etc/apparmor.d/local/fusermount3' deploy/README.md
grep -F 'apparmor_parser -r' deploy/README.md
# 中文正文（CLAUDE.md 要求）
grep -F '适用范围' deploy/README.md
grep -F '修复步骤' deploy/README.md
# 反向断言：无绝对本机路径
! grep -E '/(Users|home/zaneliu)/' deploy/README.md
```

### 动态断言（模拟执行）

```bash
# 在 macOS / 非 Ubuntu 宿主执行：应全部 skip
bash deploy/scripts/host-preflight.sh 2>&1 | tee /tmp/preflight-macos.log
grep -E 'AppArmor fusermount3.*skipped' /tmp/preflight-macos.log

# Ubuntu 24.04 docker 容器模拟（executor 如有条件）：
docker run --rm -v "$(pwd)/deploy/scripts/host-preflight.sh:/p.sh:ro" \
  ubuntu:24.04 bash -c 'apt-get update -qq && apt-get install -y apparmor-utils -qq && bash /p.sh 2>&1 || true' \
  | grep -E 'AppArmor fusermount3.*skipped'
# 预期：24.04 因 VERSION_ID < 25.04 skip；aa-status profile 为空也 skip

# Ubuntu 25.04 模拟（executor 条件允许时）：
#   无 override → FAIL + 修复指引包含 capability dac_override
#   有 override → OK
```

### Coverage contribution

> **Coverage contribution:** V-07（host-preflight Ubuntu 25.04+ AppArmor 检测通过）→ 本 plan 负责的检测函数 + 文档形成闭环；SC6 由 Plan 03（entrypoint fail-fast）+ 本 plan（宿主侧提前告警）双保险共同达成。
>
> **Pitfall coverage:** C6（Ubuntu 25.04+ AppArmor 阻断 mergerfs 挂载）→ 本 plan 通过"preflight 检测 + README 文档"承担 90% 风险；剩余 10%（operator 无视 preflight 告警继续启动）由 Plan 03 entrypoint `prepare_mergerfs_check` fail-fast 兜底。

---

## Atomic Commit Strategy

2 个原子 commit：

1. `feat(29-05): host-preflight check_apparmor_fusermount3 for Ubuntu 25.04+`
   - Task 5.1（`host-preflight.sh` 扩展）
2. `docs(29-05): deploy README v3.0 AppArmor override section`
   - Task 5.2（`deploy/README.md` 追加/新建章节）

---

## Pitfalls 防御

| Pitfall | 防御手段 | 本 plan 对应任务 |
|---------|---------|-----------------|
| **C6** Ubuntu 25.04+ AppArmor 阻断 mergerfs | preflight 检测 `/etc/apparmor.d/local/fusermount3` + README 修复指引 | Task 5.1 + 5.2 |
| **AP3**（PATTERNS）自动修复宿主安全策略 | 脚本只打印指引；`sudo tee` / `apparmor_parser -r` 由 operator 手动粘贴执行 | Task 5.1（反向断言） |
| **CLAUDE.md 隐私** 绝对路径/真实凭据 | README + 脚本内只用相对路径 + 占位符 | Task 5.1 + 5.2 |

---

## Risks / Unknowns

1. **`aa-status` 在容器内无法看到宿主 AppArmor 状态**
   - 若 operator 把 `host-preflight.sh` 跑在容器里，`aa-status` 可能返回空 → 会走 `profile not loaded` 分支 skip，错过真实问题
   - README 要注明"必须在宿主 root 上下文执行"（本 plan 已覆盖：脚本顶部已有现存 root 校验 `check_root`）
   - Fallback：若 operator 误跑容器内，preflight 会 skip 而非 fail，这是可接受的"假阴性"；Phase 03 entrypoint fail-fast 会在容器启动时报错兜底

2. **Ubuntu `VERSION_ID` 的版本比较精度**
   - `25.04` / `25.10` / `26.04` / `25.04.1` 等多种形态；上面 `${VERSION_ID%%.*}` + `${VERSION_ID##*.}` 的 shell 版本号比较对 "25.04" 正确但对 "25.4"（理论不存在但防御性）可能不正确
   - Fallback：用 `dpkg --compare-versions "${VERSION_ID}" ge 25.04`（Ubuntu 必装 dpkg）比 shell string 对比更稳；若 executor 评估后采用 `dpkg`，请同步更新 Task 5.1 代码片段注释

3. **`aa-status` 输出格式变动**
   - Ubuntu 25.04+ `aa-status` 输出若改 JSON 模式默认，`grep -q fusermount3` 仍能命中（profile 名是机器可读标识），风险低
   - Fallback：无需处理；executor 实测时留意

4. **README 现有内容未知**
   - 若 `deploy/README.md` 已有 `## v3.0 AppArmor override 部署` 同名章节（Phase 27/28 可能已加了雏形）→ 合并而非新增
   - Fallback：executor 先 `grep -n '^##' deploy/README.md` 看现有 TOC，若重名则替换/合并；本 plan 只要最终文本满足 Verification grep 即可

---

*End of Plan 05-host-preflight-docs*
