---
phase: 29-v3-worker
plan: 02-binaries
sub_scope: B
type: execute
wave: 2
depends_on: ["01-image-base"]
files_modified:
  - deploy/docker/managed-user/Dockerfile
autonomous: true
requirements:
  - M3
  - BASE-04
must_haves:
  truths:
    - "mergerfs 2.41.1 `.deb`（匹配 dpkg arch）已通过 sha256 校验并 dpkg -i 安装；`mergerfs --version` 输出含 2.41.1"
    - "mutagen-agents.tar.gz（v0.18.1 配套）放在 /opt/mutagen-agents.tar.gz；tarball 本身保留，不在构建期解包子 tarball"
    - "/etc/cloud-claude/mergerfs.version 文件存在且内容为 `2.41.1`"
    - "/etc/cloud-claude/mutagen.version 文件存在且内容为 `v0.18.1`"
    - "/etc/cloud-claude/tmux.version 文件存在（构建期占位，运行时由 entrypoint 回填）"
  artifacts:
    - path: "deploy/docker/managed-user/Dockerfile"
      provides: "mergerfs `.deb` 下载 + dpkg -i + mutagen tarball 下载 + 版本元数据写入（新增 3 条 RUN）"
      contains: "MERGERFS_VERSION=2.41.1"
    - path: "image:/opt/mutagen-agents.tar.gz"
      provides: "Mutagen agent bundle（多架构，runtime 由 entrypoint 解压）"
    - path: "image:/etc/cloud-claude/*.version"
      provides: "版本元数据文件（C4 防御）"
      contains: "mergerfs.version, mutagen.version, tmux.version"
  key_links:
    - from: "Dockerfile ARG MERGERFS_VERSION=2.41.1"
      to: "https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_${ARCH}.deb"
      via: "curl -fsSL + sha256sum -c -"
      pattern: "mergerfs_2.41.1.ubuntu-noble"
    - from: "Dockerfile ARG MUTAGEN_VERSION=v0.18.1"
      to: "/opt/mutagen-agents.tar.gz"
      via: "curl -fsSL + sha256sum -c - + tar -xzf + mv"
      pattern: "mutagen_linux_.*_v0.18.1.tar.gz"
---

## Goal

在 Plan 01 构建好的 Dockerfile 骨架上，追加 **mergerfs 2.41.1 `.deb` 预装** + **mutagen-agent v0.18.1 tarball 预放** + **版本元数据文件写入** 三段 RUN。所有下载必须通过 `sha256sum -c -` 校验，失败即 build fail；支持 amd64 + arm64 双架构；**不**在构建期解包 `mutagen-agents.tar.gz` 子 tarball（runtime 由 Plan 03 的 entrypoint `prepare_mutagen_agent` 解压）。

对应 Sub-scope：**B 二进制预置**（29-RESEARCH.md §Sub-scope 映射）。

---

## Scope

### In
1. 新增 `ARG MERGERFS_VERSION=2.41.1` + `ARG MERGERFS_SHA256_AMD64=...` + `ARG MERGERFS_SHA256_ARM64=...`（本 plan 必须由 executor 首次构建时 `curl + sha256sum` 实际计算填入——见 RESEARCH §Assumptions A1）
2. 新增 `curl + sha256sum -c - + dpkg -i + rm + mergerfs --version` 单 RUN（mergerfs `.deb` 安装）
3. 新增 `ARG MUTAGEN_VERSION=v0.18.1` + `ARG MUTAGEN_SHA256_AMD64=7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f` + `ARG MUTAGEN_SHA256_ARM64=bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a`（sha256 已由 RESEARCH 给出可直接入 Dockerfile）
4. 新增 `curl + sha256sum -c - + tar -xzf + mv` 单 RUN（mutagen tarball 下载 + 保留 `mutagen-agents.tar.gz`）
5. 新增单 RUN 写入 `/etc/cloud-claude/{mergerfs,mutagen,tmux}.version`

### Out（属于其他 plan 的职责，本 plan 禁止触碰）

