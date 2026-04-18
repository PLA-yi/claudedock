---
phase: 29-v3-worker
artifact: PLAN-CHECK
verdict: APPROVED (Round 2)
verifier: gsd-plan-checker (Claude)
date: 2026-04-18
rounds:
  - round: 1
    date: 2026-04-18
    verdict: NEEDS REVISION
    blockers: 6
  - round: 2
    date: 2026-04-18
    verdict: APPROVED
    blockers: 0
plans_checked:
  - 01-image-base/PLAN.md
  - 02-binaries/PLAN.md
  - 03-entrypoint-config/PLAN.md
  - 04-worker-contract/PLAN.md
  - 05-host-preflight-docs/PLAN.md
  - 06-imagelock-ci-gate/PLAN.md
sources_of_truth:
  - .planning/phases/29-v3-worker/29-CONTEXT.md
  - .planning/phases/29-v3-worker/29-RESEARCH.md
  - .planning/phases/29-v3-worker/29-PATTERNS.md
  - .planning/phases/29-v3-worker/29-DISCUSSION-LOG.md
  - .planning/ROADMAP.md §Phase 29
  - .planning/REQUIREMENTS.md
  - 现网代码：deploy/scripts/host-preflight.sh / deploy/docker/managed-user/{Dockerfile,entrypoint.sh,sshd_config,image.lock} / internal/agentapi/contracts.go / internal/runtime/tasks/worker.go / .github/workflows/build-images.yml
---

# Phase 29 Plan Check Report

## Final Verdict

**APPROVED（Round 2 — 2026-04-18）** —— Round 1 列出的 6 项修订（R1 关键 + R2..R6 偏小）全部按要求落地，未发现回归；可进入 `/gsd-execute-phase 29-v3-worker`。Round 1 历史报告完整保留在下文，便于追溯。

---

## 最终判定（Final Verdict — Round 1 历史）

**NEEDS REVISION** —— 共 6 项需要 planner 修订（其中 1 项较关键，5 项偏小）。整体架构 / 边界 / Wave 依赖 / 原子提交策略均合格，主要问题集中在：(a) Plan 05 引用了 `host-preflight.sh` 中**实际并不存在**的 helper 与 main 结构，会让 executor 无所适从；(b) 部分 PATTERN 编号 / 风险条目与用户 checklist 期望不一致，影响 traceability。所有问题均**可在 ≤1 小时内增量修订**完成，无需重新规划，不建议 BLOCKED。

---

## A. 目标覆盖（Goal coverage）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **A1** | 7 个 Success Criteria 至少各被一个 plan 的 Verification 覆盖 | **PARTIAL** | SC3 / SC4 / SC5 / SC6 / SC7 全部有 Phase 29 内的 runtime/static 断言（Plan 01 §Verification、Plan 03 §Verification、Plan 05 §动态断言、Plan 06 §动态断言）。**SC1（mergerfs mount 参数 `func.readdir=cor:4` 等）/ SC2（getfattr branches）的真正运行时断言被显式 defer 到 Phase 31**（Plan 03 §Coverage：「SC1 / SC2 的真正 mount 断言由 Phase 31 消费」）。这是 RESEARCH §Validation Architecture 的设计，但严格按 ROADMAP §Phase 29 文字面，Phase 29 内确实没有 SC1/SC2 的运行时断言——只有"mergerfs --version 2.41.1 可执行"+「参数文档化在注释」两个间接证据。建议 planner 在 Plan 03 Coverage 段或 Phase 29 STATE 中明确"SC1/SC2 由 Phase 31 完成端到端断言"以闭合追溯链。|
| **A2** | BASE-04（≤700MB）有 `docker image inspect` 断言，写在 Plan 06 | **PASS** | Plan 06 Task 6.2（`06-imagelock-ci-gate/PLAN.md:131-144` + `:166-176`）`SIZE_BYTES=$(docker image inspect --format='{{.Size}}' …); … -gt 734003200 ; exit 1`，与 D-28 一致。|
| **A3** | Q10（mergerfs 2 路 + env 扩展点）在 Plan 03 有文档/代码任务 | **PASS** | Plan 03 Task 3.3 prepare_mergerfs_check 注释段（`03-entrypoint-config/PLAN.md:193-194`）：`# Q10：2 路 branch 锁定，3 路扩展通过 CLOUD_CLAUDE_MERGERFS_BRANCHES env 预留`，以及对应 D-12 标注。|

---

## B. 坑位覆盖（Pitfall coverage）

| ID | Pitfall → Plan | 结论 | 证据 |
|----|---------------|------|------|
| **B1** | C1 mergerfs serial readdir → Plan 03 `func.readdir=cor:4` 文档/参数 | **PARTIAL** | Plan 03 §Pitfalls 表格行 C1 明确「参数字符串 `func.readdir=cor:4` 在镜像 README / `prepare_mergerfs_check` 注释中文档化」（`03-entrypoint-config/PLAN.md:375`），**但** Task 3.3 给出的 entrypoint 代码片段（`:185-195`）注释只写 Q10 + 「mount deferred to cloud-claude (Phase 31)」，**未直接出现 `func.readdir=cor:4` 字符串**。Verification 也未 grep 该字符串。建议 planner 在 Task 3.3 代码注释中显式包含完整参数串以便 grep 自检。|
| **B2** | C2 pfrd 随机新文件 → Plan 03 `category.create=ff` | **PARTIAL** | 同 B1：Pitfalls 表（`:376`）声明文档化但 Task 3.3 代码片段未出现 `category.create=ff` 字符串。同 B1 修订建议。|
| **B3** | C3 sshfs 抖动 → Plan 03（仅文档；Phase 31 消费） | **PASS** | Plan 03 §Pitfalls 表行 C3（`:377`）：「entrypoint 不改 sshfs 参数；镜像层只保证 sshfs --version 可用；Phase 31 传入稳定参数」。|
| **B4** | C5 非 root + Mutagen 反清空 → Plan 01 预建 + Plan 03 entrypoint 二次 chown | **PASS** | Plan 01 Task 1.3（`:148-175`）预建 `/home/claude` 家族 + chown 1000:1000；Plan 03 Task 3.3 `prepare_v3_dirs`（`:161-168`）二次 chown。双层防御完整。|
| **B5** | C6 Ubuntu 25.04 AppArmor `fusermount3`（**修正路径**）→ Plan 05 | **PASS** | Plan 05 Task 5.1（`05-host-preflight-docs/PLAN.md:126-128`）使用 `/etc/apparmor.d/local/fusermount3`（非 docker-default），并在 Task 5.2（`:193`）的 README 修复指引中也使用 fusermount3 路径。完全采用 D-23 修正版。|
| **B6** | C7 systemd-logind 杀 tmux → Plan 01（PID 1 = tini，不装 systemd） | **PASS** | Plan 01 Task 1.2（`:138`）apt 装 `tini`、Task 1.4（`:182-184`）`ENTRYPOINT ["/usr/bin/tini","--",…]`；Plan 01 §Verification（`:230-231`）`! pgrep -x systemd-logind` 反向断言。|
| **B7** | M3 禁 apt mergerfs → Plan 02 静态 `.deb` | **PASS** | Plan 02 Task 2.1（`02-binaries/PLAN.md:83-103`）`curl + sha256sum -c - + dpkg -i`；§Verification（`:182`）`! grep apt-get install mergerfs` 反向断言。|
| **B8** | M4 entrypoint 串行 + fail-fast → Plan 03 `set -euo pipefail` | **PASS** | Plan 03 Task 3.3 函数体内统一 `[[ ! -f ]] … exit 1`、`exit 1` on tmux < 3.4；entrypoint 现 1-2 行已是 `set -euo pipefail`（继承未改）；§Verification awk（`:285`）按顺序断言 prepare_v3_dirs → … → exec sshd。|
| **B9** | M7/M8 tmux truecolor + window-size → Plan 03 tmux.conf | **PASS** | Plan 03 Task 3.1（`:111-117`）`set -ga terminal-overrides ",*:RGB"` + `window-size latest` + `aggressive-resize on`；profile.d 导出 `CLAUDE_CODE_TMUX_TRUECOLOR=1`。|
| **B10** | M12 sshd KeepAlive 基线 → Plan 03 sshd_config | **PASS** | Plan 03 Task 3.2（`:135-141`）追加 `ClientAliveInterval 15` / `ClientAliveCountMax 8` / `MaxSessions 30` / `MaxStartups 60:30:120`。|
| **B11** | M17 预建目录权限 → Plan 01 + Plan 03 双 chown | **PASS** | Plan 01 Task 1.3 镜像层一次 chown；Plan 03 Task 3.3 entrypoint runtime 二次 chown。Pitfalls 表均显式标注 M17。|
| **B12** | M18 BuildKit cache mount → Plan 01 | **PASS** | Plan 01 Task 1.1（`:88-91`）`rm -f /etc/apt/apt.conf.d/docker-clean` + `keep-cache`；Task 1.2（`:104-105`）`--mount=type=cache,target=/var/cache/apt,sharing=locked` + `/var/lib/apt/lists,sharing=locked`。|

