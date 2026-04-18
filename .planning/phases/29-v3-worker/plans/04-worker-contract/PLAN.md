---
phase: 29-v3-worker
plan: 04-worker-contract
sub_scope: D
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/agentapi/contracts.go
  - internal/runtime/tasks/worker.go
  - internal/runtime/tasks/worker_volume_test.go
autonomous: true
requirements:
  - D-18
  - D-19
  - D-20
  - D-21
  - D-22
must_haves:
  truths:
    - "internal/agentapi/contracts.go 新增 VolumeMount 类型（Name / Target / ReadOnly / Labels），HostActionRequest 新增 Volumes []VolumeMount `json:\"volumes,omitempty\"`"
    - "worker.go:createHost 在 -v homeDir:homeMount 之后、Labels 遍历之前，按 request.Volumes 追加 --mount type=volume,src=X,dst=Y[,readonly]"
    - "空 Volumes（nil 或 []）不新增任何 args；v2.0 旧 HostActionRequest JSON 反序列化不破"
    - "worker.go 不调用 docker volume create（Phase 33 职责）"
    - "contracts.go 不新增 ClaudeAccountID 字段（Phase 30 职责）"
    - "新增 worker_volume_test.go 覆盖：JSON round-trip omitempty + args 拼接快照 + v2.0 旧 request 兼容"
  artifacts:
    - path: "internal/agentapi/contracts.go"
      provides: "VolumeMount struct + HostActionRequest.Volumes 字段"
      contains: "type VolumeMount struct"
      exports: ["VolumeMount"]
    - path: "internal/runtime/tasks/worker.go"
      provides: "createHost 中 --mount type=volume 追加逻辑"
      contains: "type=volume,src="
    - path: "internal/runtime/tasks/worker_volume_test.go"
      provides: "TestHostActionRequest_VolumesOmitempty + TestBuildCreateArgs_VolumesMount + 兼容性测试"
      contains: "TestHostActionRequest_VolumesOmitempty"
  key_links:
    - from: "HostActionRequest.Volumes 字段"
      to: "worker.go createHost args"
      via: "for _, vm := range request.Volumes { append --mount type=volume,src=,dst= }"
      pattern: "type=volume,src=%s,dst=%s"
---

## Goal

在 v3.0 host-agent 契约层加一个最小 `VolumeMount` 结构体与 `HostActionRequest.Volumes` 字段，让 worker `createHost` 拼 `docker create` 时追加 `--mount type=volume,src=X,dst=Y[,readonly]` 参数。**本阶段不**创建 volume（Phase 33 `ensureDockerVolume` 负责），**不**引入 `ClaudeAccountID` 字段（Phase 30 负责），JSON 通过 `omitempty` + Go 默认忽略未知字段实现与 v2.0 旧 agent 的向后兼容。

对应 Sub-scope：**D agentapi & worker 契约**（29-RESEARCH.md §Sub-scope 映射）。

---

## Scope

### In
1. `internal/agentapi/contracts.go`：
   - 新增 `VolumeMount` struct（字段顺序固定 `Name / Target / ReadOnly / Labels`）
   - `HostActionRequest` 在 `SSHKeys` 字段**之后**追加 `Volumes []VolumeMount \`json:"volumes,omitempty"\``
2. `internal/runtime/tasks/worker.go:createHost`：
   - 在 `-v homeDir:homeMount`（当前 186 行）**之后**、`for key, value := range request.Labels`（当前 189 行）**之前**插入一个 `for _, vm := range request.Volumes { ... }` 循环
   - 基本校验：`vm.Name == "" || vm.Target == ""` → 返回 `fmt.Errorf("invalid volume mount: ...")`
   - **不**对 `vm.Labels` 做容器 label 注入（D-19 明确禁止；与 `request.Labels` 的遍历是两个独立语义）
   - 可选重构：抽出 `buildCreateArgs(request) []string` helper 以便测试（推荐；见 Task 4.3）
3. 新建 `internal/runtime/tasks/worker_volume_test.go`：
   - `TestHostActionRequest_VolumesOmitempty`（空 Volumes 不出现在 JSON；非空正确序列化；round-trip 不丢字段）
   - `TestHostActionRequest_V2Compat`（v2.0 旧 JSON 无 volumes 字段 → `Volumes == nil`，无报错）
   - `TestBuildCreateArgs_VolumesMount`（若抽出 helper，断言包含 `--mount type=volume,src=claude-state-abc,dst=/var/lib/claude-persist`；ReadOnly 时 `,readonly`；空 slice 不追加）

