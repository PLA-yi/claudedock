# Phase 49: 防泄漏对抗测试 - Context

**Gathered:** 2026-05-14
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous, auto_rest accept-all)

<domain>
## Phase Boundary

把 8 条防泄漏不变量（DNS 明文 / DoT / ICMP / IPv6 / IMDS / raw socket / link-local / capability）固化为可重复跑的 e2e 用例，对抗常见旁路绕过手段。

**8 plan 对应 8 条 LEAK 不变量**：

- LEAK-01 DNS 明文 UDP/53 旁路（`dig @8.8.8.8` + host eth0 抓包断言）
- LEAK-02 DoT (853) 旁路（`kdig +tls @1.1.1.1` + host eth0 抓包断言）
- LEAK-03 ICMP 阻断（`ping 8.8.8.8` 必须失败）
- LEAK-04 IPv6 阻断（`curl -6 ipv6.google.com` 必须失败 + `disable_ipv6=1` 双保险）
- LEAK-05 IMDS 阻断（`169.254.169.254` + `169.254.170.2` 必须失败）
- LEAK-06 raw socket 拒绝（`SOCK_RAW` 必须 PermissionError）
- LEAK-07 link-local 显式 drop（nftables 规则覆盖 `169.254.0.0/16`）
- LEAK-08 capability 审计（worker CapEff/CapBnd 不含 NET_RAW/NET_ADMIN/SYS_ADMIN）

**关键设计**：所有 LEAK-* 用例通过 **host eth0 抓包 / `nft list ruleset` / `getpcaps`** 等内核/宿主机视角的独立 oracle 做断言，**不依赖容器自身报告**。

**不在本 phase 范围**：

- Kill-switch 压力测试（Phase 50）
- verify.go 多目标参数化 / `--cap-drop` 启动参数源码改造（Phase 51 QUAL-02 / QUAL-06）
- 完整 artifact 采集脚本（Phase 52）

**macOS 本地执行约束**：`tests/e2e/` `//go:build e2e && linux` 隔离。

</domain>

<decisions>
## Implementation Decisions

### Area 1: 通用 oracle 与 helpers 复用

- **host eth0 抓包**：复用 Phase 48 `TcpdumpOnHostEth0` + `netshoot` privileged sidecar 路径（`E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP` 已就绪）。
- **nft 规则查询**：在 host 上 `nft list ruleset`，输出经 `ParseNftCounters` 纯函数解析为 `map[ruleName]counter`，断言关注 counter 是否 >0 命中。
- **capability 查询**：`docker exec <container> getpcaps 1` 或 `cat /proc/1/status | grep Cap`；纯函数 `ParseProcCapabilities` 解析为 `Set[Capability]`。
- **DNS / ICMP / IPv6 / IMDS / raw socket 探测**：扩 `GoldenPath` 新方法（每条 LEAK 一个），返回结构化 `LeakProbeResult{Blocked bool, Reason string, RawOutput string}`。

### Area 2: 用例组织与套件结构

- **目录拆**：新增 `tests/e2e/leak/` 子目录（`//go:build e2e && linux`），8 个 `leak_NN_<name>_test.go` 文件；每个文件单一 `TestLeak_NN_<Name>`。
- **共享 fixture**：8 个用例可串行复用同一 GoldenPath（无破坏性操作，不会污染下用例），用 `tests/e2e/leak/suite_test.go` 起一次 fixture，分摊冷启动开销 < 5 min 整组。
- **失败 artifact**：每个用例失败时通过 DumpHook 自动收集 tcpdump pcap + nft ruleset + getpcaps + dig/curl raw stderr。
- **纯函数单测分布**：`tests/e2e/helpers_test.go` 继续收纳新增纯函数（`ParseNftCounters`、`ParseProcCapabilities`、`ClassifyLeakProbe` 等）。

### Area 3: 各 LEAK 具体断言