---

## C. 跨 plan 边界完整性（Cross-plan boundary integrity）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **C1** | 各 plan §Out 显式列出其它 5 个 plan 的归属 | **PARTIAL** | Plan 01 / 03 / 04 / 06 在 §Out 区块**逐项列出 Plan 02..06 的归属**，无孤儿文件。Plan 02 §Out（`02-binaries/PLAN.md:58-62`）只列了 Plan 01/03/06，未提 Plan 04/05（合理：Plan 04/05 与 Dockerfile 无交集）。Plan 05 §Out（`05-host-preflight-docs/PLAN.md:64-68`）较松散，没有逐 plan 列出，只标"另由 Plan 03 entrypoint 或后续 phase 承担"——可读性弱。建议 planner 在 Plan 02 / 05 的 §Out 末尾追加一行"其它 plan（Plan 04 / 05）与本 plan 文件无交集，无需互斥"的显式声明，闭合追溯。|
| **C2** | 多 plan 共享 Dockerfile 时，按 section/RUN 块分区，无重复 ownership | **PASS** | Dockerfile 由 Plan 01 / 02 / 03 共享：Plan 01 改顶部（syntax + ENV 之后 cache RUN）+ apt 清单 RUN（9-41 行）+ 预建目录 RUN（85-87 后）+ ENTRYPOINT（102 行），Plan 02 在 Chromium RUN 之后 / locale-gen 之前**追加** 3 条新 RUN（不修改 Plan 01 现有 RUN），Plan 03 在 sshd_config COPY（89 行）之后 / entrypoint COPY（90 行）之前**追加** 2 条 COPY + 1 条 chmod RUN。三方插入点不重叠。注意：Plan 01 Task 1.3 与 Plan 03 Task 3.4 都在「89-90 行」附近插入，但前者插入 RUN（before line 89），后者插入 COPY（after line 89），物理位置相邻但语义独立。建议 commit 顺序严格 Plan 01 → Plan 02 → Plan 03（已由 Wave 强制）。|
| **C3** | Wave 一致 | **PASS** | 6 份 plan front-matter wave 字段：01=1 / 02=2（depends_on:01）/ 03=3（depends_on:01,02）/ 04=1 / 05=1 / 06=3（depends_on:01,02,03）。完全符合 user checklist 期望「Plan 02 dep 01；Plan 03 dep 01+02；Plan 06 CI gate dep 01-03」。Plan 04 / 05 与 Dockerfile 无交集，独立 Wave 1 可并行。|
| **C4** | Plan 04 显式排除 ClaudeAccountID（D-21） + docker volume create（Phase 33） | **PASS** | Plan 04 §Out（`04-worker-contract/PLAN.md:69-73`）「`docker volume create` 幂等化 → Phase 33；`ClaudeAccountID` 字段 → Phase 30」；§Verification（`:299-300`）`! grep ClaudeAccountID contracts.go` + `! grep 'docker volume create' worker.go` 反向断言。|
| **C5** | Plan 06 显式排除 build-managed-image.sh 修改（D-30） | **PASS** | Plan 06 §Out（`06-imagelock-ci-gate/PLAN.md:65`）「`deploy/scripts/build-managed-image.sh` 任何改动（D-30 禁区）」；§Verification（`:215-217`）`git diff --name-only HEAD -- deploy/scripts/build-managed-image.sh` 为空的反向断言；§Pitfalls 表 D-30 行（`:271`）。|

---

## D. 决策保真度（Decision fidelity）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **D1** | Plan 05 D-23 使用修正路径（`deploy/scripts/host-preflight.sh` + `/etc/apparmor.d/local/fusermount3`） | **PASS** | Plan 05 整篇文件多处出现 `deploy/scripts/host-preflight.sh`（front-matter `files_modified`、Task 5.1、§Verification grep），**未出现** `docker-default` 字样；AppArmor 路径全部用 `/etc/apparmor.d/local/fusermount3`（Task 5.1 `:127`、Task 5.2 `:193`、§Verification `:226 / :230 / :247`）。完全采用 2026-04-18 修正版。|
| **D2** | Plan 03 D-12：2 路 branch + `CLOUD_CLAUDE_MERGERFS_BRANCHES` env 扩展点 | **PASS** | Plan 03 Task 3.3 注释（`03-entrypoint-config/PLAN.md:193-194`）：「Q10：2 路 branch 锁定，3 路扩展通过 CLOUD_CLAUDE_MERGERFS_BRANCHES env 预留（读取位置在 Phase 31 cloud-claude，本阶段仅登记 env 名称，不读取）」。与 CONTEXT D-12（`29-CONTEXT.md:52`）一致。|
| **D3** | Plan 01 D-10：tini ENTRYPOINT exec form `["/usr/bin/tini","--","/usr/local/bin/entrypoint.sh"]` | **PASS** | Plan 01 Task 1.4（`01-image-base/PLAN.md:184`）字面引用 `ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`；§Verification（`:213`）`grep -F` 完全匹配字符串。与 CONTEXT D-10（`29-CONTEXT.md:50`）一致。|
| **D4** | Plan 04 D-19：`--mount type=volume` 语法 + `,readonly`（不是 `,ro`） | **PASS** | Plan 04 Task 4.2（`04-worker-contract/PLAN.md:124-127`）`opts := fmt.Sprintf("type=volume,src=%s,dst=%s", …); if vm.ReadOnly { opts += ",readonly" }`；显式注释「`,readonly` 无值标志（不是 `,ro`；RESEARCH §Code Examples 明确）」（`:131`）；§Verification（`:305`）grep `'",readonly"'`。与 CONTEXT D-19（`29-CONTEXT.md:96`）一致。|
| **D5** | Plan 04 D-22：Volumes slice 用 `omitempty` | **PASS** | Plan 04 Task 4.1（`:103-104`）`Volumes []VolumeMount \`json:"volumes,omitempty"\``；§Verification（`:295-296`）grep；Task 4.4 §TestHostActionRequest_VolumesOmitempty（`:171-181`）测试空 Volumes 不出现在 JSON。与 CONTEXT D-22（`29-CONTEXT.md:100`）一致。|

