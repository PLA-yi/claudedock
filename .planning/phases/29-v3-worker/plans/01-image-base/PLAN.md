---
phase: 29-v3-worker
plan: 01-image-base
sub_scope: A
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/docker/managed-user/Dockerfile
autonomous: true
requirements:
  - BASE-04
  - C5
  - C7
  - M17
  - M18
must_haves:
  truths:
    - "镜像仍以 ubuntu:24.04 为 base，v1.2 遗留的 KasmVNC/Chromium/Fluxbox 层不被破坏"
    - "BuildKit cache mount 对 /var/cache/apt + /var/lib/apt/lists 生效，apt 层可复用 cache"
    - "tini 作为 PID 1，ENTRYPOINT 以 exec form 指向 /usr/bin/tini → entrypoint.sh"
    - "/home/claude, /home/claude/.claude, /home/claude/.cache/claude, /workspace-hot, /workspace-cold, /var/lib/claude-persist 在镜像中已预建且属主 1000:1000；/workspace 预建路径沿用 v2.0 不重建"
    - "locale / ENV / WORKSPACE 用户保持与 v2.0 一致，workspace 用户未被替换或重命名"
  artifacts:
    - path: "deploy/docker/managed-user/Dockerfile"
      provides: "v3 基础镜像骨架（BuildKit cache、tini、预建目录 + chown、ENTRYPOINT tini 化）"
      contains: "tini"
  key_links:
    - from: "Dockerfile apt 清单"
      to: "/usr/bin/tini"
      via: "apt-get install -y --no-install-recommends tini"
      pattern: "tini"
    - from: "Dockerfile ENTRYPOINT"
      to: "/usr/local/bin/entrypoint.sh"
      via: "tini -- entrypoint.sh"
      pattern: '"/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"'
---

## Goal

改造 `deploy/docker/managed-user/Dockerfile` 的基础结构，让它具备承载 v3 组件所需的构建特性：BuildKit apt cache mount、`--no-install-recommends` 已在但需确认、`tini` 作为 PID 1、预建 v3 目录并 `chown 1000:1000`、ENTRYPOINT 切换到 `tini` exec form。本 plan **不**下载 mergerfs / mutagen 二进制，**不**改动 entrypoint.sh 的逻辑分支，**不**改 image.lock 或 CI。

对应 Sub-scope：**A 镜像构建基线**（29-RESEARCH.md §Sub-scope 映射）。

> **Wave Coordination / Reusable Assets 备注（R5）：** `29-PATTERNS.md §Reusable Assets §Dockerfile` D1（KasmVNC `.deb` 下载 pattern，现 `deploy/docker/managed-user/Dockerfile:47-54`）是 **Plan 02** 下载 mergerfs / mutagen 的直接 analog；本 plan **保留 Dockerfile 现 47-54 行 KasmVNC RUN 不动**，以便 Plan 02 在 Wave 2 按 D1 模板在 Chromium RUN 之后 / `locale-gen` RUN 之前追加 mergerfs `.deb` 下载 RUN。故本 plan 未在任务中引用 D1（不是遗漏而是 scope 刻意排除），traceability 由此声明闭合。

---

## Scope

### In
1. **BuildKit 启用 + apt cache mount 样板**
   - 在 Dockerfile 顶部加 `# syntax=docker/dockerfile:1.7` 指令
   - 插入"去 docker-clean + keep-cache"配置 RUN（PATTERNS 代码示例 §Code Examples / BuildKit apt cache mount 段）
   - 现有 apt 合并 RUN（当前 `9-41` 行）切换为 `RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt/lists,sharing=locked ...`；继续保留 `rm -rf /var/lib/apt/lists/*` 收尾（与 cache mount 共存：cache mount 指向 `/var/lib/apt/lists`，`rm` 清的是镜像层里的副本，不影响 cache tar）
2. **apt 清单增加 `tini`**（单 RUN 追加一行，字母序插入）— D-08 的 libfuse3 保持现状（fuse3 已在清单）
3. **预建 v3 目录 + chown**：新增一条 `RUN mkdir -p /home/claude /home/claude/.claude /home/claude/.cache/claude /workspace-hot /workspace-cold /var/lib/claude-persist && chown -R ${WORKSPACE_UID}:${WORKSPACE_GID} /home/claude /workspace-hot /workspace-cold /var/lib/claude-persist`
   - **不**对 `/workspace` 再次 chown（entrypoint.sh:90 已负责 runtime chown）
4. **ENTRYPOINT 替换**：现 `ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]` 改为 `ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`
5. **预留静态 COPY 行占位**（仅注释或不动；实际 COPY 由 Plan 03 追加）

