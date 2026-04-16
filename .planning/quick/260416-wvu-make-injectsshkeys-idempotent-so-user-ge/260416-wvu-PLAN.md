---
phase: 260416-wvu
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/runtime/tasks/worker.go
  - internal/runtime/tasks/ssh_inject_test.go
autonomous: true
requirements:
  - idempotent-ssh-inject
must_haves:
  truths:
    - "容器内用户手生成的 /workspace/.ssh/id_ed25519 / id_rsa / 对应 .pub 在 create/start/rebuild 后不被控制面覆盖。"
    - "authorized_keys 中用户手加的条目（marker 之外）在控制面下一次注入后仍然存在。"
    - "控制面的权威条目（proxy 公钥 + DB 里 purpose=inbound 的公钥）被写在一对 marker 注释之间，重复执行稳定不漂移。"
    - "outbound 私钥/公钥文件首次缺失时仍然会被写入（首次自举保持可用）。"
    - "跳过覆盖时，controller 事件表出现 runtime.ssh_key_skipped_existing 事件，payload 包含 host_id 与 file。"
    - "injectSSHKeysLegacy 走到时表现出与 injectSSHKeys 对 outbound 文件一致的幂等语义。"
    - "go test ./internal/runtime/tasks/... 全绿，新增 ssh_inject_test.go 覆盖 5 个 case。"
  artifacts:
    - path: "internal/runtime/tasks/worker.go"
      provides: "injectSSHKeys / injectSSHKeysLegacy 幂等写入 + authorized_keys marker 合并"
      contains: "cloud-cli-proxy managed keys"
    - path: "internal/runtime/tasks/ssh_inject_test.go"
      provides: "injectSSHKeys 幂等与合并行为的单元测试"
      contains: "ssh_key_skipped_existing"
  key_links:
    - from: "internal/runtime/tasks/worker.go:injectSSHKeys"
      to: "docker exec bash helper"
      via: "package-level commandRunner (可测试替换)"
      pattern: "execInContainer"
    - from: "authorized_keys 合并器"
      to: "marker 常量"
      via: "字符串拼接后整体 cat > 写回"
      pattern: ">>> cloud-cli-proxy managed keys"
---

<objective>
让 `internal/runtime/tasks/worker.go` 中的 `injectSSHKeys` / `injectSSHKeysLegacy` 在 create/start/rebuild 触发的 `waitForSSH` 里变成幂等操作：不再覆盖用户在容器内手生成的 `/workspace/.ssh/id_*` 私钥/公钥；`authorized_keys` 改为 marker 块合并，保留用户手加的行。

Purpose: 现实问题是用户在容器里自己 `ssh-keygen` 后一次 rebuild / host 重启就被控制面整把刷掉，密钥链路断裂。goal-backward 看，"用户手加的 key 存续" + "控制面权威条目仍生效" + "首次自举仍可用"要同时满足。
Output: worker.go 内部小重构 + 行为修复 + 新增单测文件 `ssh_inject_test.go`。
</objective>

<execution_context>
@.cursor/get-shit-done/workflows/execute-plan.md
@.cursor/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/STATE.md

<read_first>
- internal/runtime/tasks/worker.go（重点 280-565：rebuildHost / waitForSSH / injectSSHKeys / injectSSHKeysLegacy；辅助区 597-640：loadProxyPublicKey / runDocker / containerExists）
- internal/agentapi/contracts.go（SSHKeyEntry / HostActionRequest：确认字段名 Purpose、KeyType、Label、PrivateKey、PublicKey、SSHPublicKey、SSHPrivateKey）
- internal/runtime/runtime_service.go（100-160：keyEntries 装配来源，确认测试不需要改这层）
- internal/runtime/tasks/ssh_handoff_test.go（单测风格参考）
- internal/runtime/tasks/ssh_ready_test.go（单测风格参考 + 如何替换 package-level 变量进行注入）
- deploy/docker/managed-user/entrypoint.sh（只读：确认容器初始化不会主动生成 id_* —— 本次 **不改** 此文件）
</read_first>