---

## E. 模式保真度（Pattern fidelity）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **E1** | Plan 01 引用 Dockerfile apt-merge / KasmVNC `.deb` / ENTRYPOINT exec form / Reusable Assets D1-D5 | **PARTIAL** | Plan 01 引用 D2 (apt 合并 RUN)、D5 (ENTRYPOINT exec form)、A1/A3/A5 与 Code Examples §BuildKit apt cache mount。**未引用 D1 (KasmVNC `.deb` 模板)**——但 Plan 01 本身不下载 `.deb`（mergerfs/mutagen 在 Plan 02），引用 D1 不必要。Reusable Assets D1-D5 中 D1/D3/D4 与 Plan 01 任务无直接对应，引用清单与 scope 匹配合理。建议 planner 在 Plan 01 §Tasks 顶部加一句「PATTERNS D1（KasmVNC `.deb`）由 Plan 02 复用，本 plan 不涉及」以闭合 traceability，但当前不算 FAIL。|
| **E2** | Plan 02 引用 KasmVNC `.deb` 模板（D1）+ sha256sum 增量 | **PASS** | Plan 02 Task 2.1（`02-binaries/PLAN.md:108`）：「PATTERNS：汇总行 B 列 + D1（`ARG <COMPONENT>_VERSION=X.Y.Z` + `curl -fsSL -o /tmp/...` KasmVNC `.deb` 模板）；AP5（不走 apt）」；并显式说明 sha256sum 校验为相对 D1 analog 的增量（D1 模板未含校验）。|
| **E3** | Plan 03 引用 Sub-scope C 串行 init 模式 + AP2 | **PASS** | Plan 03 Task 3.3（`03-entrypoint-config/PLAN.md:232`）：「PATTERNS：汇总行 C 列 + S3（串行编排） + S4（轮询超时…） + AP2 / AP4 / AP10」。AP2（不挤进 KasmVNC 函数）+ AP4（不在 entrypoint mount mergerfs）+ AP10（不重复 chown /workspace）三反模式齐备。|
| **E4** | Plan 04 引用 G1-G2（struct/JSON tag + args append）+ G3-G5（test infra） | **PASS** | Plan 04 Task 4.1（`04-worker-contract/PLAN.md:110`）：「PATTERNS：G1（`struct + JSON tag + omitempty` slice 字段；`SSHKeys` 是直接 analog）」；Task 4.2（`:136`）：「PATTERNS：G2（docker create args 数组 + append 遍历模式）」；Task 4.4（`:279`）：「PATTERNS：G3 / G5（`t.Run("case_name", ...)` 风格）」。G4 未显式提，但 G3+G5 已覆盖测试基础设施，符合用户期望。|
| **E5** | Plan 05 引用 S2 (`require_cmd` helper) + AP9 (修正脚本路径) | **FAIL** | **关键问题：** Plan 05 Task 5.1（`05-host-preflight-docs/PLAN.md:87 / :160`）声称「保持 2-space 缩进 + `log_info/log_warn/log_fail/log_skip` 与现脚本一致」并引用「**S2（`log_skip/log_warn/log_fail` 语义）**」——但实际现网 `deploy/scripts/host-preflight.sh`（实际只有 44 行，已通读）**完全没有 `log_*` helper、没有 `main()`、没有任何 `check_*` 函数**，唯一存在的 helper 是 `require_cmd`（PATTERNS S2 真正指向的应是 `require_cmd`，与 user checklist 一致）。脚本目前是扁平 top-level 命令（`require_cmd docker / ip / systemctl`、`modprobe fuse`、`mkdir -p /var/lib/cloud-cli-proxy`），失败一律 `exit 1`，没有 `PREFLIGHT_HAD_WARN` 这种累计变量。Plan 05 Task 5.1 的代码骨架（`:90-148`）大量调用 `log_skip / log_warn / log_fail / log_ok`，executor 会**直接遇到「未定义命令」运行错误**。Plan 05 也未引用 AP9（路径修正反模式），只引用 AP3。**修订必须项**（见 §修订清单 R1）。|
| **E6** | Plan 06 引用 C1 (build-images.yml 结构) + AP3 (不内联 build script) | **PARTIAL** | Plan 06 Task 6.1（`06-imagelock-ci-gate/PLAN.md:106`）：「PATTERNS：C1（YAML 扁平 key-value；image.lock 现有风格 analog）」——这里 C1 指 image.lock 的 YAML 模式，**不是** build-images.yml 结构。Task 6.2（`:188`）：「PATTERNS：C2（`docker image inspect --format` 标准模式）/ C3（GitHub Actions `::error::` 注解）/ AP5（不改 build shell 脚本；只改 workflow YAML）」——AP5 实际承担了 user checklist 期望的 AP3 语义（不在 build script 内联 gate）。可能是 PATTERNS 编号差异（C / AP 行号在 PATTERNS.md 中可能不同），实际语义已覆盖。建议 planner 校对 PATTERNS.md 行号并更新引用编号一致（见 §修订清单 R6）。|

---

## F. 原子提交与验证可行性（Atomic commit & verification feasibility）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **F1** | 「1 commit per task」或聚合琐碎任务，无 megacommit | **PASS** | Plan 01 §Atomic Commit Strategy（`01-image-base/PLAN.md:244-257`）4 commit；Plan 02 3 commit；Plan 03 4 commit；Plan 04 3 commit；Plan 05 2 commit；Plan 06 2 commit。每个 commit message 都遵循 `feat(29-XX): ...` / `test(29-XX): ...` / `ci(29-XX): ...` / `docs(29-XX): ...` 规范。Plan 01 §AtomicCommit 末段允许 1.1+1.2 合并、Plan 04 Task 4.3 可选——均为可执行的合并策略，无 megacommit 倾向。|
| **F2** | Verification 命令具体（明确 `docker exec` / `grep` / `go test name`） | **PASS** | 全部 6 plan 的 §Verification 都给出**字面 grep 字符串**、**字面 docker exec 命令**、**字面 go test 名**。例：Plan 01 §Verification（`:204-215`）8 条 grep；Plan 03 §Verification（`:259-298`）20+ 条 grep + awk 顺序断言；Plan 04 §动态断言（`:317-332`）`go vet` + `go test -run 'TestHostActionRequest_VolumesOmitempty\|TestHostActionRequest_V2Compat\|TestBuildCreateArgs_VolumesMount\|TestBuildCreateArgs_EmptyVolumes_NoExtraArgs'` 字面测试名；Plan 06 §动态断言（`:228-244`）`docker image inspect --format='{{.Size}}' managed-user:size-gate; test "${SIZE_BYTES}" -le 734003200`。无 vague「verify it works」表述。|
| **F3** | 每 plan 末尾包含 `Coverage contribution: SC-x, SC-y` + `Pitfall coverage: C-n, M-n` | **PASS** | Plan 01 §Coverage（`:236-240`）；Plan 02 §Coverage（`:208-212`）；Plan 03 §Coverage（`:348-352`）；Plan 04 §Coverage（`:334-338`）；Plan 05 §Coverage（`:274-278`）；Plan 06 §Coverage（`:247-251`）——6 plan 均有两行结构化 Coverage / Pitfall 声明，可机器解析。|

