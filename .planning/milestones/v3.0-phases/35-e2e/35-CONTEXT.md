# Phase 35: E2E 稳定化 + 性能验收 - Context

**Gathered:** 2026-04-22
**Status:** Ready for planning

<domain>
## Phase Boundary

在真实环境跑通 v3.0 全栈并锁定性能基线。本阶段是验收 phase，不新增功能代码，只交付基准测试、UAT 验证、CI gate、运维手册和验收清单。所有功能代码应已在前序 Phase 29-34 完成。

验收范围覆盖：
- BASE-01（元数据响应 1.5× 基线）
- BASE-02（首连 ≤ 8s）
- BASE-03（弱网容忍）
- BASE-04（镜像体积 ≤ 700MB — 二次回归）
- 30 条 functional REQ 逐项验证
- 5 章运维手册更新

</domain>

<decisions>
## Implementation Decisions

### 性能基准测试方法

- **10k 文件树**：使用 synthetic 脚本生成（`scripts/gen-bench-tree.sh`），保证可重复性。文件结构模拟 mono-repo：80% 小文件（< 4KB 源码）、15% 中等文件（< 1MB 配置/文档）、5% 大文件（< 10MB 二进制），总大小控制在 ~200MB。
- **对比基线包含 3 档**：
  1. 本地文件系统（宿主机 ext4 或 APFS）——绝对基线
  2. mergerfs full 模式（Mutagen + sshfs + mergerfs 三层全开）
  3. sshfs-only 降级模式（验证降级后的性能下限）
- **统计方式**：每种配置 warm-up 1 次 + 测量 10 次，取 P50 和 P99。报告输出 JSON + 人类可读表格。
- **CI 自动化**：在 `.github/workflows/ci.yml` 新增 `perf-benchmark` job，跑 ubuntu-latest 上的 synthetic 基准。macOS APFS 真机基准不在 CI 中跑，作为本地/真机验收项。
- **基准命令**：`rg .`（全量文本搜索）和 `ls -R /workspace`（元数据遍历），两者分别对应 CPU 密集和 metadata 密集场景。

### 弱网 UAT 执行方式

- **拔网手段**：脚本化 `tc qdisc add dev <iface> root netem loss 100%`（精确可控），恢复时 `tc qdisc del`。备选 `iptables -I OUTPUT -d <host_ip> -j DROP`。
- **判定标准（量化）**：
  - **10s 拔网**：claudedock 进程不退出；tmux 内 claude 进程 `ps` 仍在；本地 input_buffer 键入内容不丢
  - **30s 拔网**：同上 + 恢复网络后 60s 内自动重连成功；`tmux capture-pane` 与拔网前 buffer 一致
  - **2min 拔网**：claudedock 最终进入"重连失败提示"状态（REQ-F3-C）；tmux 内进程仍存活；恢复网络后手动按 Enter 可重新连接
- **"无感知"量化指标**：
  - 进程存活：`docker exec <ctr> pgrep -f claude` 在拔网全程返回 0
  - Buffer 完整性：拔网前 `tmux capture-pane` 与恢复后对比，字符级一致
  - 输入回放：本地脚本向 stdin 注入固定字符串，重连后远端 `cat` 输出与注入一致
- **执行方式**：脚本驱动（`scripts/uat-network-resilience.sh`），关键场景（30s/2min）需人工在报告中签字确认观察结果。

### 真机环境矩阵

- **macOS APFS**：使用开发者本地 M 系列 Mac 执行。GitHub Actions macos runner 为虚拟化环境，FUSE 性能数据不具参考性，故不作为 CI 基准平台。
  - 必测场景：case-insensitive 双向同步（创建 `Foo.txt` + `foo.txt` 冲突文件，断言 Mutagen `--mode=two-way-resolved` 无数据丢失）
- **Ubuntu 25.04**：CI 中使用 `ubuntu-latest`（目前 24.04）跑 AppArmor 模拟检测 + docker 三路 FUSE 挂载验证。若需严格 25.04 内核行为验证，在真机或云主机上补跑。
  - 必测场景：AppArmor `local override` 部署后 `verify-fuse-compat.sh` 全通过；sshfs + mutagen-agent + mergerfs 三路并发 mountpoint 全部就绪
- **自动化程度**：脚本化 80% + 人工签字 20%。脚本自动生成测试报告（JSON + markdown），人工在关键场景（APFS 冲突、2min 拔网、AppArmor 真机）报告中确认并签字。

### 运维手册与验收清单形式