<interfaces>
关键契约（从 worker.go 抽取，供执行者直接引用，不必再去翻代码）：

```go
// 既有入口（签名保持不变）：
func (w *Worker) injectSSHKeys(ctx context.Context, request agentapi.HostActionRequest, containerName string)
func (w *Worker) injectSSHKeysLegacy(ctx context.Context, request agentapi.HostActionRequest, containerName string)

// 目标 package-level 可注入点（新增；默认实现调用 docker exec）：
// 职责：在目标容器中以 bash -c 执行脚本，支持可选 stdin 输入，返回 stdout/stderr 合并输出与 error。
var execInContainer = func(ctx context.Context, container, script string, stdin string) ([]byte, error) {
    cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
    if stdin != "" {
        cmd.Stdin = strings.NewReader(stdin)
    }
    return cmd.CombinedOutput()
}

// Marker 常量（用于 authorized_keys 合并）：
const (
    sshManagedBeginMarker = "# >>> cloud-cli-proxy managed keys (do not edit) >>>"
    sshManagedEndMarker   = "# <<< cloud-cli-proxy managed keys <<<"
)
```

`agentapi.SSHKeyEntry` 相关字段（只读引用）：`Purpose`（`inbound` / `outbound`）、`KeyType`（`rsa` / 空）、`Label`、`PrivateKey`、`PublicKey`。
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: 抽出 execInContainer helper 与 marker 常量（零行为变更铺路）</name>
  <files>internal/runtime/tasks/worker.go</files>
  <read_first>
    - worker.go 当前 injectSSHKeys / injectSSHKeysLegacy 中每一处 `exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script)` 调用
    - worker.go 头部 import 块（package + imports）
  </read_first>
  <action>
在 worker.go 内新增两处纯铺路改动，**不改变任何对外行为**：

1. 在文件底部（与 `loadProxyPublicKey` 同区域）新增 package-level 变量：
   ```go
   var execInContainer = func(ctx context.Context, container, script, stdin string) ([]byte, error) {
       cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "bash", "-c", script)
       if stdin != "" {
           cmd.Stdin = strings.NewReader(stdin)
       }
       return cmd.CombinedOutput()
   }
   ```
   使用 `var` 而不是 `func`，保证测试可以临时替换。命名、位置与 `loadProxyPublicKey` 保持风格一致。

2. 在同一文件顶部的 `const` 区（`defaultWorkspaceMount` 附近或新增独立 const 块）新增 authorized_keys marker 常量：
   ```go
   const (
       sshManagedBeginMarker = "# >>> cloud-cli-proxy managed keys (do not edit) >>>"
       sshManagedEndMarker   = "# <<< cloud-cli-proxy managed keys <<<"
   )
   ```

3. 把 `injectSSHKeys` 与 `injectSSHKeysLegacy` 内部**所有**形如 `cmd := exec.CommandContext(ctx, "docker", "exec", "-i", containerName, "bash", "-c", script); cmd.Stdin = strings.NewReader(content); if out, err := cmd.CombinedOutput(); ...` 的段落，改为调用 `out, err := execInContainer(ctx, containerName, script, content)`。语义、事件记录逻辑、日志文案、错误分支保持完全不变。

4. 本 task 结束时，`injectSSHKeys` / `injectSSHKeysLegacy` 的外部可观测行为（事件写入、执行的脚本字符串、stdin 内容）应与改动前字节级一致 —— 仅是执行点被函数化。不删除 `cmd := exec.CommandContext(...)` 以外的任何代码。

5. 若修改后导致 `os/exec` 或 `strings` import 变得仅在单处使用，保留；本 task 不要清理 import，避免干扰 diff 审阅。

