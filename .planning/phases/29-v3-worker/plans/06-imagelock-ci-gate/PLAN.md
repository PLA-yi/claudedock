---
phase: 29-v3-worker
plan: 06-imagelock-ci-gate
sub_scope: F
type: execute
wave: 3
depends_on:
  - 01-image-base
  - 02-binaries
  - 03-entrypoint-config
files_modified:
  - deploy/docker/managed-user/image.lock
  - .github/workflows/build-images.yml
autonomous: true
requirements:
  - BASE-04
  - D-25
  - D-26
  - D-27
  - D-28
  - D-29
  - D-30
  - M18
must_haves:
  truths:
    - "image.lock 追加 6 字段：image_version=v3.0.0 / mergerfs_version / mutagen_agent_version / tmux_version_min / supports_mutagen=true / supports_mergerfs=true"
    - "image.lock 保持 YAML 扁平 key-value 结构；不改现有字段名/顺序"
    - "build-images.yml 新增 size gate step，判定 <= 700MB（≤ 734003200 bytes）否则 exit 1（BASE-04）"
    - "size gate 使用 docker buildx build 的 --load（linux/amd64 单架构）+ docker image inspect，不依赖 push=true（buildx push+load 互斥，D-29）"
    - "严禁修改 deploy/scripts/build-managed-image.sh（D-30）"
    - "严禁在 workflow 中 hard-fail 其他 step（size gate 独立 job step）"
  artifacts:
    - path: "deploy/docker/managed-user/image.lock"
      provides: "v3.0 镜像版本与能力清单"
      contains: "image_version: v3.0.0"
    - path: ".github/workflows/build-images.yml"
      provides: "docker image size <=700MB assertion step"
      contains: "docker image inspect"
  key_links:
    - from: "build + load 步骤"
      to: "size gate step"
      via: "docker image inspect --format={{.Size}}"
      pattern: "SIZE_BYTES -le 734003200"
---

## Goal

为 v3.0 受管镜像补上两道"落地即可验证"的护栏：(1) `image.lock` 追加 6 个能力/版本字段，作为 Phase 30 Entry API 与运维回溯的旁路数据源；(2) `.github/workflows/build-images.yml` 新增镜像体积 gate（≤ 700MB），把 BASE-04 从"口头承诺"落到 CI 硬约束。**严禁**改动 `deploy/scripts/build-managed-image.sh`（D-30）。

对应 Sub-scope：**F image.lock + CI 镜像体积 gate**（29-RESEARCH.md §Sub-scope 映射）。

---

## Scope

### In
1. `deploy/docker/managed-user/image.lock`：追加 6 字段（具体 Task 6.1）
2. `.github/workflows/build-images.yml`：在 managed-user 镜像构建 job 中，build step 之后追加一个新 step "Assert managed-user image size ≤ 700MB"（具体 Task 6.2）
3. 若现有 workflow 使用 `docker/build-push-action` with `push: true` 且没有并行 `--load` 机制（D-29 buildx push+load 互斥问题），**方案选型**（详见 Risks A/B）：
   - **方案 A（推荐）**：在现 build step 的 `platforms: linux/amd64` 且 `load: true` 的情况下直接 `docker image inspect`（适用于 PR 流水线，不 push）
   - **方案 B**：在 push step 之前插入一个额外的 `linux/amd64 + load: true` 的 build step（只为 inspect size，`cache-from` 复用），然后再跑原 push step
   - Executor 按现 workflow 实际情况选其一，选定后在 commit message 中说明

### Out
- `deploy/scripts/build-managed-image.sh` 任何改动（D-30 禁区）
- `image.lock` 字段的**消费者**（Phase 30 Entry API 通过 `docker exec cat /etc/cloud-claude/*.version` 读取运行时版本）
- 其他 image（scheduler-api / backend）的 size gate（只管 managed-user）
- Dockerfile/entrypoint 的改动（Plan 01-03 职责）

---

## Dependencies