---

## G. 风险与未知诚实暴露（Risks & unknowns）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **G1** | Plan 02 Risks 列：mergerfs sha256 待首次构建测出；mutagen tarball 结构假设 A2 | **PASS** | Plan 02 §Risks（`02-binaries/PLAN.md:240-264`）4 条风险：① mergerfs `.deb` SHA256 未实测（含 amd64 / arm64 实测命令 fallback）；② mutagen tarball 内部结构未实测（`tar -tzf` 验证 + `--strip-components=N` fallback）；③ tarball 体积放大；④ arm64 构建需 QEMU。完整覆盖 RESEARCH Assumption A1/A2。|
| **G2** | Plan 06 Risks 列：buildx push:true vs `docker image inspect` 兼容性（方案 A vs B） | **PASS** | Plan 06 §Risks（`06-imagelock-ci-gate/PLAN.md:280-306`）6 条风险：① 现 workflow 是 push 还是 load 模式（含 `grep -A10` 探测命令 + 方案 A/B 分支决策 + STOP/BLOCKED fallback）；② `MANAGED_USER_IMAGE_TAG` 表达式不确定；③ 700 MB vs 700 MiB 口径；④ cache scope 冲突；⑤ ARM64 size gate 假阴性；⑥ image.lock 现字段不明。**已现网核实** `.github/workflows/build-images.yml` 是 multi-arch `push: true`（`linux/amd64,linux/arm64`），所以 executor 必须走方案 B。Plan 06 已显式准备此分支，risk 暴露充分。|
| **G3** | Plan 01 Risks 列：700MB headroom 取决于 fonts-noto-cjk-core 回退（per RESEARCH §镜像体积估算） | **PASS** | Plan 01 §Risks 1（`01-image-base/PLAN.md:274-276`）：「fonts-noto-cjk 仍是镜像体积大头（~220MB）；本 plan 不裁剪…若 Plan 06 的 CI gate 实测触发 BASE-04 失败，回到本 plan 把 `fonts-noto-cjk` 换为 `fonts-noto-cjk-core`（Research §镜像体积估算）；Fallback：（a）`fonts-noto-cjk-core` 替换；（b）mutagen-agents tarball 只保留 linux/{amd64,arm64}…」与 RESEARCH 引用一致。|
| **G4** | Plan 03 Risks 列：tmux apt 版本兼容性 for `window-size latest` | **PARTIAL** | Plan 03 §Risks 2（`03-entrypoint-config/PLAN.md:395-397`）覆盖 `assert_tmux_version` 正则覆盖范围（`3.4-3.9` / `4.x+` / `3.10` 多位数情形）+ `awk -F.` 数字比较 fallback。**未显式列出**「Ubuntu 24.04 apt 仓库 tmux 是否 ≥ 2.9（`window-size latest` 引入版本）」这一兼容性风险。`window-size latest` 在 tmux 2.9+ 即可用，远低于 3.4 floor，技术上不会失败，但 user checklist 的精确措辞期望此条目。建议 planner 在 §Risks 2 后追加一条「`window-size latest` 需 tmux ≥ 2.9，apt tmux on Ubuntu 24.04 = 3.4，已远超阈值，无兼容性风险」明确闭合。|

---

## H. 项目规则（CLAUDE.md）