**约束**：
- 不改 entrypoint.sh、不改 DB schema、不改 SSHKeyEntry、不改 RebuildMode 语义。
- 不要在本 task 引入"跳过存在文件"或"authorized_keys 合并"逻辑 —— 那是 Task 2 的事。
- commit message 使用中文：`refactor(runtime): 抽出 execInContainer helper 与 managed keys marker 常量`
  </action>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./internal/runtime/tasks/...</automated>
  </verify>
  <done>
- worker.go 中 `injectSSHKeys` 与 `injectSSHKeysLegacy` 内部不再直接出现 `exec.CommandContext(ctx, "docker", "exec", "-i", containerName,` 字样（grep 应为空）。
- 文件内存在 `var execInContainer = func(`、`sshManagedBeginMarker`、`sshManagedEndMarker` 三个标识。
- `go build ./... &amp;&amp; go vet ./internal/runtime/tasks/...` 均通过。
- 不引入新依赖（go.mod 无变化）。
  </done>
  <acceptance_criteria>
- `rg "exec.CommandContext\(ctx, \"docker\", \"exec\", \"-i\", containerName" internal/runtime/tasks/worker.go` 无输出，或仅剩 `syncContainerCredentials` 中 chpasswd 场景（本 task 可不改 chpasswd）。
- `rg "execInContainer\(" internal/runtime/tasks/worker.go` 至少 5 处（authorized_keys 1 + outbound 私钥 1 + outbound 公钥 1 + legacy 私钥 1 + legacy 公钥 1）。
- `rg "sshManagedBeginMarker|sshManagedEndMarker" internal/runtime/tasks/worker.go` 各至少 1 处（声明）。
- 该 task 下没有事件类型名变动：`rg "runtime\.ssh_" internal/runtime/tasks/worker.go` 行数与改动前一致。
  </acceptance_criteria>
</task>

<task type="auto">
  <name>Task 2: 实现幂等写入 + authorized_keys marker 合并 + skipped 事件</name>
  <files>internal/runtime/tasks/worker.go</files>
  <read_first>
    - Task 1 改完之后的 worker.go（`injectSSHKeys` 428-515、`injectSSHKeysLegacy` 518-565、package helpers 597-640）
    - `repository.RecordEventParams` 的使用惯例（参考同文件 448-454 / 488-493 已有事件写入）
  </read_first>
  <action>
在 Task 1 的铺路基础上加入真正的幂等 + 合并行为：

**（2.1）新增内部 helper `containerFileNonEmpty` 与 `containerReadFile`：**
```go
// containerFileNonEmpty：docker exec test -s path；exit 0 → true。任何异常 → false（后续按"需要写"处理）。
func containerFileNonEmpty(ctx context.Context, container, path string) bool {
    // 通过 execInContainer 用 `[ -s "$P" ] && echo y || echo n` 返回单行结果。
    // stdin 用 path，脚本从环境变量读取，避免 path 内含特殊字符被 shell 解释。
    script := `P=$(cat) && [ -s "$P" ] && echo y || echo n`
    out, err := execInContainer(ctx, container, script, path)
    if err != nil { return false }
    return strings.TrimSpace(string(out)) == "y"
}

// containerReadFile：不存在 / 读失败都返回 ("", false)；存在返回完整内容与 true。
func containerReadFile(ctx context.Context, container, path string) (string, bool) {
    script := `P=$(cat) && [ -f "$P" ] && cat "$P" || exit 42`
    out, err := execInContainer(ctx, container, script, path)
    if err != nil { return "", false }
    return string(out), true
}
```
注意：`execInContainer` 只接受一个 stdin 字符串，`[ -s ]` 检查这里用 `P=$(cat)` 从 stdin 读 path 的技巧，避免脚本字符串里对 `path` 做 shell 拼接。

