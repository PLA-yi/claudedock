---
phase: 260416-wvu
plan: 01
subsystem: runtime/tasks
tags: [ssh, idempotency, authorized_keys, bugfix]
dependency_graph:
  requires:
    - internal/runtime/tasks/worker.go（injectSSHKeys / injectSSHKeysLegacy 既有骨架）
    - internal/agentapi/contracts.go（SSHKeyEntry / HostActionRequest，只读引用）
  provides:
    - injectSSHKeys / injectSSHKeysLegacy 的幂等语义
    - authorized_keys 的 marker 合并能力
    - runtime.ssh_key_skipped_existing 事件
    - runtime.ssh_key_chown_failed 事件
  affects:
    - create / start / rebuild(preserve-home) 下的 SSH 密钥注入路径
tech_stack:
  added: []
  patterns:
    - package-level 可注入闭包（execInContainer）用于对 docker exec 做单测级替换
    - authorized_keys 的 begin/end marker 块合并（非破坏式重写）
key_files:
  created:
    - internal/runtime/tasks/ssh_inject_test.go
  modified:
    - internal/runtime/tasks/worker.go
decisions:
  - "使用 `# >>> claudedock managed keys (do not edit) >>>` / `<<<` 成对 marker 包裹控制面权威条目，用户自加行保持原位。"
  - "跳过覆盖用户手生成私钥/公钥时，仍主动修正属主与权限（chown+chmod），并把任何失败记录为 runtime.ssh_key_chown_failed warn 事件。"
  - "docker exec 封装为 package-level `var execInContainer`，测试通过替换该变量消除 docker 依赖。"
  - "mergeAuthorizedKeys 抽成纯函数：managed 空 + existing 空 → 返回空串由调用方 skip；managed 空 + existing 非空且无 marker → 返回 existing 原样不触发写入。"
metrics:
  duration: 约 1 小时（3 个原子提交）
  completed: 2026-04-16
  tasks: 3
  files_touched: 2
---

# Phase 260416-wvu Plan 01: 让 injectSSHKeys 变幂等，保留用户手加密钥 Summary

**一句话总结：** 把 `injectSSHKeys` / `injectSSHKeysLegacy` 从"整把覆盖"改成"存在即跳过 + authorized_keys 按 marker 块合并"，控制面 rebuild / 重启不再刷掉用户在容器里 `ssh-keygen` 出的 id_ed25519 / id_rsa 与手加的 authorized_keys 行。

## 背景与目标

现实症状：用户在容器里手动 `ssh-keygen` 后，一次 `rebuild(preserve-home)` 或宿主机重启就被控制面整把刷掉 `/workspace/.ssh/id_*`，密钥链路断裂。目标要同时满足三件事：

- 用户手加 / 手生成的 key 存续（不被覆盖）；
- 控制面权威条目（proxy 公钥 + DB `purpose=inbound` 的公钥）仍然生效且幂等稳定；
- 首次自举（容器里什么都没有）依然能把 DB outbound 密钥写入容器。

## 修改文件

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/runtime/tasks/worker.go` | 修改 | 抽出 `execInContainer` helper、新增 marker 常量、加入 `containerFileNonEmpty` / `containerReadFile` / `mergeAuthorizedKeys`、outbound 私钥/公钥与 legacy 路径加入"已存在则跳过 + 仅修正属主"、authorized_keys 改为 marker 合并。 |
| `internal/runtime/tasks/ssh_inject_test.go` | 新增 | 通过替换 package-level `execInContainer` 与 fake 文件系统，覆盖 5 个幂等 / 合并 case，零 docker 依赖。 |

## 执行任务与提交

| Task | 内容 | Commit |
|------|------|--------|
| 1 | `refactor(runtime): 抽出 execInContainer helper 与 managed keys marker 常量` —— 零行为变更铺路。 | `78b416c` |
| 2 | `fix(runtime): injectSSHKeys 幂等且合并 authorized_keys，保留用户手加密钥` —— 真正的行为修复。 | `b0266fa` |
| 3 | `test(runtime): 覆盖 injectSSHKeys 幂等与 authorized_keys 合并场景` —— 新增 5 个子测试。 | `7bda3cb` |

## 关键决策

1. **Marker 合并（非破坏式）**：`authorized_keys` 用 `# >>> claudedock managed keys (do not edit) >>>` 与 `# <<< claudedock managed keys <<<` 成对包裹控制面条目。marker 外的行完全不动，用户手加的条目永远保留。`mergeAuthorizedKeys` 作为纯函数方便单测直接覆盖边界。
2. **"已存在则跳过"适用于 outbound 私钥/公钥与 legacy 路径**：用 `containerFileNonEmpty` 做存在性检查（经 `[ -s ]`），命中即走 skip 分支并写 `runtime.ssh_key_skipped_existing` 事件；但仍然会对已有文件跑一遍 `chown user:user && chmod 600/644`，保证属主/权限在 rebuild 后不漂移。属主修正失败记 `runtime.ssh_key_chown_failed` warn 事件，不影响主流程。
3. **docker exec 可注入**：用 `var execInContainer = func(ctx, container, script, stdin) ([]byte, error)` 包裹所有 docker exec，测试用闭包替换；运行时行为完全不变，仅是执行点被函数化，便于 5 个 case 的 fake 文件系统模拟。
4. **managed 为空 + existing 为空不创建文件**：避免出现"仅有 marker 却没有任何 key"的空壳文件；`merged == existing` 时也不重写，完全幂等。
5. **path 走 stdin**：`containerFileNonEmpty` / `containerReadFile` 的脚本用 `P=$(cat)` 从 stdin 读 path，避免脚本字符串层面的 shell 拼接注入风险。