- **LEAK-01 DNS 明文**：容器内 `dig +short +time=3 @8.8.8.8 example.com`；host eth0 `tcpdump -i eth0 -nn -c 5 udp port 53 and dst 8.8.8.8 and src <workerIP>` 必须 0 包；`dig` 输出预期 timeout/SERVFAIL，不允许返回正常 A 记录（区分 Phase 46 MVS-03 「OR 语义」—— 这里要求**纯防泄漏**，不接受「走 tun 接管返回正常 A 记录」语义）。
- **LEAK-02 DoT 853**：容器内 `kdig +tls @1.1.1.1 example.com` 或 `openssl s_client -connect 1.1.1.1:853`；host eth0 `tcpdump tcp port 853 and dst 1.1.1.1 and src <workerIP>` 必须 0 包；容器内连接预期 connection refused / timeout。
- **LEAK-03 ICMP**：容器内 `ping -c 1 -W 3 8.8.8.8`；exit code 非 0 + stderr/stdout 表明被 drop；host eth0 `tcpdump icmp and dst 8.8.8.8 and src <workerIP>` 必须 0 包。
- **LEAK-04 IPv6**：容器内 `curl -6 --max-time 3 https://ipv6.google.com` 必须失败；同时验证 `/proc/sys/net/ipv6/conf/all/disable_ipv6 = 1`（双保险）；host eth0 `tcpdump ip6 and src <worker IPv6>` 必须 0 包（若 worker 无 IPv6 地址则跳过抓包断言）。
- **LEAK-05 IMDS**：容器内 `curl --max-time 3 http://169.254.169.254/latest/meta-data/` 必须失败（HTTP 4xx/5xx 或 connect timeout）；`curl --max-time 3 http://169.254.170.2/v2/credentials/x` 同样必须失败；nft counter 关注 `link-local drop` 规则命中数 >0。
- **LEAK-06 raw socket**：容器内 `python3 -c 'import socket; socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_ICMP)'` 预期 `PermissionError: [Errno 1] Operation not permitted`；如果 worker 没 python，用 `bash -c 'exec 3<>/dev/raw/icmp'` 或 `cap_net_raw=... ping`。
- **LEAK-07 link-local 显式 drop**：`nft list ruleset` 输出包含针对 `169.254.0.0/16` 的 drop 规则；`ParseNftRules` 纯函数提取规则集合，断言至少一条 destination 是 `169.254.0.0/16` 的 drop。
- **LEAK-08 capability**：`docker exec <worker> cat /proc/1/status | grep -E 'CapEff|CapBnd'`，`ParseProcCapabilities` 解析；断言 `CapEff` 和 `CapBnd` 都**不**含 `CAP_NET_RAW` / `CAP_NET_ADMIN` / `CAP_SYS_ADMIN`。

### Area 4: 复用 + 验证策略

- **VERIFICATION 策略**：darwin 编译 + 新增纯函数单测 PASS = `status: passed`；Linux 真机 8 条 LEAK 用例列 `human_verification`（deferred-to-CI）；**重点**：纯函数单测必须覆盖每条 LEAK 的 fixture 输出解析（确保解析逻辑稳）。
- **Plan 粒度**：严格 8 plan / 8 用例（49-01..08），每 plan 独立 commit。
- **整组耗时**：≤ 5 分钟（CONTEXT 锁定，CI 上跑）；darwin 上只跑纯函数 < 30s。
- **每个 LEAK 用例的"独立 oracle"**：不允许只靠容器内自报；至少一个 host/kernel 层证据（tcpdump / nft / proc）。

### Claude's Discretion

- nft / getpcaps 输出的实际格式如何（执行时按真实输出对齐解析器）
- 每条 LEAK 用例的具体探测命令（按 worker 镜像内可用工具调整：有无 kdig / openssl / python3）
- LEAK-04 IPv6 worker 是否有 IPv6 地址（如果完全无 IPv6 stack，抓包断言可跳过，只做 disable_ipv6 + curl 失败断言）
- LEAK-07 nft 规则匹配的粒度（精确 CIDR vs 包含 169.254.0.0/16 子集）
- 8 个用例文件目录结构（`leak/` 子目录 vs `tests/e2e/leak_*.go`，推荐前者）

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets

- **Phase 46/47/48 已交付** `tests/e2e/helpers.go` + `helpers_linux.go`：直接复用 `GoldenPath`、`StartGoldenPath`、`Vote`、`ClassifyDNSResult`、`DefaultDenyMatrix`、`KillGateway`、`TamperResolvConf`、`TcpdumpOnHostEth0`、`ProbeOutboundFromUser`、`ProbeDNSFromUser`、`InspectContainerIPv4`。
- **`netshoot` privileged sidecar** + `E2E_TCPDUMP_IMAGE` / `E2E_ALLOW_HOST_TCPDUMP`：Phase 48 已就绪。
- **`tests/e2e/harness/dump.go`**：DumpHook 失败时自动 dump。
- **`internal/network/`** / `internal/runtime/`：nft 规则 + worker container 配置（worker 当前是否带 NET_RAW / NET_ADMIN 取决于源码现状；Phase 51 QUAL-06 会把 NET_RAW / NET_ADMIN 显式 `--cap-drop`）。