**（2.2）修复 outbound 私钥/公钥（`injectSSHKeys` 内部循环，~456-515）：**
- 在调用写入脚本之前，先 `if containerFileNonEmpty(ctx, containerName, keyFile) { … skip + 事件 … }`。同样对 `pubFile` 单独判断一次。
- 跳过时调用 `w.repo.RecordEvent(ctx, repository.RecordEventParams{ HostID: &request.HostID, Level: "info", Type: "runtime.ssh_key_skipped_existing", Message: "outbound key file already present, skip overwrite", Metadata: map[string]any{"host_id": request.HostID, "file": keyFile /* 或 pubFile */} })`。
- **即使跳过写入，也应当修正权限/属主**：额外调用一次 `execInContainer(ctx, containerName, fmt.Sprintf("chown %s:%s %s && chmod 600 %s", user, user, keyFile, keyFile), "")`（私钥；公钥改为 `chmod 644`）。`chown` 失败不 fatal，但应记录一条 `runtime.ssh_key_chown_failed` warn 事件（同样带 host_id + file）。
- 没跳过时（文件不存在/为空）走旧写入路径，保持 proxy 首次自举可用。

**（2.3）重写 `authorized_keys` 写入为 marker 合并：**
- 先计算控制面权威条目 `authorizedKeys`（proxy pub + purpose==inbound 的 DB pub），保持现有剔空与 TrimSpace 逻辑。
- 读取容器现有文件：`existing, ok := containerReadFile(ctx, containerName, sshDir+"/authorized_keys")`。
- 抽出纯函数 `mergeAuthorizedKeys(existing string, managed []string) string`，职责：
  - 若 `existing` 为空或 `ok == false`：
    - 若 `managed` 为空 → 返回空串，调用方应**完全跳过写入**（避免创建仅含 marker 的文件）。
    - 否则返回 `BEGIN\n<managed joined>\nEND\n`。
  - 若 `existing` 非空：
    - 用 `strings.Split(existing, "\n")` + 扫描找到一对 marker 行（第一个 begin 与之后第一个 end）。
    - 若找到：把 marker 之间的行（含 marker）替换为新的 marker 块（managed 为空时替换为零行，即整段删掉，保留 marker 之外其余行原样）。
    - 若未找到：在文件末尾追加一块新的 marker 块（仍保留原有行）。
    - 保证结果以 `\n` 结尾，不出现空行漂移（不要主动 TrimSpace 整体，只 Trim 尾部多余的 `\n` 再补一个）。
    - managed 为空 + existing 非空 + 没找到 marker → 直接返回 existing（本次不动文件，且**不要写回**，由调用方判断 `merged == existing` 时 skip）。
- 调用方：
  - 若最终 `merged == ""` → 完全 skip 写入（别创建空文件）。
  - 若最终 `merged == existing` → 幂等、也 skip 写入（可选记录 debug；为了测试观察性，仍然 run 一次 `chown`/`chmod` 修正属主，但不重写内容）。
  - 否则 `execInContainer(ctx, containerName, "mkdir -p ... && cat > .../authorized_keys && chmod 600 ... && chown ...", merged)` 覆盖写入。失败沿用既有 `runtime.ssh_authorized_keys_failed` warn 事件。

**（2.4）`injectSSHKeysLegacy` 同样加入"已存在则跳过"判断：**
- 对 `keyFile`（id_ed25519 或 id_rsa）与 `pubKeyFile` 各做一次 `containerFileNonEmpty` 检查；
- 跳过时写 `runtime.ssh_key_skipped_existing` 事件，metadata 带 file 名；
- 跳过时同样尝试修正属主/权限；
- 不跳过时走原逻辑。

**（2.5）空 proxy 公钥保护（对齐 findings 的 D 项）：**
- `loadProxyPublicKey()` 返回空串时不要拼成一行 `""` 进入 `authorizedKeys` 切片（当前实现已经判空，保留此判断即可；如 Task 1 改动后丢失了判空，补回来）。

**约束**：
- 不改任何函数对外签名；不新增 public API。
- 所有用户可见的事件 Message 使用中文即可，Type 保持英文下划线格式。
- commit message：`fix(runtime): injectSSHKeys 幂等且合并 authorized_keys，保留用户手加密钥`
  </action>
  <verify>
    <automated>go build ./... &amp;&amp; go vet ./internal/runtime/tasks/...</automated>
  </verify>
  <done>