### Out（属于其他 plan 的职责，本 plan 禁止触碰）
- `curl` 下载 mergerfs `.deb` / mutagen tarball、`/etc/cloud-claude/*.version` 写入 → **Plan 02**（会在本 plan 的 RUN 之后串接新 RUN）
- `COPY deploy/docker/managed-user/tmux.conf` / `profile.d-cloud-claude.sh`、entrypoint.sh 内容修改、sshd_config 修改 → **Plan 03**
- `internal/agentapi/contracts.go` / `worker.go` / `worker_volume_test.go` → **Plan 04**
- `deploy/scripts/host-preflight.sh` / 运维文档 → **Plan 05**
- `deploy/docker/managed-user/image.lock` / `.github/workflows/build-images.yml` → **Plan 06**

---

## Dependencies

- **None**（Wave 1 起点之一）
- Plan 02 在本 plan 的 Dockerfile 骨架之上追加 RUN；Plan 03 依赖本 plan 的预建目录（entrypoint 二次 chown 需要目录存在）

---

## Tasks

### Task 1.1 — 启用 BuildKit syntax 声明与 docker-clean 移除

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 第 1 行之前插入 `# syntax=docker/dockerfile:1.7`（PATTERNS D2 延伸）
- 在 `FROM ubuntu:24.04` 与 `ENV DEBIAN_FRONTEND=noninteractive` 之间不动；在 `ENV WORKSPACE_GID=1000`（第 7 行）之后、apt RUN（第 9 行）之前插入一条 RUN：
  ```dockerfile
  # 启用 BuildKit apt cache mount（必须先移除 docker-clean，否则 cache 会被自动清空）
  # 参考：https://docs.docker.com/build/cache/optimize (D-03, M18 防御)
  RUN rm -f /etc/apt/apt.conf.d/docker-clean \
      && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' \
          > /etc/apt/apt.conf.d/keep-cache
  ```

**对应：** D-03（BuildKit cache + `--no-install-recommends` + 合并 RUN） / M18（镜像体积防御）
**PATTERNS：** Sub-scope A 汇总行 A1（新增 BuildKit cache mount）+ D2（apt 合并 RUN）
**Anti-pattern 回避：** 不合并这条 RUN 到 apt 清单 RUN（`/etc/apt/apt.conf.d/keep-cache` 必须早于任何 apt-get update 存在）

### Task 1.2 — apt 清单 RUN 改为 cache-mount 版本并追加 `tini`

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 当前第 9-41 行单 RUN 的 `RUN apt-get update && apt-get install -y --no-install-recommends ...` 改为：
  ```dockerfile
  RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
      --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
      apt-get update \
      && apt-get install -y --no-install-recommends \
          openssh-server \
          bash \
          zsh \
          curl \
          git \
          tmux \
          sudo \
          ca-certificates \
          jq \
          procps \
          iproute2 \
          nodejs \
          npm \
          locales \
          fluxbox \
          pcmanfm \
          dbus-x11 \
          fonts-liberation \
          fonts-noto-cjk \
          xdg-utils \
          xclip \
          xsel \
          gnupg \
          libegl1 \
          libgl1 \
          x11-utils \
          x11-xserver-utils \
          xterm \
          sshfs \
          fuse3 \
          tini \
      && rm -rf /var/lib/apt/lists/*
  ```
- **插入点是字母序**：`tini` 排在 `sshfs`/`fuse3` 之后（现清单未严格字母序，延续原顺序，`tini` 紧随 `fuse3`）
- 不新增其它包；**禁止**把 `tini` 另起 RUN（D-03 要求合并；重复 RUN 增加 layer 数）

**对应：** D-03 / D-10（tini 安装）
**PATTERNS：** 汇总行 A 列 A3（tini 安装）+ D2（apt 合并 RUN）+ Code Examples §BuildKit apt cache mount；Anti-pattern AP2（不挤进其它 RUN）不适用于此，但 AP 级别的"不要独立 RUN 装 tini"为本任务的隐式约束
**Pitfalls 防御：** C7（systemd-logind 杀 tmux）依赖 tini 作为 PID 1 + 不装 systemd；M18（镜像体积）依赖 cache mount 复用

### Task 1.3 — 预建 v3 目录 + chown 1000:1000

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 在现 `RUN mkdir -p /workspace/.ssh /var/run/sshd ...`（当前 85-87 行）**之后**、`COPY ...sshd_config ...`（当前 89 行）**之前**插入新 RUN：
  ```dockerfile
  # v3: 预建 mergerfs / mutagen / 持久化 volume 挂载点 + chown 到 workspace 用户。
  # C5 / M17 防御：named volume 首次挂载会继承镜像中目标路径的权限；
  # entrypoint 会在运行时再做一次 chown 作为二次保险（Plan 03 prepare_v3_dirs）。
  RUN mkdir -p \
          /home/claude \
          /home/claude/.claude \
          /home/claude/.cache/claude \
          /workspace-hot \
          /workspace-cold \
          /var/lib/claude-persist \
      && chown -R "${WORKSPACE_UID}:${WORKSPACE_GID}" \
          /home/claude \
          /workspace-hot \
          /workspace-cold \
          /var/lib/claude-persist
  ```