### Established Patterns

- **build tag**：`//go:build e2e && linux`；纯函数无 tag。
- **failure-only artifact**：DumpHook 已挂。
- **中文沟通**。

### Integration Points

- **新增目录 `tests/e2e/leak/`**：放 8 个用例文件 + 1 个 suite_test.go（共享 fixture）。
- **扩 `tests/e2e/helpers_linux.go`**：新增方法
  - `DigPlainDNS(ctx, server, name string) (*LeakProbeResult, error)`
  - `DigDoT(ctx, server, name string) (*LeakProbeResult, error)`
  - `PingICMP(ctx, target string) (*LeakProbeResult, error)`
  - `CurlIPv6(ctx, url string) (*LeakProbeResult, error)`
  - `ReadProcFile(ctx, path string) (string, error)` —— 通用 /proc 读
  - `CurlIMDS(ctx, url string) (*LeakProbeResult, error)`
  - `TryRawSocket(ctx) (*LeakProbeResult, error)`
  - `ListNftRulesOnHost(ctx) (string, error)`
  - `GetProcCapabilities(ctx, pid int) (string, error)`
- **扩 `tests/e2e/helpers.go`**（无 tag）：
  - `LeakProbeResult` 类型
  - `ParseNftCounters(raw string) map[string]uint64`
  - `ParseNftRules(raw string) []NftRule`
  - `ParseProcCapabilities(raw string) (capEff, capBnd Set[Capability], err error)`
  - `ClassifyLeakProbe(result *LeakProbeResult, expectBlocked bool) LeakVerdict`
- **扩 `tests/e2e/helpers_test.go`**：上述纯函数 fixture-driven 单测（用 testdata 里 nft / getpcaps / dig / curl 真实输出做 fixture）。
- **可能新增 `tests/e2e/testdata/`**：放 nft ruleset / getpcaps / dig 等输出 fixture（避免在 darwin 上跑真命令）。
- **不引入新 Go 依赖**。

</code_context>

<specifics>
## Specific Ideas

- **`LeakProbeResult` 结构**：
  ```go
  type LeakProbeResult struct {
      Blocked       bool          // 是否被防御层阻断（true = 防泄漏成功）
      Reason        string        // 阻断原因（timeout / refused / drop / permission_denied 等）
      RawStdout     string
      RawStderr     string
      ExitCode      int
      Duration      time.Duration
  }
  ```
- **`LeakVerdict` 三值枚举**：`LeakVerdictPass` / `LeakVerdictFail` / `LeakVerdictInconclusive`。
- **`NftRule` 结构**：
  ```go
  type NftRule struct {
      Table  string
      Chain  string
      Action string // drop / accept / reject / counter
      Dst    string // 169.254.0.0/16 / 1.1.1.1/32 / any
      Proto  string // tcp / udp / icmp / ip6
      Port   int    // 0 = any
  }
  ```
- **`Capability` 枚举**：`CAP_NET_RAW` / `CAP_NET_ADMIN` / `CAP_SYS_ADMIN` 等（≥5 个常用，作为锁定常量）。

</specifics>

<deferred>
## Deferred Ideas

- **Tetragon TracingPolicy 内核 oracle**：v2 范围（REQUIREMENTS 已 deferred）。
- **mDNS / LLMNR / NetBIOS / SSDP 协议旁路**：本 phase 8 条不变量覆盖 v3.5 已识别清单；新协议属 Phase 51 QUAL-* 后续。
- **Worker 镜像 capability 源码改造**：Phase 51 QUAL-06 用 `--cap-drop` 启动参数显式去除 NET_RAW/NET_ADMIN；本 phase 49 只测「当前是否已无」。
- **整组耗时基线持续验证**：本 phase 锁定 ≤5 min，但不做性能回归基线；属性能优化候选。
- **Linux runner 真机签字**：deferred-to-CI。
- **完整 artifact 采集**：Phase 52 OBS-01..03 范围。

</deferred>