- **手册位置**：`docs/runbooks/` 目录新增 5 章，与已有 `v3-claude-state-volumes.md` 保持一致风格。文件名前缀 `v3-`：
  - `v3-upgrade-guide.md` — 升级指南
  - `v3-apparmor-deployment.md` — AppArmor override 部署
  - `v3-doctor-troubleshoot.md` — doctor 排障手册
  - `v3-persistent-volume-lifecycle.md` — 持久卷生命周期与 GC（与已有 `v3-claude-state-volumes.md` 整合，不重复）
  - `v3-error-code-index.md` — 错误码索引
- **验收清单**：`scripts/v3-acceptance-checklist.sh` — 可执行 bash 脚本，遍历 30 条 REQ + 4 条 BASE，每项输出 `[PASS]/[FAIL]/[SKIP]` + 证据路径。脚本末尾生成 markdown 报告 `v3-acceptance-report.md`。
- **签字流程**：
  1. 脚本在目标环境执行生成报告
  2. 报告附于 Phase 35 PR 中
  3. PR 合并视为"签字通过"
  4. 真机环境需在报告中显式标注机器信息（OS 版本、硬件型号、执行时间）
- **版本标记**：手册头部标注 `适用版本: v3.0.x`，验收报告文件名含日期戳（`v3-acceptance-report-20260422.md`）。

### Claude's Discretion

- 10k 文件 synthetic 生成的具体目录深度和文件分布比例
- 性能基准报告的精确输出格式（JSON schema 细节）
- 弱网 UAT 脚本中 `tc` vs `iptables` 的最终选型（优先 `tc`，如环境不支持回退 `iptables`）
- 验收清单脚本中 SKIP 项的判定逻辑（环境不具备时优雅跳过）
- 运维手册的章节内具体排版和示例命令格式

</decisions>

<specifics>
## Specific Ideas

- 性能基准应输出类似 `go test -bench` 风格的表格，方便与后续版本对比回归
- 弱网 UAT 的 2min 场景脚本应在拔网后每 10s 打印一次状态，方便观察退避序列
- 验收清单参考 Phase 34 的 `ci-doctor-grep.sh` 风格——断言明确、失败时输出具体行内容
- 运维手册每章必须包含"快速诊断命令"小节（3-5 条最常用的 copy-paste 命令）

</specifics>

<code_context>
## Existing Code Insights

### Reusable Assets

- `scripts/ci-doctor-grep.sh`：Phase 34 的 doctor M14 验证脚本，可作为验收清单脚本的模板（JSON/文本双模式检查、错误码格式断言）
- `scripts/verify-fuse-compat.sh`：FUSE 兼容性验证脚本，Phase 35 可复用阶段 1-4 的逻辑作为基准测试前置检查
- `scripts/verify-managed-image.sh`：镜像验证脚本，BASE-04 CI gate 可直接复用
- `internal/claudedock/errcodes/`：错误码注册表，验收清单可遍历断言所有错误码均有中文 message + next_action
- `internal/claudedock/doctor/`：5 维度检查框架，运维手册 `v3-doctor-troubleshoot.md` 可直接引用其检查逻辑
- `test/bootstrap/e2e_bootstrap_ssh.sh`：e2e 测试脚本模板（PASS/FAIL 计数、断言模式）

### Established Patterns

- 验证脚本统一风格：`pass()`/`fail()`/`warn()`/`info()` 函数 + 汇总计数 + 退出码 0/1 区分
- CI 工作流在 `.github/workflows/ci.yml`，新增 job 遵循现有 `go-test` / `web-build` 的矩阵结构
- 运维手册在 `docs/runbooks/` 目录，markdown 格式，头部标注适用版本和关联 REQ-ID
- 错误码格式 `<DOMAIN>_<KIND>_<NUM>`，验收时需断言无重复、每条有中文 message + next_action

### Integration Points

- CI gate：`.github/workflows/ci.yml` 新增 `perf-benchmark` job 和 `image-size-regression` job
- 基准脚本：输出到 `.planning/phases/35-e2e/benchmarks/` 目录，供后续版本对比
- 验收报告：脚本生成后提交到版本控制，作为 v3.0 发布附件
- 运维手册：与现有 `docs/zh/guide/` 用户文档互补，不重复架构说明，聚焦排障和运维操作

</code_context>

<deferred>
## Deferred Ideas

- 持续性能监控（perf regression dashboard）—— v3.1+ 可考虑，不在本验收 phase 内
- 自动化真机农场（multi-OS CI runner）—— 资源投入较大，v3.1 评估
- 性能基准的历史趋势图（自动生成折线图对比各版本）—— 需要额外基础设施，v3.1 评估
- 弱网 UAT 的 packet-level 抓包分析 —— 如验收发现问题时可深入，非本 phase 交付物

</deferred>

---

*Phase: 35-e2e*
*Context gathered: 2026-04-22*