| ID | 项 | 结论 | 证据 |
|----|----|------|------|
| **H1** | 无绝对路径 `/Users/`、`/home/<user>/` | **PASS** | 6 plan 全文未出现 `/Users/`、`/home/zaneliu`、`C:\Users\`。Plan 05 §Verification（`05-host-preflight-docs/PLAN.md:253`）甚至显式 `! grep -E '/(Users\|home/zaneliu)/' deploy/README.md` 反向断言。所有路径均为相对仓库根（`deploy/...`、`internal/...`、`.github/...`）或容器内绝对路径（`/etc/...`、`/usr/local/...`、`/workspace`、`/home/claude`）。|
| **H2** | 无真实凭据 / 密钥 / 邮箱 / 手机号 | **PASS** | 6 plan 中唯一硬编码的"长字符串"是 mutagen sha256 校验值（`7735286c778cc438418209f24d03a64f3a0151c8065ef0fe079cfaf093af6f8f` / `bcba735aebf8cbc11da9b3742118a665599ac697fa06bc5751cac8dcd540db8a`）——这是 Mutagen v0.18.1 GitHub release tarball 的**公开 SHA256**，非密钥；mergerfs SHA256 留空 ARG 由 executor 实测填入。Plan 03 sshd_config 改动不涉及密码字段。Plan 05 README 明确禁止 IP / 密码 / token。|
| **H3** | 中文叙述 + 英文 code/path/command | **PASS** | 6 plan 全部 Markdown 正文为中文（Goal / Scope / Tasks / Verification / Risks / Pitfalls 表头中文），代码块、文件路径、Dockerfile 指令、shell 命令、Go 标识符均英文。Pitfalls 表 / commit message 模板严格中英分离。Plan 05 README 模板章节标题（「适用范围 / 背景 / 修复步骤 / 自动检测」）也是中文。|

---

## 修订清单（Revision items — planner 必须处理）

> 以下按优先级排列。R1 关键，影响 executor 直接报错；R2-R6 偏小，影响可读性 / traceability。

### R1（关键 — Plan 05 引用了 host-preflight.sh 中实际不存在的 helper）

- **文件**：`.planning/phases/29-v3-worker/plans/05-host-preflight-docs/PLAN.md`
- **位置**：Task 5.1（`:81-160`），尤其是 `:87`、`:90-148`（函数体代码块）、`:152-160`（调用入口段落）
- **现网事实**：`deploy/scripts/host-preflight.sh` 现 44 行，**只有** `require_cmd` 一个 helper；无 `log_info` / `log_warn` / `log_fail` / `log_ok` / `log_skip`；无 `main()`；无任何 `check_*` 函数（`check_fuse3_available` 等都不存在）；失败统一 `echo … >&2; exit 1`，无 `PREFLIGHT_HAD_WARN` 累计变量。
- **要求修改**：
  - **方案 A（推荐）**：把代码骨架中的 `log_skip "..."` / `log_warn "..."` / `log_fail "..."` / `log_ok "..."` 全部替换为现网风格 `echo "..." >&2`（warn/fail/skip 都是 stderr echo）+ 仅在 fail 时 `return 1`（保持函数返回码）。把「在 main() 或主调用段按现 check_* 调用风格加入」改写为「在 `require_cmd curl`（脚本 `:41`）行**之后**、`mkdir -p /var/lib/cloud-cli-proxy`（脚本 `:43`）**之前**追加一行 `check_apparmor_fusermount3 || true`（`|| true` 防止 `set -e` 因检测项不通过中断后续 mkdir）」。
  - **方案 B（次优）**：在追加 `check_apparmor_fusermount3` 之前，先用一个独立 commit 给 `host-preflight.sh` 引入 `log_*` helper 与 `main()` 包装，再追加新函数。但 Plan 05 现非该 scope，且会膨胀 blast radius，**不推荐**。
- **同步修改**：把 §PATTERNS 引用「S2（`log_skip/log_warn/log_fail` 语义）」改为「S2（`require_cmd` helper 复用 + 失败 `echo >&2; exit/return` 风格）」；增加引用「AP9（D-23 路径修正：`fusermount3` 而非 `docker-default`，且扩展现有脚本而非新建）」。
- **影响**：不修订会让 executor 在执行 Plan 05 时遇到「`log_skip: command not found`」之类运行时错误，必须返工。

### R2（偏小 — Plan 03 entrypoint 注释未显式包含 mergerfs 参数串）

- **文件**：`.planning/phases/29-v3-worker/plans/03-entrypoint-config/PLAN.md`
- **位置**：Task 3.3（`:185-195`）`prepare_mergerfs_check` 代码骨架
- **要求修改**：在该函数注释段（当前只有 Q10 + 「mount deferred to cloud-claude」两行）追加 1-2 行明确包含 SC1 / SC2 关键参数字符串，便于 grep 自检与 Phase 31 上游引用：
  ```bash
  # SC1 / C1 / C2 mergerfs mount params (Phase 31 cloud-claude consumes):
  #   func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off
  #   category.create=ff,inodecalc=path-hash
  #   branches: /workspace-hot=RW:/workspace-cold=NC,RO （2 路；3 路扩展见 CLOUD_CLAUDE_MERGERFS_BRANCHES env）
  ```
- 同步在 §Verification（`:280-285`）追加 `grep -F 'func.readdir=cor:4' deploy/docker/managed-user/entrypoint.sh` + `grep -F 'category.create=ff' …` 两条静态断言。

### R3（偏小 — Plan 02 / Plan 05 §Out 未逐项列全 5 个 plan 归属）

- **文件**：`.planning/phases/29-v3-worker/plans/02-binaries/PLAN.md` §Out（`:58-62`）；`.planning/phases/29-v3-worker/plans/05-host-preflight-docs/PLAN.md` §Out（`:64-68`）
- **要求修改**：在两份 plan 的 §Out 末尾追加一行明确声明：
  - Plan 02：「Plan 04（contracts/worker）/ Plan 05（host-preflight + 运维 README）— 与本 plan 无文件交集，无需互斥」
  - Plan 05：「Plan 01 / 02 / 03（Dockerfile + entrypoint + tmux/profile.d/sshd_config）/ Plan 04（contracts/worker）/ Plan 06（image.lock + CI workflow）— 与本 plan 无文件交集，无需互斥」
- 目的：闭合 cross-plan boundary 追溯链，避免新进度审查者反复怀疑是否遗漏归属。

### R4（偏小 — Plan 03 §Risks 未显式覆盖 `window-size latest` 兼容性）

- **文件**：`.planning/phases/29-v3-worker/plans/03-entrypoint-config/PLAN.md` §Risks（`:388-407`）
- **要求修改**：在 Risk 2（assert_tmux_version 正则）之后或合并为 Risk 2.1，追加一条：
  > 2.1 **`window-size latest` 与 `aggressive-resize on` 的 tmux 版本下限**
  > - `window-size latest` 引入于 tmux 2.9，`aggressive-resize on` 自 tmux 1.8 起可用。Ubuntu 24.04 apt tmux 当前 = 3.4（已 ≥ 2.9），无兼容性风险。
  > - Fallback：若未来回退到更老 base image，`tmux.conf` 解析会失败导致 tmux 启动报错；Plan 03 §assert_tmux_version 的 ≥ 3.4 断言已覆盖该兜底。

### R5（偏小 — Plan 01 PATTERNS D1 引用补全）

- **文件**：`.planning/phases/29-v3-worker/plans/01-image-base/PLAN.md`
- **位置**：§Tasks 任意位置（建议在 §Goal 段尾追加一句即可）
- **要求修改**：增加一句声明「Reusable Assets D1（KasmVNC `.deb` 模板）由 Plan 02 复用 mergerfs/mutagen 下载，本 plan 不涉及 .deb 下载，故未引用 D1」，闭合 user checklist E1 的 D1 期望与实际 scope 的差异说明。

### R6（偏小 — Plan 06 PATTERNS 编号与 user checklist 不一致）

- **文件**：`.planning/phases/29-v3-worker/plans/06-imagelock-ci-gate/PLAN.md` Task 6.2 PATTERNS 引用（`:188`）
- **要求修改**：核对 `29-PATTERNS.md` 中 build-images.yml 结构 / 反内联 build-script 的实际行号编号；若实际 PATTERNS 中两者编号为 C1 + AP3（user checklist 期望），则把 Plan 06 的「C2 / C3 / AP5」更正为对应行号；若 PATTERNS 实际编号确实是 C2 / C3 / AP5，则在 Plan 06 §Tasks 顶部加一句脚注「PATTERNS C1（user checklist 旧映射）≡ 本 plan 引用的 C2（`docker image inspect`）+ C3（`::error::` 注解）」与「AP5（user checklist 旧 AP3）≡ 不修改 build-managed-image.sh」，统一术语。

---

## 备注（Notes）

1. **现网代码事实核查**：本次 check 对照真实代码读取了：
   - `deploy/scripts/host-preflight.sh`（44 行，确认无 `log_*` / `main` / `check_*`，仅 `require_cmd`）
   - `deploy/docker/managed-user/Dockerfile`（102 行，确认 Plan 01 引用的 9-41 行 apt RUN、85-87 行 mkdir RUN、102 行 ENTRYPOINT 行号准确）
   - `deploy/docker/managed-user/entrypoint.sh`（186 行，确认 Plan 03 引用的「`exec /usr/sbin/sshd -D -e` 在 186 行」准确）
   - `deploy/docker/managed-user/sshd_config`（14 行，确认 Plan 03 Task 3.2 的 Subsystem 在 14 行）
   - `deploy/docker/managed-user/image.lock`（9 行扁平 YAML，确认 Plan 06 Task 6.1 「不破现字段顺序」可执行）
   - `internal/agentapi/contracts.go`（确认 SSHKeys 在 40 行、HostActionRequest 在 21-41 行、Plan 04 引用准确）
   - `internal/runtime/tasks/worker.go`（确认 -v homeDir:homeMount 在 186 行、Labels 循环在 189 行、Plan 04 Task 4.2 引用准确；module 名为 `github.com/zanel1u/cloud-cli-proxy`，与 Plan 04 测试 import 路径一致）
   - `.github/workflows/build-images.yml`（confirms `linux/amd64,linux/arm64` + `push: true` multi-arch 模式 → Plan 06 必须走方案 B；Plan 06 §Risks 已正确暴露此选择）
2. **Phase 29 内部一致性**：Plan 01 / 02 / 03 三个 Dockerfile 改动 plan 的插入点不冲突；Plan 04 与 Dockerfile 系完全正交；Plan 05 与镜像层正交；Plan 06 与 Dockerfile 内容正交（仅外部 image.lock + workflow），可放心并行 Wave。
3. **跨 phase 依赖确认**：Plan 02 / 03 多次声明「Phase 31 消费」、Plan 04 声明「Phase 30 / Phase 33 消费」、Plan 06 声明「Phase 30 Entry API 旁路数据源」——这些跨 phase 依赖与 ROADMAP §Phase 30 / 31 / 33 的 Depends on / Scope 一致，无悬挂。
4. **commit 顺序建议**：Wave 1 中 Plan 01 / 04 / 05 可完全并行；Plan 02 必须等 Plan 01 落（Dockerfile 序列化）；Plan 03 等 Plan 01+02；Plan 06 等 Plan 01+02+03；故关键路径 = Plan 01 → 02 → 03 → 06。Plan 04 / 05 可挂 Wave 1 任意时间窗。
5. **未发现 BLOCKED 级别的问题**：所有 6 plan 的 Goal / Scope / Dependencies / 文件 ownership / Wave / Atomic Commit 均合规；CONTEXT D-01..D-30 全部映射到了至少一个 plan；REQUIREMENTS BASE-04 + Critical Pitfalls C1..C7 / M3..M18 均有归属（部分 SC1/SC2 mount-runtime 断言显式 defer 到 Phase 31，是设计选择不是缺陷）。

---

## CHECK NEEDS REVISION — 6 项需要修订（其中 R1 关键：Plan 05 引用了 host-preflight.sh 中不存在的 log_* helper，会让 executor 直接报错；其余 5 项偏小可读性 / 追溯链问题，可在 1 小时内全部修订完成）

---

## Round 2 Re-check 2026-04-18

> 复检对象：planner 在 Round 1 反馈后对 Plan 01 / 02 / 03 / 05 / 06 的增量修订；逐项确认 R1..R6 已落地、未引入回归。

### 一、R1..R6 修订落地情况

| 项 | 范围 | 复检证据 | 结论 |
|----|------|---------|------|
| **R1**（关键 — Plan 05 host-preflight 风格对齐） | `plans/05-host-preflight-docs/PLAN.md` Task 5.1 | (a) 函数体（`:110-168`）整段只用 `echo "..." >&2` + `cat >&2 <<'FIX_INSTRUCTIONS'` + `return 0/1`，**全文未出现** `log_info` / `log_warn` / `log_fail` / `log_ok` / `log_skip` / `main(` / `PREFLIGHT_HAD_WARN`（grep 0 hit）。(b) 插入点严格描述（`:98-100`）："函数定义紧随 `require_cmd` helper 结束行（现 9 行）后、第一条 `require_cmd docker`（现 11 行）前；调用入口在 `require_cmd curl`（现 41 行）后、`mkdir -p /var/lib/cloud-cli-proxy`（现 43 行）前"——与 R1 期望完全一致。(c) 调用形式 `check_apparmor_fusermount3 \|\| true`（`:175`）——advisory 非阻断。(d) 非 Ubuntu 分支 `:118-120` 用 `return 0`（不是 `exit 0`），版本不足分支 `:127-130` 同样 `return 0`。(e) §Verification 反向断言 `:273-277` 显式 grep `! '^(log_info\|log_warn\|log_fail\|log_ok\|log_skip)\\(' ` + `! '^main\\(\\)'` + `! 'PREFLIGHT_HAD_WARN'` + `! 'exit 0'`（函数体内）。(f) §PATTERNS 行 `:186` 已改为「S2（复用 `require_cmd` helper 风格 —— 失败统一 `echo ... >&2` + `exit/return` 传递；本 plan 不引入 `log_*` helper）」并新增 AP9（D-23 路径修正）引用。(g) §Scope §In `:51-60` 全段重写为现网风格描述并显式禁止 `log_*` / `main()` / `PREFLIGHT_HAD_WARN`。 | **PASS** |
| **R2**（Plan 03 mergerfs 参数串字面化 + grep 自检） | `plans/03-entrypoint-config/PLAN.md` Task 3.3 + §Verification | (a) `prepare_mergerfs_check` 注释段（`:193-199`）字面包含 `func.readdir=cor:4`、`category.create=ff`、`inodecalc=path-hash` 三个关键参数串。(b) 同函数体 `:200` 追加 echo 日志 `[entrypoint] v3: expected mergerfs params (documented for Phase 31): func.readdir=cor:4 category.create=ff`，让运行时日志也可断言。(c) §Verification §静态断言 `:293-295` 新增 3 条 grep -F 静态断言：`func.readdir=cor:4` / `category.create=ff` / `inodecalc=path-hash` 必须出现在 entrypoint.sh。(d) §Pitfalls 表 C1 / C2 行（`:387-388`）已同步更新为「参数字符串 ... 硬编码到 `prepare_mergerfs_check` 注释 + echo 日志（Phase 29 静态 grep 可断言；真正挂载由 Phase 31 消费）」。 | **PASS** |
| **R3**（Plan 02 / Plan 05 §Out 显式列全其它 5 plan） | `plans/02-binaries/PLAN.md` §Out + `plans/05-host-preflight-docs/PLAN.md` §Out | (a) Plan 02 §Out（`:58-67`）分两段：第一段「Dockerfile 共享但责任切分」逐条列出 Plan 01 / 03 / 06；第二段「与本 plan 无文件交集」显式列出 Plan 04 / 05。完全闭合。(b) Plan 05 §Out（`:67-79`）顶部 3 行原 Out（D-24 deferred / 现有 require_cmd 不动 / C6 daemon.json 后续阶段）+ 下方「其余 5 plan 归属」段（`:72-78`）逐条列出 Plan 01 / 02 / 03 / 04 / 06 各自负责的文件 + 与本 plan「无文件交集，可 Wave 1 并行」。 | **PASS** |
| **R4**（Plan 03 §Risks 增加 `window-size latest` 兼容性条目） | `plans/03-entrypoint-config/PLAN.md` §Risks | Risk 2.1（`:411-414`）新增「`window-size latest` / `aggressive-resize on` 与 tmux apt 版本兼容性（R4）」：明确说明 `window-size latest` 引入于 tmux 2.9，`aggressive-resize on` 自 1.8 起可用，Ubuntu 24.04 apt tmux = 3.4 已远超 2.9 下限；fallback 用 `assert_tmux_version` 的 ≥ 3.4 硬 gate 兜底。措辞与 user checklist 期望一致。 | **PASS** |
| **R5**（Plan 01 PATTERNS D1 / KasmVNC `.deb` 复用声明） | `plans/01-image-base/PLAN.md` §Goal | §Goal 段尾新增整段 `> Wave Coordination / Reusable Assets 备注（R5）`（`:45`）：明确声明 `29-PATTERNS.md §Reusable Assets §Dockerfile` D1（KasmVNC `.deb` 下载 pattern，现 `Dockerfile:47-54`）是 Plan 02 下载 mergerfs / mutagen 的直接 analog；本 plan 保留 KasmVNC RUN 不动，故未引用 D1（不是遗漏而是 scope 刻意排除）。traceability 闭合。 | **PASS** |
| **R6**（Plan 06 PATTERNS 引用统一为 C1 + C2 + AP3 + 29-PATTERNS.md anchors） | `plans/06-imagelock-ci-gate/PLAN.md` Task 6.1 + Task 6.2 | (a) Task 6.2 §PATTERNS 段（`:188-192`）重写为 4 条带 `29-PATTERNS.md §Reusable Assets §CI` / `§Anti-patterns` 锚的引用：**C1**（multi-arch buildx + GHCR push matrix 基线）+ **C2**（`echo "X=Y" >> $GITHUB_ENV` 动态 env 模式）+ **AP3**（不把 size gate 塞进 `build-managed-image.sh`）+ 「新增模式（无 analog）」声明 `docker image inspect --format='{{.Size}}'` + `::error::` 是 phase 首次引入。已删除 Round 1 引用的 AP5。(b) Task 6.1 §PATTERNS 段（`:106`）同步更新为引用「`29-PATTERNS.md §汇总表 Sub-scope F`」+ 显式说明「§Reusable Assets §CI 的 C1 / C2 是 workflow 资产，不适用于 image.lock；本 task 引用的是汇总表 Sub-scope F 行而非 Reusable Assets 编号」——避免新审查者把 image.lock 的 C 与 workflow 的 C 混淆。(c) §Pitfalls 表 AP3 行（`:277`）也已同步引用 `29-PATTERNS.md §Anti-patterns`。 | **PASS** |

### 二、回归检查（确保 Round 1 已 PASS 项未在修订过程中被破坏）

| 项 | 复检证据 | 结论 |
|----|---------|------|
| **D-23 修正路径**（Plan 05） | front-matter `files_modified` `:9` `deploy/scripts/host-preflight.sh`；must_haves `:19 / :30` 与 Task 5.1 `:106 / :146` 与 §Verification `:252 / :288` 全部使用 `/etc/apparmor.d/local/fusermount3`；全文 grep `docker-default` = **0 hit**。 | **PASS（无回归）** |
| **D-19 `,readonly` 无值标志**（Plan 04） | Task 4.2 `:124-127` 仍为 `opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target); if vm.ReadOnly { opts += ",readonly" }`；`:131` 显式注释「`,readonly` 无值标志（不是 `,ro`；RESEARCH §Code Examples 明确）」；§Verification 静态断言保留对 `",readonly"` 的 grep。 | **PASS（无回归）** |
| **D-10 tini ENTRYPOINT exec form**（Plan 01） | Task 1.4 `:184-186` 仍为 `ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]`；§Verification `:215` 仍 `grep -F` 完全匹配该 JSON 数组字符串。 | **PASS（无回归）** |
| **D-22 `omitempty` slice 字段**（Plan 04） | Task 4.1 `:104` 仍为 `Volumes []VolumeMount \`json:"volumes,omitempty"\``；Task 4.4 `TestHostActionRequest_VolumesOmitempty` `:171-221` 仍存在并 round-trip 断言空 Volumes 不出现在 JSON、ReadOnly=false 不序列化、ReadOnly=true 序列化。 | **PASS（无回归）** |
| **Plan 03 串行调用顺序 + chown 范围**（C5 / M17 / AP10） | Task 3.3 `:222-229` 调用顺序 `prepare_v3_dirs → prepare_mutagen_agent → prepare_mergerfs_check → assert_tmux_version → exec sshd` 不变；`prepare_v3_dirs` `:161-168` chown 列表仍为 `/home/claude /workspace-hot /workspace-cold /var/lib/claude-persist`，**未**包含 `/workspace`（AP10 仍生效）。§Verification awk 顺序断言 `:296-297` 保留。 | **PASS（无回归）** |
| **Plan 06 size gate 数值 + D-30 反向断言** | Task 6.2 `:135-143` 的 `734003200` bytes（700 MiB）阈值 + `::error::` GitHub 注解保留；§Verification `:217-221` 保留 `git diff -- deploy/scripts/build-managed-image.sh` 必须为空的反向断言；§Pitfalls 表 D-30 行（`:275`）保留。 | **PASS（无回归）** |
| **Plan 02 sha256 校验 + Wave 2 依赖** | Task 2.1 / 2.2 `:88-141` 的 `curl + sha256sum -c - + dpkg -i` / `tar -xzf` 模板未变；mutagen amd64 / arm64 SHA256 字面值（`7735286c...` / `bcba735a...`）保留；§Verification `:181-182` grep 仍命中。 | **PASS（无回归）** |