### Out
- Dockerfile / entrypoint / tmux / sshd_config / image.lock / host-preflight / CI → Plan 01/02/03/05/06
- `docker volume create` 幂等化 → Phase 33
- `ClaudeAccountID` 字段 → Phase 30
- `HostActionRequest.Volumes` 的真实消费者（Phase 33 生成 `claude-state-{account_id}` volume）→ Phase 33

---

## Dependencies

- **None**（Wave 1，可与 Plan 01 / 05 / 06 并行）
- 本 plan 全部文件与 Plan 01..03 / 05 / 06 的 `files_modified` 无重叠（Go 代码 vs Dockerfile / sh / YAML / workflow）

---

## Tasks

### Task 4.1 — contracts.go 新增 VolumeMount 类型 + HostActionRequest.Volumes 字段

**文件：** `internal/agentapi/contracts.go`

**改动要点：**
- 在现 `SSHKeyEntry` struct（当前 13-19 行）**之后**、`HostActionRequest` struct（当前 21-41 行）**之前**插入：
  ```go
  // VolumeMount 描述 docker create --mount type=volume 的最小契约。
  // Phase 29 仅支持 named volume；生命周期（create/rm）由 Phase 33 管理。
  type VolumeMount struct {
      Name     string            `json:"name"`
      Target   string            `json:"target"`
      ReadOnly bool              `json:"read_only,omitempty"`
      Labels   map[string]string `json:"labels,omitempty"`
  }
  ```
- 在 `HostActionRequest` struct 内部，`SSHKeys []SSHKeyEntry \`json:"ssh_keys,omitempty"\`` 字段（当前 40 行）**之后**追加：
  ```go
      Volumes []VolumeMount     `json:"volumes,omitempty"`
  ```
- **严禁**改动现有字段顺序 / tag / 类型（向后兼容红线）
- **严禁**新增 `ClaudeAccountID` 字段（D-21：Phase 30 职责）

**对应：** D-18（VolumeMount 字段清单） / D-22（omitempty 向后兼容）
**PATTERNS：** G1（`struct + JSON tag + omitempty` slice 字段；`SSHKeys` 是直接 analog）；AP6（不加 schema 版本字段）
**代码参考：** RESEARCH §Go 契约 diff 示意

### Task 4.2 — worker.go:createHost 插入 --mount type=volume 拼接循环

**文件：** `internal/runtime/tasks/worker.go`

**改动要点：**
- 定位当前 `-v fmt.Sprintf("%s:%s", homeDir, firstNonEmpty(request.HomeMount, defaultWorkspaceMount))` 追加行（当前 186 行的 `args = append(args, ... "-v", ...)`）**之后**、`for key, value := range request.Labels {` 循环（当前 189 行）**之前**插入：
  ```go
  for _, vm := range request.Volumes {
      if vm.Name == "" || vm.Target == "" {
          return fmt.Errorf("invalid volume mount: name=%q target=%q", vm.Name, vm.Target)
      }
      opts := fmt.Sprintf("type=volume,src=%s,dst=%s", vm.Name, vm.Target)
      if vm.ReadOnly {
          opts += ",readonly"
      }
      args = append(args, "--mount", opts)
  }
  ```
- **`,readonly` 无值标志**（不是 `,ro`；RESEARCH §Code Examples 明确），区分于现 `-v` bind mount 语法
- **不**对 `vm.Labels` 做 `--label` 注入（D-19）
- **不**调用 `docker volume create`（D-20）——volume 不存在时 docker create 会失败，错误经 `runDocker` 正常冒泡（与现 `host_action_failed` 错误码一致）

**对应：** D-19（拼接语法） / D-20（不创建 volume）
**PATTERNS：** G2（docker create args 数组 + append 遍历模式）；AP1（`-v` 旧语法不替换，仅 Volumes 走新语法）；AP6（不加 schema 版本）
**代码参考：** RESEARCH §Go 契约 diff 示意 §worker.go 段

### Task 4.3 — 抽出 buildCreateArgs 辅助函数（可选但推荐）

**文件：** `internal/runtime/tasks/worker.go`

**改动要点（推荐执行）：**
- 把 `createHost` 中 "args := []string{...}" 到 "args = append(args, request.ImageName)" 这段纯拼接逻辑抽成 `buildCreateArgs(request agentapi.HostActionRequest, containerName, hostname string) ([]string, error)`（返回 slice + 参数校验错误）
- `createHost` 改为 `args, err := w.buildCreateArgs(request, containerName, hostname); if err != nil { return err }`
- 抽出后测试（Task 4.4）可直接断言 `buildCreateArgs` 的输出，无需 fake docker / exec