- `mergeAuthorizedKeys` 作为可单测的纯函数存在于 worker.go（不需要导出，但签名可被同包测试调用）。
- `injectSSHKeys` / `injectSSHKeysLegacy` 在目标 id_* 文件非空时不会再向容器写入其内容；会触发 `runtime.ssh_key_skipped_existing` 事件。
- authorized_keys 的写入路径 100% 经过 `mergeAuthorizedKeys`；`managed=空 + existing=空` 时不创建文件。
- `go build ./... &amp;&amp; go vet ./internal/runtime/tasks/...` 通过。
  </done>
  <acceptance_criteria>
- `rg "runtime.ssh_key_skipped_existing" internal/runtime/tasks/worker.go` ≥ 2 处（outbound 私钥分支 + legacy 分支，至少）。
- `rg "mergeAuthorizedKeys\(" internal/runtime/tasks/worker.go` ≥ 2 处（1 定义 + 1 调用）。
- `rg "sshManagedBeginMarker" internal/runtime/tasks/worker.go` 出现在 `mergeAuthorizedKeys` 实现中。
- `rg "containerFileNonEmpty\(" internal/runtime/tasks/worker.go` ≥ 3 处（outbound key + outbound pub + legacy 分支；实际 ≥ 3）。
- `rg "SSHKeys\s*==\s*0|len\(request.SSHKeys\) == 0" internal/runtime/tasks/worker.go` 保留早退路径不变。
  </acceptance_criteria>
</task>

<task type="auto" tdd="true">
  <name>Task 3: 新增 ssh_inject_test.go 覆盖 5 个幂等 case</name>
  <files>internal/runtime/tasks/ssh_inject_test.go</files>
  <read_first>
    - Task 2 结束后的 worker.go（尤其 execInContainer、mergeAuthorizedKeys、containerFileNonEmpty、containerReadFile、marker 常量）
    - internal/runtime/tasks/ssh_handoff_test.go（单测风格、断言工具）
    - internal/runtime/tasks/ssh_ready_test.go（如果已有"package-level var 替换"的先例，参照其做法）
    - internal/agentapi/contracts.go（SSHKeyEntry 字段、HostActionRequest.SSHKeys）
  </read_first>
  <behavior>
覆盖 5 个最小 case，全部通过 fake `execInContainer` 跑，零 docker 依赖。fake 维护一张 `map[string]string` 模拟容器内 `/workspace/.ssh/*` 文件；脚本解析允许"足够覆盖本测试"的最小 shell 语义（见 action 说明）。

1) **empty_container_writes_outbound**: 容器内 `id_ed25519` 不存在；DB SSHKeys 含一条 outbound（ed25519 私+公）。调用 `w.injectSSHKeys(ctx, req, "c1")` 后，fake 里出现 `/workspace/.ssh/id_ed25519` 与 `.pub`，内容 = DB 提供的字符串。无 `ssh_key_skipped_existing` 事件。
2) **existing_outbound_is_preserved**: 容器内已有 `id_ed25519`（内容 = "USER-GENERATED-PRIV"），DB 同样给一条 outbound 私钥（内容不同）。调用后 fake 里 `id_ed25519` **保持为 "USER-GENERATED-PRIV"**；事件表里出现至少一条 `runtime.ssh_key_skipped_existing`，metadata.file == `/workspace/.ssh/id_ed25519`。
3) **authorized_keys_fresh_write**: 容器内 `authorized_keys` 不存在；DB 给 1 条 inbound pub，proxy pub 也非空。调用后 fake 里 `authorized_keys` 存在，内容按顺序包含 `sshManagedBeginMarker`、proxy pub 一行、inbound pub 一行、`sshManagedEndMarker`。
4) **authorized_keys_preserves_user_lines**: 容器内 `authorized_keys` 已有三行：`ssh-ed25519 USERLINE1`、`sshManagedBeginMarker`、`ssh-ed25519 OLDMANAGED`、`sshManagedEndMarker`、`ssh-ed25519 USERLINE2`。DB inbound 换成新 pub `NEWMANAGED`。调用后结果中仍然能看到 `USERLINE1` 与 `USERLINE2`（位置不变/相对顺序不变），且 marker 块中只含 proxy pub + `NEWMANAGED`，不含 `OLDMANAGED`。
5) **stable_on_second_call**: 在 case 4 的基础上保持 DB inbound 不变，再调用一次 `injectSSHKeys`。第二次调用后 fake 内 `authorized_keys` 字节级等于第一次调用结束时的内容（用 `assert equal string` 或 `bytes.Equal`）；并且第二次调用不产生"authorized_keys 写入失败"事件。
  </behavior>
  <action>