**Dockerfile 共享但责任切分（插入点互斥）：**
- **Plan 01 — Sub-scope A 镜像构建基线 — 负责** `Dockerfile` 顶部 BuildKit `# syntax` 指令 / `docker-clean` 移除 RUN / apt 清单 RUN（含 `tini`）加 `--mount=type=cache` / 预建 `/home/claude` 家族目录 + `chown 1000:1000` RUN / `ENTRYPOINT ["/usr/bin/tini", "--", ...]`。本 plan 的 3 条新 RUN **追加** 在 Plan 01 现有 Chromium RUN 之后、`locale-gen` RUN 之前，不修改 Plan 01 任何 RUN。
- **Plan 03 — Sub-scope C entrypoint & 配置 — 负责** `deploy/docker/managed-user/{entrypoint.sh,tmux.conf,profile.d-cloud-claude.sh,sshd_config}` + `Dockerfile` 的 `COPY tmux.conf` / `COPY profile.d-cloud-claude.sh` 两行 + 相应 chmod。Plan 03 的 entrypoint `prepare_mutagen_agent` 在 runtime 解压 `mutagen-agents.tar.gz` 到 `/usr/local/libexec/mutagen/agents/` —— **本 plan 不触碰 tarball 内部，保留为 `.tar.gz` 形式**。
- **Plan 06 — Sub-scope F image.lock + CI gate — 负责** `deploy/docker/managed-user/image.lock` 追加 `mergerfs_version: 2.41.1` / `mutagen_agent_version: v0.18.1` / `tmux_version_min: "3.4"` 等 6 字段 + `.github/workflows/build-images.yml` 的 size gate step。**本 plan 只写镜像内 `/etc/cloud-claude/*.version` 文件**，不触碰仓库内 image.lock / workflow。

**与本 plan 无文件交集的 plan（无需互斥，只需列明以闭合 traceability）：**
- **Plan 04 — Sub-scope D Worker volumes contract — 负责** `internal/agentapi/contracts.go` + `internal/runtime/tasks/worker.go` + `internal/runtime/tasks/worker_volume_test.go`。与本 plan Dockerfile 改动完全正交。
- **Plan 05 — Sub-scope E host-preflight & 运维文档 — 负责** `deploy/scripts/host-preflight.sh` + `deploy/README.md`。与本 plan Dockerfile 改动完全正交。

---

## Dependencies

- **Wave 2**，`depends_on: ["01-image-base"]`
- 原因：本 plan 新增的 RUN 依赖 Plan 01 的 BuildKit cache 样板（否则 `curl` + `dpkg` 无法享用 cache）；以及 Plan 01 的 apt 清单（curl、ca-certificates、dpkg 均在清单中）

---

## Tasks

### Task 2.1 — 新增 mergerfs 2.41.1 `.deb` 下载 + 安装 RUN

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 插入位置：**紧随 Chromium RUN 块**（当前 `rm /etc/apt/sources.list.d/debian-chromium.list ... && rm -rf /var/lib/apt/lists/*` 结束后，大约 64 行后），**在 `RUN locale-gen`（66 行）之前**
- 新增 ARG + RUN：
  ```dockerfile
  # mergerfs 2.41.1 — GitHub release static .deb（D-04 / M3 防御）
  # apt 仓库版本 2.33.5 缺 func.readdir=cor:N / inodecalc=path-hash，不可用。
  ARG MERGERFS_VERSION=2.41.1
  ARG MERGERFS_SHA256_AMD64=""
  ARG MERGERFS_SHA256_ARM64=""
  RUN ARCH=$(dpkg --print-architecture) \
      && case "${ARCH}" in \
           amd64) EXPECTED_SHA="${MERGERFS_SHA256_AMD64}" ;; \
           arm64) EXPECTED_SHA="${MERGERFS_SHA256_ARM64}" ;; \
           *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;; \
         esac \
      && if [ -z "${EXPECTED_SHA}" ]; then \
           echo "MERGERFS_SHA256_${ARCH^^} not set — supply via --build-arg" >&2; exit 1; \
         fi \
      && curl -fsSL -o /tmp/mergerfs.deb \
          "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb" \
      && echo "${EXPECTED_SHA}  /tmp/mergerfs.deb" | sha256sum -c - \
      && dpkg -i /tmp/mergerfs.deb \
      && rm /tmp/mergerfs.deb \
      && mergerfs --version
  ```