**如不执行 Task 4.3（保守做法）：**
- Task 4.4 只能测 contracts.go JSON round-trip，跳过 args 拼接快照断言；Verification 的 V-02 落到"人工 + CI 构建时偶然 assertions"弱验证

**对应：** RESEARCH §Assumptions A6（ssh_inject_test.go 风格复用）；PATTERNS D 子项 D6 "若 worker.go 的 createHost 整体难以独立测试，可抽出 buildCreateArgs helper 独立测"

### Task 4.4 — 新建 worker_volume_test.go 单元测试

**文件：** `internal/runtime/tasks/worker_volume_test.go`（新建）

**改动要点：**

```go
package tasks

import (
    "encoding/json"
    "strings"
    "testing"

    "github.com/zanel1u/cloud-cli-proxy/internal/agentapi"
)

// V-01 + D-22：空 Volumes 不出现在 JSON（omitempty 行为）
func TestHostActionRequest_VolumesOmitempty(t *testing.T) {
    t.Run("empty_volumes_not_serialized", func(t *testing.T) {
        req := agentapi.HostActionRequest{TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost}
        buf, err := json.Marshal(req)
        if err != nil {
            t.Fatalf("marshal: %v", err)
        }
        if strings.Contains(string(buf), `"volumes"`) {
            t.Fatalf("empty Volumes must be omitempty, got: %s", buf)
        }
    })

    t.Run("non_empty_volumes_serialized", func(t *testing.T) {
        req := agentapi.HostActionRequest{
            TaskID: "t1", HostID: "h1", Action: agentapi.ActionCreateHost,
            Volumes: []agentapi.VolumeMount{
                {Name: "claude-state-abc", Target: "/var/lib/claude-persist"},
            },
        }
        buf, err := json.Marshal(req)
        if err != nil {
            t.Fatalf("marshal: %v", err)
        }
        if !strings.Contains(string(buf), `"volumes"`) {
            t.Fatalf("non-empty Volumes must serialize, got: %s", buf)
        }

        var parsed agentapi.HostActionRequest
        if err := json.Unmarshal(buf, &parsed); err != nil {
            t.Fatalf("round-trip unmarshal: %v", err)
        }
        if len(parsed.Volumes) != 1 || parsed.Volumes[0].Name != "claude-state-abc" {
            t.Fatalf("round-trip lost data: %+v", parsed)
        }
    })

    t.Run("readonly_omitempty_behavior", func(t *testing.T) {
        rw := agentapi.VolumeMount{Name: "v1", Target: "/mnt/a"}
        ro := agentapi.VolumeMount{Name: "v2", Target: "/mnt/b", ReadOnly: true}

        bufRW, _ := json.Marshal(rw)
        bufRO, _ := json.Marshal(ro)

        if strings.Contains(string(bufRW), `"read_only"`) {
            t.Fatalf("ReadOnly=false must be omitempty, got: %s", bufRW)
        }
        if !strings.Contains(string(bufRO), `"read_only":true`) {
            t.Fatalf("ReadOnly=true must serialize, got: %s", bufRO)
        }
    })
}

// V-06：v2.0 旧 client 发送无 volumes 字段的 JSON → 不破
func TestHostActionRequest_V2Compat(t *testing.T) {
    oldJSON := `{"task_id":"t","host_id":"h","action":"create_host","image_name":"img","default_user":"workspace","home_mount":"/workspace","rebuild_mode":"","container_name":"c","home_dir":"/d","labels":null,"timezone":"","hostname":""}`
    var req agentapi.HostActionRequest
    if err := json.Unmarshal([]byte(oldJSON), &req); err != nil {
        t.Fatalf("v2.0 JSON must unmarshal cleanly: %v", err)
    }
    if req.Volumes != nil {
        t.Fatalf("Volumes must be nil when absent in JSON, got: %+v", req.Volumes)
    }
}

// V-02（仅在 Task 4.3 抽出 buildCreateArgs 后可测）：
// docker create args 正确拼接 --mount type=volume,src=X,dst=Y[,readonly]
// 若 Task 4.3 未执行，此测试块可用 t.Skip("requires buildCreateArgs helper from Task 4.3") 占位
func TestBuildCreateArgs_VolumesMount(t *testing.T) {
    // 假设 Task 4.3 已抽出：
    //   func (w *Worker) buildCreateArgs(req agentapi.HostActionRequest, containerName, hostname string) ([]string, error)
    // 若未抽出，改 t.Skip 并在 Task 4.3 决议后回来补
    t.Skip("enable after Task 4.3 extracts buildCreateArgs helper")

    // Example skeleton once helper exists:
    // w := &Worker{}
    // req := agentapi.HostActionRequest{
    //     TaskID: "t", HostID: "h", Action: agentapi.ActionCreateHost,
    //     Volumes: []agentapi.VolumeMount{
    //         {Name: "claude-state-abc", Target: "/var/lib/claude-persist"},
    //         {Name: "ro-cache", Target: "/mnt/ro", ReadOnly: true},
    //     },
    // }
    // args, err := w.buildCreateArgs(req, "c1", "c1")
    // if err != nil { t.Fatalf("buildCreateArgs: %v", err) }
    // if !argsContainPair(args, "--mount", "type=volume,src=claude-state-abc,dst=/var/lib/claude-persist") {
    //     t.Fatalf("missing rw volume mount, args=%v", args)
    // }
    // if !argsContainPair(args, "--mount", "type=volume,src=ro-cache,dst=/mnt/ro,readonly") {
    //     t.Fatalf("missing ro volume mount, args=%v", args)
    // }
}

// 空 Volumes slice 不 append 任何 args（防御性断言；仅在 Task 4.3 后可测）
func TestBuildCreateArgs_EmptyVolumes_NoExtraArgs(t *testing.T) {
    t.Skip("enable after Task 4.3 extracts buildCreateArgs helper")
    // Example:
    // w := &Worker{}
    // reqEmpty := agentapi.HostActionRequest{TaskID: "t", HostID: "h", Action: agentapi.ActionCreateHost}
    // reqNil := reqEmpty
    // // Volumes: nil
    // argsEmpty, _ := w.buildCreateArgs(reqEmpty, "c1", "c1")
    // if argsContain(argsEmpty, "--mount") && strings.Contains(strings.Join(argsEmpty, " "), "type=volume") {
    //     t.Fatalf("nil Volumes must not add --mount type=volume args")
    // }
}
```