### 三、附加抽查（Plan 05 §Verification R1 强化）

Plan 05 §Verification 静态断言段（`:247-294`）新增 5 条 R1 反向断言（除上述 R1 列已述外）：

```bash
# R1 反向断言：严禁引入现网不存在的 log_* / main / PREFLIGHT_HAD_WARN helper
! grep -Eq '^(log_info|log_warn|log_fail|log_ok|log_skip)[[:space:]]*\(' deploy/scripts/host-preflight.sh
! grep -Eq '^main[[:space:]]*\(\)' deploy/scripts/host-preflight.sh
! grep -F 'PREFLIGHT_HAD_WARN' deploy/scripts/host-preflight.sh
# R1 反向断言：非 Ubuntu / 版本不足分支用 `return 0` 而非 `exit 0`
! awk '/check_apparmor_fusermount3\(\)/{in_fn=1} in_fn && /^}/{in_fn=0} in_fn && /exit 0/' deploy/scripts/host-preflight.sh
# 调用位置 awk 三点对齐断言（curl 行 < 调用行 < mkdir 行）
awk '... /^require_cmd curl$/.../^check_apparmor_fusermount3.*\|\|.*true/.../^mkdir -p \/var\/lib\/cloud-cli-proxy/ ...'
```

这些断言既能在 executor 落代码后立刻自检，又会在 Round 3 任何回归场景下触发硬失败，使 R1 修订持续可观察。