- `sha256` 留 ARG 空默认 + 构建时 `--build-arg MERGERFS_SHA256_AMD64=<实际>`；也可由 executor 首次构建后把实际值硬编码到默认 ARG（见 Risks 1）
- **`mergerfs --version` 作为构建期自检**，输出必须含 `2.41.1`

**对应：** D-04（GitHub release `.deb`） / M3（禁止 apt mergerfs） / D-03（合并 RUN）
**PATTERNS：** 汇总行 B 列 + D1（`ARG <COMPONENT>_VERSION=X.Y.Z` + `curl -fsSL -o /tmp/...` KasmVNC `.deb` 模板）；AP5（不走 apt）
**Anti-pattern 回避：** 不把 `.deb` 文件 COPY 进镜像（维护成本更高；每次升级改二进制）；不用 apt repo（AP5 / M3）

### Task 2.2 — 新增 mutagen v0.18.1 tarball 下载 + 保留 mutagen-agents.tar.gz 的 RUN

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 插入位置：**紧随 Task 2.1 的 mergerfs RUN 之后**
- 新增 ARG + RUN：
  ```dockerfile
  # Mutagen agent v0.18.1 — 仅保留 mutagen-agents.tar.gz，runtime 由 entrypoint 解压（D-05）
  ARG MUTAGEN_VERSION=v0.18.1
  ARG MUTAGEN_SHA256_AMD64=7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f
  ARG MUTAGEN_SHA256_ARM64=bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a
  RUN ARCH=$(dpkg --print-architecture) \
      && case "${ARCH}" in \
           amd64) EXPECTED_SHA="${MUTAGEN_SHA256_AMD64}" ;; \
           arm64) EXPECTED_SHA="${MUTAGEN_SHA256_ARM64}" ;; \
           *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;; \
         esac \
      && curl -fsSL -o /tmp/mutagen.tar.gz \
          "https://github.com/mutagen-io/mutagen/releases/download/${MUTAGEN_VERSION}/mutagen_linux_${ARCH}_${MUTAGEN_VERSION}.tar.gz" \
      && echo "${EXPECTED_SHA}  /tmp/mutagen.tar.gz" | sha256sum -c - \
      && mkdir -p /opt \
      && tar -xzf /tmp/mutagen.tar.gz -C /tmp mutagen-agents.tar.gz \
      && mv /tmp/mutagen-agents.tar.gz /opt/mutagen-agents.tar.gz \
      && rm /tmp/mutagen.tar.gz \
      && ls -l /opt/mutagen-agents.tar.gz
  ```
- `tar -xzf ... -C /tmp mutagen-agents.tar.gz` 仅解包顶层 tarball 的一个成员（RESEARCH Assumption A2 已验证 tarball 顶层是 `mutagen` + `mutagen-agents.tar.gz` 两文件，无子目录）
- **不**解压 `mutagen-agents.tar.gz`；保留为 `.tar.gz` 形式（D-05：runtime 由 cloud-claude / entrypoint 触发 extract）

**对应：** D-05（Mutagen tarball 来源） / D-03（合并 RUN）
**PATTERNS：** 汇总行 B 列；Anti-pattern：不把 `mutagen` CLI 二进制也进镜像（D-05 明确只要 agent bundle；CLI 由 Phase 31 cloud-claude 通过 `go:embed` 分发）
**Risks：** 若 `tar -tzf` 发现 tarball 内部结构与假设不符（RESEARCH A2），executor 必须在失败时调整 `-C` 参数和解包成员名；见本 plan §Risks 2

### Task 2.3 — 写入 /etc/cloud-claude/*.version 版本元数据

**文件：** `deploy/docker/managed-user/Dockerfile`