**对应：** V-01 / V-02 / V-06（RESEARCH §Validation Architecture）
**PATTERNS：** G3 / G5（`t.Run("case_name", ...)` 风格）；G1（omitempty 断言直对 SSHKeys 先例）
**注意：** 本 plan 不引入 `execInContainer` 注入（本 feature 是纯拼接 + JSON round-trip，无容器 IO 调用；PATTERNS D 子项 D4 明确）

---

## Verification

### 静态断言

```bash
# contracts.go
grep -E 'type VolumeMount struct \{'                       internal/agentapi/contracts.go
grep -E 'Name +string +`json:"name"`'                      internal/agentapi/contracts.go
grep -E 'Target +string +`json:"target"`'                  internal/agentapi/contracts.go
grep -E 'ReadOnly +bool +`json:"read_only,omitempty"`'     internal/agentapi/contracts.go
grep -E 'Labels +map\[string\]string +`json:"labels,omitempty"`' internal/agentapi/contracts.go
grep -F 'Volumes       []VolumeMount' internal/agentapi/contracts.go || \
  grep -F 'Volumes []VolumeMount `json:"volumes,omitempty"`' internal/agentapi/contracts.go
# AP6：禁新增 schema 版本字段
! grep -F 'VolumesVersion' internal/agentapi/contracts.go
# D-21：禁 ClaudeAccountID 出现在本 plan（Phase 30 职责）
! grep -F 'ClaudeAccountID' internal/agentapi/contracts.go

# worker.go
grep -F 'for _, vm := range request.Volumes' internal/runtime/tasks/worker.go
grep -F 'type=volume,src=%s,dst=%s'          internal/runtime/tasks/worker.go
grep -F '",readonly"'                         internal/runtime/tasks/worker.go
# AP1 反向：不修改 -v 旧语法（homeDir bind mount 行保持不动）
grep -F '"-v", fmt.Sprintf("%s:%s", homeDir,' internal/runtime/tasks/worker.go
# D-20：本 plan 不调 docker volume create
! grep -F 'docker volume create' internal/runtime/tasks/worker.go

# 新测试文件存在
test -f internal/runtime/tasks/worker_volume_test.go
```

### 动态断言（Go）