- **01-image-base / 02-binaries / 03-entrypoint-config** — Wave 3
  - 原因：size gate step 必须在"Dockerfile 已完整"的前提下才有意义（若 Plan 02 未加 mergerfs / mutagen，镜像必然远小于 700MB，gate 假阳性绿灯）
  - `image.lock` 追加字段本身不依赖任何 plan，但为了 commit 粒度一致，一起放 Wave 3
- 可选分拆：executor 若想提前并行，可把 Task 6.1（image.lock）独立为 Wave 1；Task 6.2（CI gate）保留 Wave 3。该拆分不影响 commit 边界

---

## Tasks

### Task 6.1 — image.lock 追加 6 字段

**文件：** `deploy/docker/managed-user/image.lock`

**改动要点：**
- 先读现文件确认顶层 YAML 结构（当前为扁平 `key: value` 若干行，如 `image: managed-user` / `user: workspace` / `home: /workspace` 等）
- 在文件**末尾**追加（**不**改动已有字段顺序 / 值）：
  ```yaml
  # v3.0 baseline — Phase 29 追加
  image_version: v3.0.0
  mergerfs_version: 2.41.1
  mutagen_agent_version: v0.18.1
  tmux_version_min: "3.4"
  supports_mutagen: true
  supports_mergerfs: true
  ```
- **严禁**：
  - 改动现存任何字段名或值（向后兼容；build-managed-image.sh 可能解析现字段）
  - 引入嵌套结构（`capabilities: {...}`）；D-25 明确要求扁平 key-value
  - 写入真实 SHA256 / token（CLAUDE.md 隐私）
- **版本号需与 Plan 02 Dockerfile `ARG MERGERFS_VERSION / MUTAGEN_VERSION` 保持一致**（否则 Phase 30 回溯数据不可信）

**对应：** D-25（image.lock 字段清单）/ D-26（扁平结构）
**PATTERNS：** `29-PATTERNS.md §汇总表 Sub-scope F` 首行（image.lock 现 9 行 YAML 扁平 key-value analog；`tmux_version_min: "3.4"` 引号必须保留 —— 避免 YAML 解析成 float）。注：`29-PATTERNS.md §Reusable Assets §CI` 的 C1 / C2 是 workflow 资产（见 Task 6.2），不适用于 image.lock；本 task 引用的是汇总表 Sub-scope F 行而非 Reusable Assets 编号。
**参考：** RESEARCH §Code Examples §image.lock

### Task 6.2 — build-images.yml 新增 size gate step

**文件：** `.github/workflows/build-images.yml`

**改动要点：**

**步骤 A — 定位当前 managed-user 构建 job**
```bash
grep -n 'managed-user' .github/workflows/build-images.yml
grep -n 'docker/build-push-action' .github/workflows/build-images.yml
```
识别 managed-user 对应的 job（如 `build-managed-user`）和 build step 的 `uses: docker/build-push-action@vX` 行号。

**步骤 B — 判断现 workflow 模式（决定用方案 A 还是 B）**
- 若现 build step `load: true`（或 `platforms: linux/amd64` 且无 `push: true`）→ **方案 A**
- 若现 build step `push: true`（且 multi-platform `linux/amd64,linux/arm64`）→ **方案 B**（buildx push 模式不支持同时 `--load`）

**步骤 C — 方案 A：直接追加 inspect step**

在 managed-user build step **之后**插入：

```yaml
      - name: Assert managed-user image size <= 700MB
        shell: bash
        run: |
          set -euo pipefail
          # D-27 / BASE-04：已构建 image 必须 ≤ 700MB（734003200 bytes）
          IMG="${MANAGED_USER_IMAGE_TAG:-ghcr.io/${{ github.repository_owner }}/cloud-cli-proxy-managed-user:latest}"
          SIZE_BYTES=$(docker image inspect --format='{{.Size}}' "${IMG}")
          SIZE_MB=$(( SIZE_BYTES / 1048576 ))
          echo "managed-user image size: ${SIZE_BYTES} bytes (${SIZE_MB} MiB)"
          if [ "${SIZE_BYTES}" -gt 734003200 ]; then
            echo "::error::managed-user image ${SIZE_MB} MiB exceeds 700 MiB hard cap (BASE-04)"
            exit 1
          fi
```