**改动要点：**
- 插入位置：**紧随 Task 2.2 的 mutagen RUN 之后**，`RUN locale-gen`（66 行）之前
- 新增 RUN（单层，写 3 个小文件）：
  ```dockerfile
  # 版本元数据（C4 Mutagen 版本漂移防御 / Phase 30 Entry API 读取的旁路数据源）
  RUN mkdir -p /etc/cloud-claude \
      && echo "${MERGERFS_VERSION}" > /etc/cloud-claude/mergerfs.version \
      && echo "${MUTAGEN_VERSION}" > /etc/cloud-claude/mutagen.version \
      && echo "runtime-filled" > /etc/cloud-claude/tmux.version
  ```
- `tmux.version` 构建期占位 `runtime-filled`；运行时由 entrypoint `assert_tmux_version`（Plan 03）回填实际 `tmux -V` 输出
- **严禁**在这里写 `image.lock` 的镜像内副本（image.lock 是构建仓库内资产，不嵌入镜像；参考 D-27）

**对应：** D-07（版本元数据文件清单）
**PATTERNS：** 汇总行 B 列；D3（`locale-gen` + ENV 段的"构建期写配置"精神延续）

---

## Verification

### 静态断言

```bash
grep -F 'ARG MERGERFS_VERSION=2.41.1' deploy/docker/managed-user/Dockerfile
grep -F 'mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb' deploy/docker/managed-user/Dockerfile
grep -F 'sha256sum -c -' deploy/docker/managed-user/Dockerfile | wc -l | grep -Eq '^[[:space:]]*2$'  # mergerfs + mutagen 各一处
grep -F 'ARG MUTAGEN_VERSION=v0.18.1' deploy/docker/managed-user/Dockerfile
grep -F 'MUTAGEN_SHA256_AMD64=7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f' deploy/docker/managed-user/Dockerfile
grep -F 'MUTAGEN_SHA256_ARM64=bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a' deploy/docker/managed-user/Dockerfile
grep -F '/opt/mutagen-agents.tar.gz' deploy/docker/managed-user/Dockerfile
grep -F '/etc/cloud-claude/mergerfs.version' deploy/docker/managed-user/Dockerfile
grep -F '/etc/cloud-claude/mutagen.version' deploy/docker/managed-user/Dockerfile
grep -F '/etc/cloud-claude/tmux.version' deploy/docker/managed-user/Dockerfile
! grep -F 'apt-get install' deploy/docker/managed-user/Dockerfile | grep -F 'mergerfs'   # AP5 + M3 反向断言
```

### 运行时断言（需 Plan 01+02 构建的镜像）

```bash
DOCKER_BUILDKIT=1 docker build \
  --build-arg MERGERFS_SHA256_AMD64=<executor 填入实测值> \
  -f deploy/docker/managed-user/Dockerfile \
  -t local/managed-user:p02-test .

docker run -d --rm --name mu-p02 --cap-add SYS_ADMIN --device /dev/fuse local/managed-user:p02-test

# mergerfs 二进制可执行
docker exec mu-p02 mergerfs --version 2>&1 | grep -F '2.41.1'

# mutagen agent tarball 存在（尚未解压）
docker exec mu-p02 test -f /opt/mutagen-agents.tar.gz
docker exec mu-p02 file /opt/mutagen-agents.tar.gz | grep -Eq 'gzip compressed'

# 版本元数据文件内容
docker exec mu-p02 cat /etc/cloud-claude/mergerfs.version | grep -Fq '2.41.1'
docker exec mu-p02 cat /etc/cloud-claude/mutagen.version | grep -Fq 'v0.18.1'
docker exec mu-p02 cat /etc/cloud-claude/tmux.version | grep -Fq 'runtime-filled'
```

### Coverage contribution

> **Coverage contribution:** SC3（`/etc/cloud-claude/mutagen.version` + mutagen agent 预置） → 本 plan 负责镜像层的 mergerfs/mutagen 二进制 + 元数据落位；`mutagen-agent --version` 等于 `/etc/cloud-claude/mutagen.version` 的 runtime 断言由 Phase 31 消费 entrypoint 解压后完成。
>
> **Pitfall coverage:** M3（禁 apt mergerfs） / C4 前置（版本元数据写入）→ 本 plan 用 GitHub release `.deb` + `sha256sum -c -` + 版本文件三位一体防御。