1. 新建 `internal/runtime/tasks/ssh_inject_test.go`，与 worker.go 同 package `tasks`（同包测试，可访问 package-level 变量）。
2. 在 test 里定义 `fakeContainer` 结构：
   ```go
   type fakeContainer struct {
       files map[string]string // path -> content
       log   []string          // 记录脚本，便于调试
   }
   ```
   并实现一个能替换 `execInContainer` 的闭包 `fc.runner(ctx, container, script, stdin) ([]byte, error)`。

3. 闭包内的脚本解析**只需要识别本实现真正用到的几种形态**（不做通用 bash 解释器）：
   - `P=$(cat) && [ -s "$P" ] && echo y || echo n`：path = stdin，`files[path]` 非空字符串 → "y\n"；否则 "n\n"。
   - `P=$(cat) && [ -f "$P" ] && cat "$P" || exit 42`：path = stdin，存在 → 返回内容 + nil；不存在 → 返回空 + `&exec.ExitError{}` 风格 error（实际上返回一个普通 `errors.New("exit 42")` 即可，调用方只看 err != nil）。
   - `mkdir -p .../ && cat > <file> && chmod ... && chown ...`：从脚本中用简单正则 `cat > (\S+)` 抓出 file 路径，把 stdin 写入 `files[file]`，返回 ok。
   - `chown ... && chmod ...`（不带 `cat >`）：no-op 成功。
   其他脚本：直接返回 ok（本测试不需要覆盖）。

4. 把 fake 赋给 package-level `execInContainer`（测试结束 `t.Cleanup` 恢复原值）：
   ```go
   prev := execInContainer
   execInContainer = fc.runner
   t.Cleanup(func() { execInContainer = prev })
   ```

5. 构造一个最小 fake repo 满足 `WorkerRepo` 接口：只实现 `RecordEvent`（把 `RecordEventParams` 追加到 `events []repository.RecordEventParams`）；其余方法返回零值/空 error 即可。让 test 可以断言 `events` 中存在 `Type == "runtime.ssh_key_skipped_existing"` 且 `Metadata["file"]` 等于期望路径。

6. 在每个 case 中构造 `agentapi.HostActionRequest`：`HostID: "h1"`、`SSHKeys: []agentapi.SSHKeyEntry{…}`；不需要真实 network.Provider。`Worker` 只要 `repo` 字段即可，`provider` 留 nil（injectSSHKeys 路径不触发 provider）。

7. proxy 公钥：本测试避免走真实文件系统；可以在 test 里用 `t.Setenv("DATA_DIR", t.TempDir())` 并在临时目录写入一个固定内容的 `ssh_host_ed25519_key.pub`，或者另起一个本地 helper（如果 Task 2 决定让 `loadProxyPublicKey` 可替换，也可以走 package-level var 注入 —— **只有在 Task 2 已经这么做**时才走这条路，不要为测试回改生产代码）。默认建议走 `t.Setenv("DATA_DIR", tmp)` + 写文件。

8. 测试组织：5 个子测试 `t.Run("empty_container_writes_outbound", ...)` 等。每个 case 独立构造自己的 fake 和 request，避免状态串流。