### 四、Plan 01 / 02 / 03 / 04 / 06 边界 / Wave / 原子提交策略未变

复检逐 plan front-matter `wave` / `depends_on` / `files_modified` 字段：

| Plan | wave | depends_on | files_modified 集合 | 与 Round 1 是否一致 |
|------|------|------------|---------------------|---------------------|
| 01-image-base | 1 | `[]` | `Dockerfile` | 一致 |
| 02-binaries | 2 | `["01-image-base"]` | `Dockerfile` | 一致 |
| 03-entrypoint-config | 3 | `["01-image-base", "02-binaries"]` | `entrypoint.sh / tmux.conf / profile.d-cloud-claude.sh / sshd_config / Dockerfile` | 一致 |
| 04-worker-contract | 1 | `[]` | `internal/agentapi/contracts.go / internal/runtime/tasks/worker.go / internal/runtime/tasks/worker_volume_test.go` | 一致 |
| 05-host-preflight-docs | 1 | `[]` | `deploy/scripts/host-preflight.sh / deploy/README.md` | 一致 |
| 06-imagelock-ci-gate | 3 | `["01-image-base", "02-binaries", "03-entrypoint-config"]` | `image.lock / .github/workflows/build-images.yml` | 一致 |

依赖图、原子提交策略（Plan 01=4 / 02=3 / 03=4 / 04=3 / 05=2 / 06=2 commit）、commit message 前缀（`feat(29-XX)` / `test(29-XX)` / `ci(29-XX)` / `docs(29-XX)`）均未在 Round 2 修订过程中被破坏。

### 五、Round 2 结论

- **R1..R6 全部 PASS**（6/6）
- **回归检查 7 项全部 PASS**
- 未发现新引入的 blocker / warning
- 6 plan 现状满足"executor 可直接落地"门槛

最终判定：**APPROVED**。可执行 `/gsd-execute-phase 29-v3-worker`，按 Wave 1（Plan 01 / 04 / 05 并行）→ Wave 2（Plan 02）→ Wave 3（Plan 03 / 06 并行）顺序推进。Plan 05 Round 1 关键问题（host-preflight.sh helper 不存在）已通过将函数体改写为现网 `echo >&2` + `return` 风格 + 多重反向断言彻底解决；executor 落代码时若不慎回归到 `log_*` 风格，Verification 阶段会立即触发硬失败。

---