其中 `MANAGED_USER_IMAGE_TAG` 应复用现 workflow 已定义的 env / output（如现 step 用了 `tags:` 多行，取第一个 tag）。具体 tag 表达式由 executor 在识别 workflow 现状后填入。

**步骤 D — 方案 B：插入独立 load step 后再 inspect**

若现 build step 是 multi-arch push：

```yaml
      - name: Build managed-user (amd64, load-only for size gate)
        uses: docker/build-push-action@v5
        with:
          context: .
          file: deploy/docker/managed-user/Dockerfile
          platforms: linux/amd64
          load: true
          push: false
          cache-from: type=gha,scope=managed-user
          cache-to: type=gha,scope=managed-user,mode=max
          tags: managed-user:size-gate
          # 注：本 step 与下方 push step 共享 gha 缓存；实际构建只跑一次

      - name: Assert managed-user image size <= 700MB
        shell: bash
        run: |
          set -euo pipefail
          SIZE_BYTES=$(docker image inspect --format='{{.Size}}' managed-user:size-gate)
          SIZE_MB=$(( SIZE_BYTES / 1048576 ))
          echo "managed-user image size: ${SIZE_BYTES} bytes (${SIZE_MB} MiB)"
          if [ "${SIZE_BYTES}" -gt 734003200 ]; then
            echo "::error::managed-user image ${SIZE_MB} MiB exceeds 700 MiB hard cap (BASE-04)"
            exit 1
          fi
```

然后保留现有的 multi-arch push step（无需改动）。

**注意事项：**
- **严禁**改动 `deploy/scripts/build-managed-image.sh`（D-30 绝对禁区）
- **严禁**修改现 build step 的 `platforms: linux/amd64,linux/arm64` → `linux/amd64` 以便 load（会影响生产镜像）；必须通过方案 B 独立 step 隔离
- gate 只作用于 managed-user 镜像；scheduler-api / backend 等其他镜像不加
- `734003200` = `700 * 1024 * 1024` bytes（严格 700 MiB；MB vs MiB 取标准二进制）；D-27 文本 "700MB" 统一解释为 MiB，与 RESEARCH 一致

**对应：** BASE-04 / M18 / D-27（700MB 上限）/ D-28（`docker image inspect --format`）/ D-29（push vs load 互斥）/ D-30（build-managed-image.sh 不改）
**PATTERNS（R6 对齐 `29-PATTERNS.md` 实际编号）：**
- `29-PATTERNS.md §Reusable Assets §CI` **C1**（multi-arch buildx + GHCR push 的 matrix 结构，全文基线）—— 本 task 在 managed-user 矩阵行的 build step 之后**插入新 post-build step**，不重构现 matrix
- `29-PATTERNS.md §Reusable Assets §CI` **C2**（`echo "X=Y" >> $GITHUB_ENV` 动态 env 传递，现 `.github/workflows/build-images.yml:56-57`）—— 若方案 A 需要把 image tag 从 meta step 传到 gate step 时复用该模式（`steps.meta.outputs.tags` → env）
- `29-PATTERNS.md §Anti-patterns` **AP3**（不把 size gate bash 塞进 `build-managed-image.sh`，D-30 禁区）—— 本 task 只改 workflow YAML，坚决不引入 `build-managed-image.sh --check-size` 之类参数
- **新增模式（无 analog）**：`docker image inspect --format='{{.Size}}'` + GitHub Actions `::error::` 注解属本 phase 首次引入，无 Reusable Assets 条目；按 RESEARCH §Code Examples §CI gate 骨架实现
**参考：** RESEARCH §Code Examples §CI gate 草稿 + §Open Questions §buildx push+load 方案选型