9. commit message：`test(runtime): 覆盖 injectSSHKeys 幂等与 authorized_keys 合并场景`

**约束**：
- 不启动真实 docker；不引入新依赖（只用标准库 + 项目已有 testify 或 `testing` 原生断言，看 `ssh_handoff_test.go` 现状）。
- 不测试 legacy 路径的单独行为（可选加一个 bonus 子测试，但不计入 5 个必须 case）。
- 不触碰 entrypoint.sh、前端、migration。
  </action>
  <verify>
    <automated>go test ./internal/runtime/tasks/... -run TestInjectSSHKeys -count=1</automated>
  </verify>
  <done>
- `internal/runtime/tasks/ssh_inject_test.go` 存在，包含 5 个子测试。
- `go test ./internal/runtime/tasks/... -count=1` 全绿（含既有 ssh_handoff_test.go、ssh_ready_test.go）。
- 不引入新 go.mod 依赖。
  </done>
  <acceptance_criteria>
- `rg "empty_container_writes_outbound|existing_outbound_is_preserved|authorized_keys_fresh_write|authorized_keys_preserves_user_lines|stable_on_second_call" internal/runtime/tasks/ssh_inject_test.go` 5 个字符串各至少 1 次命中。
- `rg "execInContainer\s*=" internal/runtime/tasks/ssh_inject_test.go` ≥ 1（证明做了注入替换）。
- `rg "ssh_key_skipped_existing" internal/runtime/tasks/ssh_inject_test.go` ≥ 1（证明有对事件的断言）。
- `go test ./internal/runtime/tasks/... -count=1` 退出码 0。
  </acceptance_criteria>
</task>

</tasks>

<verification>
整体验证（人工在 task 全部完成后执行一次）：

1. `go build ./... && go vet ./...` 全部通过。
2. `go test ./internal/runtime/tasks/... -count=1` 全部通过，新增用例 5 个全部绿。
3. 核对 worker.go：
   - `rg "runtime\.ssh_key_skipped_existing" internal/runtime/tasks/worker.go` ≥ 2。
   - `rg "mergeAuthorizedKeys\(" internal/runtime/tasks/worker.go` ≥ 2（定义 + 调用）。
   - `rg "execInContainer\(" internal/runtime/tasks/worker.go` ≥ 5（各写入点 + helper 内部调用）。
4. 手工阅读 diff 确认：entrypoint.sh、runtime_service.go、DB schema、contracts.go 均无改动。
5. 可选 smoke（若本地有 docker）：触发一次 rebuild（`preserve-home` 模式），在容器里 `ls -la /workspace/.ssh/`，确认用户手生成的 key 仍在；`cat /workspace/.ssh/authorized_keys` 看到 marker 块 + 任意手加的行。
</verification>

<success_criteria>
- 控制面在 create / start / rebuild(`preserve-home`) 后不再覆盖容器内已存在的 `/workspace/.ssh/id_ed25519` / `id_rsa` / 对应 `.pub`。
- `authorized_keys` 合并行为符合 marker 规则；用户手加行保留；控制面权威条目幂等稳定。
- 首次自举（容器里没有 id_*）依然可以把 DB 里的 outbound 密钥写入容器（不影响既有工作流）。
- 重复调用 `injectSSHKeys` 不产生额外 diff；跳过覆盖时能从事件流看到 `runtime.ssh_key_skipped_existing`。
- 新增单测覆盖 5 个 case，`go test ./internal/runtime/tasks/...` 全绿。
- 不改 entrypoint.sh、DB schema、SSHKeyEntry、RebuildMode 语义、前端与 API 边界。
</success_criteria>

<output>
完成后创建 `.planning/quick/260416-wvu-make-injectsshkeys-idempotent-so-user-ge/260416-wvu-SUMMARY.md`，内容包含：修改文件清单、关键决策（marker 方案 + "已存在则跳过"策略 + `execInContainer` 可注入）、测试结果、下一步观察项（真实 docker 环境下做一次 smoke）。
</output>