## 验证结果

- `go build ./...`：通过。
- `go vet ./internal/runtime/tasks/...`：通过。
- `go test ./internal/runtime/tasks/... -count=1`：全绿，包含新增 `TestInjectSSHKeys` 5 个子测试（`empty_container_writes_outbound` / `existing_outbound_is_preserved` / `authorized_keys_fresh_write` / `authorized_keys_preserves_user_lines` / `stable_on_second_call`），以及既有 `TestBuildSSHHandoffMetadata` / `TestWaitForSSHReady`。

### Acceptance Criteria 核验

- `rg 'execInContainer\(' internal/runtime/tasks/worker.go` → 8 处（≥ 5，满足）。
- `rg 'runtime.ssh_key_skipped_existing' internal/runtime/tasks/worker.go` → 4 处（≥ 2，满足）。
- `rg 'mergeAuthorizedKeys\(' internal/runtime/tasks/worker.go` → 2 处（1 定义 + 1 调用，满足）。
- `rg 'containerFileNonEmpty\(' internal/runtime/tasks/worker.go` → 5 处（≥ 3，满足）。
- `rg 'sshManagedBeginMarker|sshManagedEndMarker' internal/runtime/tasks/worker.go` → 声明 + 合并实现中各有使用。
- `rg 'exec.CommandContext\(ctx, "docker", "exec", "-i", containerName' internal/runtime/tasks/worker.go` → 仅剩 1 处 `syncContainerCredentials` 中 chpasswd 调用（PLAN 明确允许保留）。
- 测试文件侧 5 个子测试名、`execInContainer =` 替换、`ssh_key_skipped_existing` 断言均命中。

## 实际 vs 计划差异

- **Task 1 acceptance 文案微调**：PLAN 原文写"`rg "exec.CommandContext(...containerName" worker.go` 无输出，或仅剩 `syncContainerCredentials` 中 chpasswd 场景"。实际实现确实仅剩 chpasswd 一处，落在计划允许的豁免里，未进一步调整。
- 其余无偏离。

## Deviations from Plan

无自动修复类偏离（Rules 1-3 未触发）。所有任务按 PLAN.md 执行，没有架构层面的改变。

## 未做/边界

- **entrypoint.sh、DB schema、SSHKeyEntry、RebuildMode、前端、migration、API 边界**：按约束未动。
- **legacy 路径的单独端到端测试**：PLAN 标为 optional bonus，本次未加子测试；legacy 路径的跳过逻辑已与 outbound 一致，靠代码对称性 + vet 保障，没有专门断言。
- **真实 docker 下的 smoke（rebuild preserve-home → `ls /workspace/.ssh/` → `cat authorized_keys`）**：本次属于单元测试验证范围，留待在具备 docker 宿主机的环境下人工跑一次 smoke。

## 下一步观察项

1. 在真实宿主机上触发一次 `rebuild(preserve-home)`，进容器检查 `/workspace/.ssh/id_ed25519{,.pub}` 与 `authorized_keys`，确认用户手加行与 marker 块同时存在。
2. 观察控制面事件流：一次 rebuild 后应出现若干 `runtime.ssh_key_skipped_existing`（如果容器里已有 key），而不是每次都覆盖写入；必要时配仪表盘计数。
3. 若未来引入"强制刷新 outbound 密钥"的运维动作，应走显式 API/脚本，而不是把 `injectSSHKeys` 改回覆盖写 —— 在设计层保住"默认幂等"的不变量。

## Self-Check: PASSED

- `.planning/quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/260416-wvu-SUMMARY.md` 已由本次写入（待 final commit 入库）。
- `internal/runtime/tasks/ssh_inject_test.go` 存在，且 `TestInjectSSHKeys` 5 个子测试在 `go test` 中全绿。
- `internal/runtime/tasks/worker.go` 已修改，包含 `execInContainer` / `mergeAuthorizedKeys` / `containerFileNonEmpty` / `containerReadFile` / marker 常量。
- 提交哈希核验（`git log --oneline 157e97f..HEAD`）：
  - `78b416c` refactor(runtime): 抽出 execInContainer helper 与 managed keys marker 常量 — FOUND。
  - `b0266fa` fix(runtime): injectSSHKeys 幂等且合并 authorized_keys，保留用户手加密钥 — FOUND。
  - `7bda3cb` test(runtime): 覆盖 injectSSHKeys 幂等与 authorized_keys 合并场景 — FOUND。