---

## Verification

### 静态断言

```bash
# Task 6.1 — image.lock 字段齐全
grep -F 'image_version: v3.0.0'                deploy/docker/managed-user/image.lock
grep -F 'mergerfs_version: 2.41.1'             deploy/docker/managed-user/image.lock
grep -F 'mutagen_agent_version: v0.18.1'       deploy/docker/managed-user/image.lock
grep -E 'tmux_version_min:\s*"?3\.4"?'         deploy/docker/managed-user/image.lock
grep -F 'supports_mutagen: true'               deploy/docker/managed-user/image.lock
grep -F 'supports_mergerfs: true'              deploy/docker/managed-user/image.lock
# YAML 语法（若 executor 环境有 yq）
command -v yq >/dev/null && yq eval '.' deploy/docker/managed-user/image.lock >/dev/null
# 反向断言：无嵌套结构（D-26 扁平）
! grep -E '^\s{2,}(image_version|mergerfs_version|mutagen_agent_version)' deploy/docker/managed-user/image.lock

# Task 6.2 — workflow size gate 存在
grep -F 'Assert managed-user image size' .github/workflows/build-images.yml
grep -F 'docker image inspect --format'  .github/workflows/build-images.yml
grep -F '734003200'                      .github/workflows/build-images.yml
grep -F '::error::'                      .github/workflows/build-images.yml
# D-30 反向断言：build-managed-image.sh 未被修改（对比 HEAD）
git diff --name-only HEAD -- deploy/scripts/build-managed-image.sh | grep -q . && \
  { echo "FAIL: deploy/scripts/build-managed-image.sh modified (D-30 violation)"; exit 1; } || true

# workflow YAML 语法
command -v yq >/dev/null && yq eval '.' .github/workflows/build-images.yml >/dev/null

# actionlint（若可用）
command -v actionlint >/dev/null && actionlint .github/workflows/build-images.yml
```

### 动态断言（本地 smoke）

```bash
# 在本机执行 managed-user 构建 + inspect（如 Plan 01-03 已完成）
# --- Linux/amd64 宿主 ---
DOCKER_BUILDKIT=1 docker build \
  -t managed-user:size-gate \
  -f deploy/docker/managed-user/Dockerfile \
  --build-arg MERGERFS_SHA256_AMD64="${MERGERFS_SHA256_AMD64}" \
  --build-arg MUTAGEN_SHA256_AMD64=7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f \
  .

SIZE_BYTES=$(docker image inspect --format='{{.Size}}' managed-user:size-gate)
echo "size=${SIZE_BYTES} bytes ($(( SIZE_BYTES / 1048576 )) MiB)"
test "${SIZE_BYTES}" -le 734003200

# image.lock 字段运行时可读（Phase 30 Entry API 不依赖此通道，但本地验证方便）
yq eval '.image_version' deploy/docker/managed-user/image.lock   # → v3.0.0
yq eval '.supports_mergerfs' deploy/docker/managed-user/image.lock  # → true
```

### Coverage contribution

> **Coverage contribution:** V-08（image.lock 6 字段写入） / V-09（CI size gate 对超 700MB 镜像 exit 1）→ 本 plan 完整承担 SC7（BASE-04 ≤ 700MB）的落地验证；Phase 30 Entry API 能力协商的旁路数据源（image.lock）由本 plan 埋点。
>
> **Pitfall coverage:** M18（镜像膨胀到 >1GB）→ 本 plan 通过 CI hard gate 兜底（Plan 01 BuildKit cache 是前置优化，本 plan 是后置 assertion）。

---

## Atomic Commit Strategy

2 个原子 commit：

1. `feat(29-06): managed-user image.lock add v3.0 baseline fields`
   - Task 6.1（image.lock 追加 6 字段）
2. `ci(29-06): build-images workflow assert managed-user size <= 700MB`
   - Task 6.2（workflow size gate step；commit message 中注明选用方案 A 还是 B）