- **严禁**把 `/workspace` 纳入 chown 列表（AP10：`/workspace` 由现 85-87 RUN + entrypoint 负责，重复 chown 触发 C5 邻近坑位）

**对应：** D-16（预建目录清单） / D-17（不换用户）
**PATTERNS：** 汇总行 A 列 A5（新增预建目录 + chown）；AP10（不重复 chown /workspace）
**Pitfalls 防御：** C5 / M17（named volume UID 差异）

### Task 1.4 — ENTRYPOINT 切换到 tini exec form

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 当前第 102 行 `ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]` 原地替换为：
  ```dockerfile
  ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
  ```
- **保持 exec form JSON 数组**（PATTERNS D5）；不改 `EXPOSE` / `WORKDIR`；不新增 STOPSIGNAL 等指令

**对应：** D-10（tini PID 1 改造）
**PATTERNS：** 汇总行 A 列 A3 + D5（ENTRYPOINT exec form）
**Pitfalls 防御：** C7（systemd-logind 杀 tmux 的根因之一是错误 PID 1 管 SSH session）

---

## Verification

### 静态断言（build / grep / bash）

1. **`docker build` 成功**（仅跑到 apt 清单阶段即可，由本 plan 范围验证；后续 mergerfs/mutagen 下载属于 Plan 02，跨 plan 时分开构建）：
   ```bash
   DOCKER_BUILDKIT=1 docker build -f deploy/docker/managed-user/Dockerfile -t local/managed-user:p01-test .
   ```
   若 Plan 02/03 尚未落，apt 清单外新增的下载/COPY 也未落；本 plan 的构建检验应在 Plan 01 的单点 commit 上跑（可用 `git stash` 或 `git checkout <commit>` 隔离）。

2. **Dockerfile 静态内容断言：**
   ```bash
   grep -F '# syntax=docker/dockerfile:1.7' deploy/docker/managed-user/Dockerfile           # Task 1.1
   grep -F 'rm -f /etc/apt/apt.conf.d/docker-clean' deploy/docker/managed-user/Dockerfile   # Task 1.1
   grep -F '--mount=type=cache,target=/var/cache/apt' deploy/docker/managed-user/Dockerfile # Task 1.2
   grep -Eq '^[[:space:]]+tini[[:space:]]*\\?$' deploy/docker/managed-user/Dockerfile || \
     grep -Eq 'tini \\' deploy/docker/managed-user/Dockerfile                                # Task 1.2
   grep -F 'mkdir -p' deploy/docker/managed-user/Dockerfile | grep -F '/home/claude'        # Task 1.3
   grep -Eq 'chown -R "\$\{WORKSPACE_UID\}:\$\{WORKSPACE_GID\}"' deploy/docker/managed-user/Dockerfile
   grep -F '"/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"' deploy/docker/managed-user/Dockerfile   # Task 1.4
   ! grep -F '/workspace' deploy/docker/managed-user/Dockerfile | grep -F 'mkdir -p /home/claude'         # /workspace 不在 v3 chown 列表
   ```

### 运行时断言（docker build + docker exec；需 Plan 02/03 一并就位时才能端到端跑）

> 本 plan 单独构建的镜像无 entrypoint.sh 的 v3 阶段、无 mergerfs/mutagen，运行时断言只能验证 "tini PID 1" + "预建目录存在"：

```bash
docker run -d --rm --name mu-p01 --cap-add SYS_ADMIN --device /dev/fuse local/managed-user:p01-test
# PID 1 断言（SC5 子集）
docker exec mu-p01 ps -o pid=,comm= -p 1 | awk '{print $2}' | grep -q '^tini$'
# 预建目录属主断言（SC3 子集 / C5 / M17）
for p in /home/claude /home/claude/.claude /home/claude/.cache/claude \
         /workspace-hot /workspace-cold /var/lib/claude-persist; do
  docker exec mu-p01 stat -c '%u:%g %n' "$p" | grep -E '^1000:1000 ' || exit 1
done
# systemd-logind 不存在（SC5 C7 子集）
! docker exec mu-p01 pgrep -x systemd-logind
```

> 注：entrypoint 现有逻辑会在没有 DISPLAY 的环境下尝试启动 Xvnc；若运行时验证失败（Xvnc 启动错误），可暂时把 ENTRYPOINT 改为 `["/usr/bin/tini", "--", "/bin/bash", "-c", "sleep 30"]` 做冒烟测试，验证完毕切回。**禁止把这个临时改动 commit**。