```bash
cd $(git rev-parse --show-toplevel)

# 编译 + vet
go vet ./internal/agentapi/... ./internal/runtime/tasks/...

# 单元测试（本 plan 新增）
go test ./internal/agentapi/...           -run '' -count=1
go test ./internal/runtime/tasks/...      -run 'TestHostActionRequest_VolumesOmitempty|TestHostActionRequest_V2Compat|TestBuildCreateArgs_VolumesMount|TestBuildCreateArgs_EmptyVolumes_NoExtraArgs' -count=1 -v

# 现有测试不破（回归）
go test ./internal/runtime/tasks/... -run TestInjectSSHKeys -count=1

# 整体套件保险
go test ./... -count=1
```

### Coverage contribution

> **Coverage contribution:** V-01（Volumes omitempty） / V-02（createHost 拼接正确） / V-06（v2.0 旧 JSON 兼容） → 本 plan 负责 contracts + worker + 单测全链路的静态正确性；端到端的 runtime 断言（实际 docker create）要等 Phase 33 调用 ensureDockerVolume 后才能闭环。
>
> **Pitfall coverage:** 无 Critical Pitfall 直接归属本 plan；AP1 / AP6（PATTERNS 反模式）由本 plan 通过"不改 -v 旧语法 + 不加 schema 版本"两条消极约束间接承担。

---

## Atomic Commit Strategy

3 个原子 commit：

1. `feat(29-04): agentapi add VolumeMount type and HostActionRequest.Volumes field`
   - Task 4.1（contracts.go 改动）
2. `feat(29-04): worker createHost append --mount type=volume for request.Volumes`
   - Task 4.2（worker.go 改动）+ Task 4.3（可选抽出 buildCreateArgs）
   - 如 Task 4.3 执行，commit message 可加副标题 `refactor(29-04): extract buildCreateArgs helper`
3. `test(29-04): worker volume JSON round-trip and args assembly tests`
   - Task 4.4（新测试文件）

---

## Pitfalls 防御

本 plan 无 Critical Pitfall 直接归属，但承担以下反模式约束：

| Anti-pattern | 防御手段 | 本 plan 对应任务 |
|--------------|---------|-----------------|
| **AP1** `-v` 旧语法替换 | 只对新增 Volumes 用 `--mount type=volume`；`-v` bind mount 保留原地不动 | Task 4.2 |
| **AP6** 为 Volumes 加 schema 版本字段 | 依赖 `omitempty` + Go encoding/json 默认忽略未知字段；不加 `VolumesVersion` 等 | Task 4.1（消极约束） |
| **D-20 越界** Phase 33 `docker volume create` | worker 不调 `volume create`；volume 不存在时 `docker create` 正常失败，错误码沿用 | Task 4.2 |
| **D-21 越界** Phase 30 `ClaudeAccountID` | contracts.go 不新增该字段 | Task 4.1（消极约束） |

---

## Risks / Unknowns

1. **`buildCreateArgs` helper 抽出的 blast radius**
   - 当前 `createHost`（`internal/runtime/tasks/worker.go` 大约 120-220 行）包含：args 拼接 + `runDocker` 调用 + 网络配置 + `waitForSSH`；只有 args 拼接是纯函数
   - 若 executor 担心抽出过多逻辑破坏可读性，Task 4.3 可保守降级为只抽出 `volumeMountArgs(request.Volumes) []string`（15 行），Task 4.4 的 BuildCreateArgs 快照测试简化为 `TestVolumeMountArgs`
   - Fallback：保守方案下仍能覆盖 V-02（args 拼接正确性）；全量抽出只在需要快照测试 memory/cpu/env 时才有收益

2. **`map[string]string` 的 JSON 序列化顺序**
   - `Labels map[string]string` 在不同 Go 版本 JSON 序列化顺序可能不同，若 Task 4.4 的某些断言依赖字段顺序会 flaky
   - Fallback：断言用 `strings.Contains` 而非整串等值；不依赖 map key 顺序

3. **`runDocker` 对超长 args 的处理**
   - 若 `request.Volumes` 极多（100+），`args` slice 会超长；Linux `execve` 有参数长度上限（一般 128KB）
   - 本阶段 Phase 33 一个 account 只用一个 volume，1000 个 account 也只产生 1 个 `--mount`，远小于上限
   - 本 plan 不处理；若 Phase 33 设计改变，回流加校验

4. **Test 2.0 旧 JSON 字段清单**
   - Task 4.4 `TestHostActionRequest_V2Compat` 的 `oldJSON` 常量是手工编的，若 v2.0 实际发出的 JSON 有更多字段（如 `ssh_keys`），也应能正常 unmarshal
   - Fallback：executor 实测时用 `git log` 找 v2.0 最后一个 HostActionRequest JSON sample，替换 `oldJSON` 保真度

---

*End of Plan 04-worker-contract*