---

## Pitfalls 防御

| Pitfall | 防御手段 | 本 plan 对应任务 |
|---------|---------|-----------------|
| **M18 / BASE-04** 镜像膨胀 | CI `docker image inspect`/734003200 hard gate | Task 6.2 |
| **D-30 禁区** 误改 build-managed-image.sh | 静态断言 `git diff HEAD -- deploy/scripts/build-managed-image.sh` 为空 | Verification 反向断言 |
| **D-29** buildx push+load 互斥 | 方案 B 独立 `load: true` amd64 step + 原 push step 共存；通过 gha 缓存避免双倍构建 | Task 6.2 步骤 D |
| **AP3**（`29-PATTERNS.md §Anti-patterns`）改 shell 脚本绕过 workflow | 只改 YAML，不引入 `build-managed-image.sh --check-size` 等新参数；D-30 禁区 | Task 6.2 约束 |
| **CLAUDE.md 隐私** image.lock 内硬编码 SHA | image.lock 只写 version 不写 SHA（SHA 通过 Dockerfile ARG 注入） | Task 6.1 约束 |

---

## Risks / Unknowns

1. **现 `build-images.yml` 到底是 push 模式还是 load 模式？**
   - Executor 必须先 `grep -A10 'docker/build-push-action' .github/workflows/build-images.yml` 看清现状再选方案
   - 若是 push-only multi-arch，选方案 B（双 step + gha 缓存）；若 PR 构建已 load，选方案 A（单 step + inspect）
   - **Fallback**：若 executor 发现 workflow 结构完全不符合预期（如 managed-user 不在此文件而在别的 workflow），应 STOP 并在 commit message 中 BLOCKED，让 orchestrator 重新定义 Plan 06

2. **`MANAGED_USER_IMAGE_TAG` 表达式**
   - 现 workflow 可能用 `${{ steps.meta.outputs.tags }}` 多行 tag、`env.IMAGE_TAG` 或直接 hardcode
   - Task 6.2 示例用 `${{ github.repository_owner }}` 占位，executor 需以现 workflow 实际 tag 表达式替换
   - **Fallback**：若 tag 复杂（多行），改用 `docker images --format '{{.Repository}}:{{.Tag}} {{.ID}}' | head -n1` 拿到最近构建的 image

3. **700 MB vs 700 MiB 口径**
   - D-27 / ROADMAP 写 "700MB"；本 plan 统一按 **700 MiB = 734003200 bytes**（与 docker image inspect 的 Size 字段单位一致，Size 是 bytes）
   - 若未来审计要求严格十进制 MB（734000000），改 gate 常量即可，影响很小
   - **Fallback**：无需处理；本 plan 注释已说明口径

4. **`cache-from/cache-to` 冲突**
   - 方案 B 双 step 同 scope `managed-user` 会共享缓存，理论上第二次 build 只是 load 导出，极快；若 GitHub Actions 的 gha cache 并发写有冲突，可 scope 加后缀（`scope=managed-user-gate`）
   - **Fallback**：若 CI 出现 "context already being written" 类错误，方案 B 的 size-gate step 改用 `cache-to: type=inline`（不持久化）

5. **ARM64 平台的 size gate**
   - 本 plan 只对 amd64 inspect；arm64 镜像通常比 amd64 略大或略小（差 < 5%），仍在 700MB 阈值内
   - 若 arm64 超标而 amd64 未超标，会假阴性放行
   - **Fallback**：v1 只守 amd64 基线；arm64 gate 延后到 Phase 33 或专用 CI 补丁

6. **image.lock 现有字段不明**
   - Executor 先 `cat deploy/docker/managed-user/image.lock` 看现状；若有重名字段（极不可能，但历史遗留难说）要先 rename 旧字段并提交一个独立的 refactor commit（不在本 plan 范围，应 BLOCKED）

---

*End of Plan 06-imagelock-ci-gate*