---

## Atomic Commit Strategy

3 个原子 commit，每个对应一个 Task：

1. `feat(29-02): Dockerfile install mergerfs 2.41.1 from GitHub release deb`
2. `feat(29-02): Dockerfile stage mutagen-agents tarball for v0.18.1`
3. `feat(29-02): Dockerfile write /etc/cloud-claude version metadata files`

**不合并**：每条下载/校验都可能因为 sha256 不匹配而失败回滚；独立 commit 便于 `git bisect` 定位。

---

## Pitfalls 防御

| Pitfall | 防御手段 | 本 plan 对应任务 |
|---------|---------|-----------------|
| **M3** apt mergerfs 版本滞后 | 走 GitHub release `.deb` + `dpkg -i` | Task 2.1 |
| **C4** Mutagen 版本漂移（前置） | 写 `/etc/cloud-claude/mutagen.version` 供 Phase 31 握手比对 | Task 2.3 |
| **M18** 镜像体积（间接） | `rm /tmp/*.deb` / `rm /tmp/*.tar.gz` + 合并 RUN 避免多层 | Task 2.1 / 2.2 |

---

## Risks / Unknowns

1. **mergerfs `.deb` SHA256 未实测（RESEARCH Assumption A1）**
   - 当前 ARG `MERGERFS_SHA256_AMD64=""` / `MERGERFS_SHA256_ARM64=""` 空默认值会触发"SHA256 未设置"build fail
   - **Fallback 策略**：executor 首次构建时：
     ```bash
     # amd64
     curl -fsSL -o /tmp/x.deb https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_amd64.deb
     sha256sum /tmp/x.deb   # 记录输出
     # arm64（需 QEMU 或 arm64 机器）
     curl -fsSL -o /tmp/y.deb https://github.com/trapexit/mergerfs/releases/download/2.41.1/mergerfs_2.41.1.ubuntu-noble_arm64.deb
     sha256sum /tmp/y.deb
     ```
     把实测值通过 `--build-arg` 传入，或者一次性硬编码到 Dockerfile 默认 ARG（推荐，消除 CI 侧 `--build-arg` 维护）
   - 若 GitHub release 被 upstream 重新打包（校验值变化），`sha256sum -c -` 会立刻失败，CI 停在此 step；人工比对 upstream 新值后更新 ARG

2. **mutagen tarball 内部结构未实测（RESEARCH Assumption A2）**
   - 假设：顶层 `mutagen` + `mutagen-agents.tar.gz` 两个文件、无子目录
   - **Fallback**：executor 首次构建前 `curl ... && tar -tzf /tmp/mutagen.tar.gz` 打印清单；若实际有子目录（如 `mutagen-linux-amd64/mutagen-agents.tar.gz`），调整 Task 2.2 中 `tar -xzf ... -C /tmp` 的成员路径与 `mv` 源路径。
   - 最坏情况：完整 `tar -xzf /tmp/mutagen.tar.gz -C /tmp && find /tmp -name 'mutagen-agents.tar.gz' -exec mv {} /opt/ \;`

3. **Mutagen tarball 体积放大（~100MB 含多架构 agent）**
   - 若 Plan 06 的 CI gate 触发 BASE-04 失败，本 plan 可加一个裁剪步骤：在 mutagen RUN 内 `tar -xzf /opt/mutagen-agents.tar.gz -C /tmp/ma && find /tmp/ma -mindepth 1 -maxdepth 1 -type d ! -name 'linux_amd64' ! -name 'linux_arm64' -exec rm -rf {} + && tar -czf /opt/mutagen-agents.tar.gz -C /tmp/ma . && rm -rf /tmp/ma`（RESEARCH §镜像体积估算 裁剪候选）
   - 本 plan **默认不裁剪**；Plan 06 的 CI gate 实测结果决定是否回流

4. **构建 arm64 需 QEMU**
   - CI 已用 `docker buildx + setup-qemu-action`（见 `.github/workflows/build-images.yml`）；本地开发机用 amd64 单架构构建即可，arm64 交给 CI matrix

---

*End of Plan 02-binaries*