### Coverage contribution

> **Coverage contribution:** SC3（`/home/claude` 属主 1000:1000） / SC5（PID 1 = tini）→ 本 plan 负责证明镜像层预建目录 + tini 安装 + ENTRYPOINT 正确；端到端由 Plan 03 的 entrypoint 二次 chown + Plan 06 的 CI gate 闭环。
>
> **Pitfall coverage:** C5, C7, M17, M18 → 本 plan 的 Task 1.2（tini 安装）+ Task 1.3（预建 + chown）+ Task 1.1（BuildKit cache）为这些坑位在镜像层的第一道防线（C5/M17 由 Plan 03 二次 chown 兜底）。

---

## Atomic Commit Strategy

拆分为 4 个原子 commit，每个 commit 只改 Dockerfile 一小块，确保 `git bisect` 可用：

1. `feat(29-01): Dockerfile enable BuildKit syntax and apt cache mount baseline`
   - Task 1.1（syntax 指令 + docker-clean 移除 RUN）
2. `feat(29-01): Dockerfile use apt cache mount and install tini for PID 1`
   - Task 1.2（apt 清单 RUN 改为 cache-mount + 追加 tini）
3. `feat(29-01): Dockerfile pre-create v3 mount targets with chown 1000:1000`
   - Task 1.3（新增预建目录 + chown RUN）
4. `feat(29-01): Dockerfile switch ENTRYPOINT to tini exec form`
   - Task 1.4（ENTRYPOINT 行替换）

**合并策略：** 如果 Task 1.1+1.2 合并评审更清晰（两者都是 apt 层），可合为单 commit `feat(29-01): Dockerfile BuildKit cache + tini install`；Task 1.3 / 1.4 保持独立。

---

## Pitfalls 防御

| Pitfall | 防御手段 | 本 plan 对应任务 |
|---------|---------|-----------------|
| **C5** Mutagen 首次同步清空 /workspace | Dockerfile 预建 `/home/claude` 家族并 `chown 1000:1000`；named volume 首挂时继承该权限 | Task 1.3 |
| **C7** systemd-logind 杀 tmux server | PID 1 改为 `tini`，不装 systemd | Task 1.2（安装） + Task 1.4（ENTRYPOINT） |
| **M17** named volume UID 差异 | Dockerfile 阶段先 chown；entrypoint 二次 chown（Plan 03 负责） | Task 1.3（一次 chown） |
| **M18** 镜像体积 > 700MB | BuildKit apt cache mount + 合并 RUN + `--no-install-recommends`（已在） | Task 1.1 + Task 1.2 |

---

## Risks / Unknowns

1. **`fonts-noto-cjk` 仍是镜像体积大头（~220MB）**
   - 本 plan **不**裁剪，遵循 D-03 "超标再优先裁 Chromium recommends / fonts"；若 Plan 06 的 CI gate 实测触发 BASE-04 失败，回到本 plan 把 `fonts-noto-cjk` 换为 `fonts-noto-cjk-core`（Research §镜像体积估算）。
   - Fallback：若 Plan 06 确认仍超标 →（a）`fonts-noto-cjk-core` 替换；（b）mutagen-agents tarball 只保留 `linux/{amd64,arm64}`（由 Plan 02 在解包时 `tar --delete` 后再 gzip 重打包）。两步合计估减 ~230MB。

2. **BuildKit cache 与 `rm -rf /var/lib/apt/lists/*` 共存是否有效**
   - RESEARCH §Code Examples 明确列出二者共存的官方写法；`rm -rf` 删的是镜像层里的副本，不是 cache mount 目录。**需要 executor 首次构建实测 `docker history` 验证层 size 没有回升**。
   - Fallback：若实测 cache 失效 → 去掉 `rm -rf /var/lib/apt/lists/*`，依赖 cache mount 的 ephemeral 特性（需权衡 image 体积影响）。

3. **`${WORKSPACE_UID}:${WORKSPACE_GID}` 在 Dockerfile chown 中展开时机**
   - Dockerfile `ARG`/`ENV` 在 RUN 内 shell 展开；本 plan 用双引号保留展开，避免 `--uid 1000` 硬编码。若某些 BuildKit 版本的 cache 行为对 ENV 展开敏感，可直接写 `chown -R 1000:1000 ...` 作为保险。

4. **ENTRYPOINT exec form JSON 中 `tini --` 的参数传递**
   - `["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]` 是 tini 官方模板，`--` 分隔 tini 自身参数与被 exec 的子进程；已由 RESEARCH §Code Examples 验证。不使用 tini 的"subreaper"启发式选项（默认行为已满足）。

---

*End of Plan 01-image-base*