## Round 2 Re-check 2026-04-18（独立复核 / second pass）

> 第二轮独立复核：在前一次 Round 2 报告结论之上，对 6 份 PLAN.md 做一次仅依赖源文本的再次 grep / 行号交叉核对，确认 R1..R6 的修订文字在当前 HEAD 中字面存在，且未引入已知回归。本节不触碰 plans / source-of-truth，只追加到本报告文末。

### R1..R6 复核结果（逐项字面引用）

| 项 | 核查点 | 证据（行号 + 字面截取） | 结论 |
|----|--------|------------------------|------|
| **R1** | Plan 05 Task 5.1 仅使用 `echo ... >&2` / `cat >&2 <<'FIX_INSTRUCTIONS'` / `return 0/1`；全文无 `log_info/log_warn/log_fail/log_ok/log_skip`；非 Ubuntu 分支 `return 0`；调用 `check_apparmor_fusermount3 \|\| true`；插入位置为 `require_cmd curl`（现 41 行）后、`mkdir -p /var/lib/cloud-cli-proxy`（现 43 行）前 | `plans/05-host-preflight-docs/PLAN.md`：`:57` 描述「调用入口插在 `require_cmd curl`（现 41 行）**之后**、`mkdir -p /var/lib/cloud-cli-proxy`（现 43 行）**之前**」；`:100` 复述插入点；`:110-168` 函数体—`return 0`（113/120/129/136/142/166）、`return 1`（163）、`cat >&2 <<'FIX_INSTRUCTIONS'`（149）；`:175` `check_apparmor_fusermount3 \|\| true`；`:179-183` 严禁清单；`:273-275` 反向断言 `! grep -Eq '^(log_info\|log_warn\|log_fail\|log_ok\|log_skip)[[:space:]]*\(' ...` + `! grep -Eq '^main[[:space:]]*\(\)' ...` + `! grep -F 'PREFLIGHT_HAD_WARN' ...` | **PASS** |
| **R2** | Plan 03 `prepare_mergerfs_check` 含字面 `func.readdir=cor:4` + `category.create=ff`；§Verification 新增 3 条 `grep -F` 静态断言 | `plans/03-entrypoint-config/PLAN.md`：`:195` 注释 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true,cache.files=off`；`:196` 注释 `category.create=ff,inodecalc=path-hash`；`:200` runtime echo `func.readdir=cor:4 category.create=ff`；`:293-295` 三条静态断言 `grep -F 'func.readdir=cor:4'` / `grep -F 'category.create=ff'` / `grep -F 'inodecalc=path-hash'` | **PASS** |
| **R3** | Plan 02 §Out + Plan 05 §Out 显式列出其它 5 plan | Plan 02 `:58-67` 分段列出「Dockerfile 共享」Plan 01 / 03 / 06 + 「无文件交集」Plan 04 / 05；Plan 05 `:67-79` 顶部保留原 Out（D-24 / require_cmd / C6）+ `:72-78` 逐条声明 Plan 01 / 02 / 03 / 04 / 06 的文件归属 | **PASS** |
| **R4** | Plan 03 §Risks 新增 `window-size latest` 兼容性条目 | `plans/03-entrypoint-config/PLAN.md`：`:411-414` Risk 2.1 行文「`window-size latest` 引入于 **tmux 2.9**，`aggressive-resize on` 自 tmux 1.8 起可用；Ubuntu 24.04 apt tmux 当前版本 = **3.4**，已远超 2.9 下限」+ `assert_tmux_version` ≥ 3.4 硬 gate 兜底描述 | **PASS** |
| **R5** | Plan 01 声明 D1（KasmVNC `.deb`）由 Plan 02 复用 | `plans/01-image-base/PLAN.md`：`:45` 整段 blockquote「`29-PATTERNS.md §Reusable Assets §Dockerfile` D1（KasmVNC `.deb` 下载 pattern …）是 **Plan 02** 下载 mergerfs / mutagen 的直接 analog；本 plan **保留 Dockerfile 现 47-54 行 KasmVNC RUN 不动**…」 | **PASS** |
| **R6** | Plan 06 Task 6.2 PATTERNS 引用统一为 C1 + C2 + AP3（29-PATTERNS.md anchors），无 AP5 残留 | `plans/06-imagelock-ci-gate/PLAN.md`：`:188-192` 四条 bullet：**C1** multi-arch buildx + GHCR push matrix、**C2** `echo "X=Y" >> $GITHUB_ENV`、**AP3** 不把 size gate 塞进 `build-managed-image.sh`、「新增模式（无 analog）」`docker image inspect`；`:277` §Pitfalls 表 AP3 行锚 `29-PATTERNS.md §Anti-patterns`；全文 `grep -F 'AP5'` = **0 hit**（R6 残留清零） | **PASS** |

### 回归点独立复核

| 回归点 | 证据 | 结论 |
|--------|------|------|
| Plan 05 仍用 `/etc/apparmor.d/local/fusermount3`（非 docker-default） | `plans/05-host-preflight-docs/PLAN.md`：`:30` / `:106` / `:146` / `:151` / `:158` / `:252` / `:288` 全部使用 `/etc/apparmor.d/local/fusermount3`；全文 `grep -F 'docker-default'` = **0 hit** | **PASS** |
| Plan 05 扩展 `deploy/scripts/host-preflight.sh`（不新建 `deploy/host-preflight.sh`） | front-matter `:9` `deploy/scripts/host-preflight.sh`；全文任何 grep 均指向 `deploy/scripts/host-preflight.sh`，无 `deploy/host-preflight.sh` 出现 | **PASS** |
| Plan 04 使用 `--mount type=volume,src=X,dst=Y,readonly`（逗号 + readonly，不是 `,ro`） | `plans/04-worker-contract/PLAN.md`：`:124` `opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)`；`:125-127` `if vm.ReadOnly { opts += ",readonly" }`；`:131` 显式注释「`,readonly` 无值标志（不是 `,ro`；RESEARCH §Code Examples 明确）」 | **PASS** |
| Plan 04 Volumes slice 用 `omitempty` | `:21` truths「`Volumes []VolumeMount \`json:\"volumes,omitempty\"\``」；`:58` In 段；`:104` 代码片段 `Volumes []VolumeMount     \`json:"volumes,omitempty"\``；`:171-221` `TestHostActionRequest_VolumesOmitempty` round-trip + ReadOnly omitempty 三子用例 | **PASS** |
| Plan 01 ENTRYPOINT exec form `["/usr/bin/tini","--","/usr/local/bin/entrypoint.sh"]` | `plans/01-image-base/PLAN.md`：`:36` must_haves pattern；`:59` In 描述；`:186` Task 1.4 代码片段；`:215` Verification `grep -F` 字面字符串 | **PASS** |

### 独立复核判定

- R1..R6 全部 PASS（6/6）
- 5 项回归点全部 PASS
- 未发现前一次 Round 2 结论之外的新问题
- 与现有顶部 front-matter `verdict: APPROVED (Round 2)` 一致

最终判定：**APPROVED**，维持前一次 Round 2 结论，不需追加修订。Plan 29 计划集进入 `/gsd-execute-phase 29-v3-worker` 的前置条件（PLAN.md 完备 + 独立复核无阻断）已满足。

---

## CHECK APPROVED — Round 2 独立复核：R1..R6 字面修订与 5 项回归点全部保持 PASS，Phase 29 计划集维持 APPROVED 可执行
