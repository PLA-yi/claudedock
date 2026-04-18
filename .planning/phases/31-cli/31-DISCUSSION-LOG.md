# Phase 31: CLI 三层文件映射重构 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-18
**Phase:** 31-cli
**Mode:** `--auto`（所有 gray area 选定，所有 question 自动选 recommended）
**Areas discussed:** Mutagen 二进制 + daemon、Mutagen 默认同步模式 + macOS APFS、安全门 + 50MB 白名单、降级状态机 + 错误码 + banner、OAuth 检查 + 并发挂载时序

---

## Mutagen 二进制分发 + daemon 生命周期（Q1 + Q2）

### Q1.1 — Mutagen 客户端二进制如何获取？

| Option | Description | Selected |
|--------|-------------|----------|
| go:embed 多平台二进制 | darwin/linux × amd64/arm64 共 4 个 ~3MB，首次运行 extract 到 `~/.cloud-claude/bin/mutagen` | ✓ |
| 检测本机 brew install / 提示用户安装 | 增加用户摩擦，违背"零增量摩擦"承诺 | |
| 首次运行从 GitHub release 下载 | 需出网且全隧道下可能受出口 IP 影响，违背"零增量特权"目标 | |

**[auto] 选择：** go:embed 多平台二进制
**理由：** Q1 ROADMAP 倾向 (a)；与 v2.0 单一二进制分发模型一致；无新增网络依赖

### Q1.2 — Mutagen daemon 生命周期？

| Option | Description | Selected |
|--------|-------------|----------|
| cloud-claude 启动时 daemon start，退出不停（长期复用） | 避免每次启动 ~500ms 开销，符合 ≤8s 基线 | ✓ |
| 每次 session 起停 daemon | 资源最小，但首连耗时增加 | |

**[auto] 选择：** 长期复用
**理由：** Q2 ROADMAP 倾向 (a)；多 cloud-claude 共享同一 daemon（`MUTAGEN_DATA_DIRECTORY=~/.cloud-claude/mutagen`）；session 命名 `cloud-claude-{claude_account_id}-{cwd_hash8}` 唯一化

### Q1.3 — 多 cloud-claude 并发对同一 daemon 写竞争？

| Option | Description | Selected |
|--------|-------------|----------|
| 共享同一 daemon + session 命名唯一化 | Mutagen daemon 自身天然单实例，session 命名约定避免碰撞 | ✓ |
| 每 cloud-claude 独立 daemon（多 data dir） | 违背 Q2 决策，资源开销大 | |

**[auto] 选择：** 共享 daemon + session 命名唯一化
**理由：** 账号级单例锁 REQ-F5-D 在 Phase 32 落地，本阶段只需保证 session 名不冲突

---

## Mutagen 默认同步模式 + macOS APFS（Q3 + PITFALLS M5）

### Q2.1 — Mutagen 默认 mode？

| Option | Description | Selected |
|--------|-------------|----------|
| `two-way-resolved`（alpha 优先，本地胜出） | CLI 工具典型流程"本地编辑 + 远端 claude 偶尔写"，本地权威符合直觉；冲突自动按 alpha 解决避免堆积 | ✓ |
| `two-way-safe`（保守，冲突堆积需人工） | 数据保护强但 REQ-F1-E 警告会高频触发，影响"日常开发主战场"目标 | |

**[auto] 选择：** `two-way-resolved`
**理由：** Q3 是 ROADMAP 明确"必须本阶段拍板"的议题；研究子结论矛盾；Mutagen 官方推荐 CLI 用 resolved；与 macOS APFS 强制要求一致

### Q2.2 — macOS APFS case-insensitive 行为？

| Option | Description | Selected |
|--------|-------------|----------|
| 启动检测 `diskutil info /` + 命中输出 informational + 强制 `two-way-resolved` | 与默认 mode 已一致，仅打印告知；防御 PITFALLS M5 | ✓ |
| 完全交给 Mutagen safety mode 处理 | 风险高，case 冲突可能丢数据 | |
| 拒绝在 APFS 上启动 mutagen，强制 sshfs-only | 体验下降明显，macOS 是主要开发平台 | |

**[auto] 选择：** 检测 + informational + 强制 two-way-resolved
**理由：** PITFALLS M5 标记为 BLOCKER；与 Q3 决策天然一致；不阻塞 macOS 用户

### Q2.3 — 强制 owner / group 与 mode？

| Option | Description | Selected |
|--------|-------------|----------|
| `--default-owner-beta=id:1000 --default-group-beta=id:1000` + Mutagen 默认 mode | 与 Phase 29 D-17 容器 UID/GID 对齐；mode 不显式覆盖避免与 git 冲突 | ✓ |
| 显式设置 file mode 0644 / dir mode 0755 | 与 git 默认行为可能冲突，复杂度高 | |

**[auto] 选择：** owner/group 强制 + mode 默认
**理由：** REQ 仅要求 owner/group 强制（Phase 29 D-17 上下文），mode 留 Mutagen 默认更安全

---

## 首次同步安全门 + 50MB 白名单（Q7 + REQ-F1-D）

### Q3.1 — 首次同步前 alpha/beta 安全门？

| Option | Description | Selected |
|--------|-------------|----------|
| 是 + 轻量探测（≤300ms） | 仅检查 alpha empty + beta non-empty，不阻塞 ≤8s 基线；防御 C5 反向清空 | ✓ |
| 完整 alpha/beta 双向 diff | 阻塞 ≤8s 基线，违背 REQ-F1-B | |
| 不做安全门，信任 Mutagen safety mode | 违背 PITFALLS C5（safety mode 仅防 alpha 完全清空，防不住"远端有文件被本地空目录覆盖"） | |

**[auto] 选择：** 是 + 轻量探测
**理由：** Q7 ROADMAP 倾向"是 + 但不阻塞"；C5 是数据安全 BLOCKER 必须防御；远程 `find -mindepth 1 -maxdepth 1 | head -1` ≤300ms 完成

### Q3.2 — 触发安全门后的动作？

| Option | Description | Selected |
|--------|-------------|----------|
| 错误码 `MOUNT_MUTAGEN_SAFETY_GUARD` + 中文提示 + 退出非 0 | 数据安全风险，必须用户显式确认；不静默降级 | ✓ |
| 自动降级到 sshfs-only 不告知 | 违背 PITFALLS M13 禁止静默降级 | |
| 自动从 beta 反向同步到 alpha | 风险极高，可能覆盖用户本地 | |

**[auto] 选择：** 错误码 + 退出非 0
**理由：** ROADMAP §Phase 31 Success Criteria 第 6 条字面要求"必须中止并输出 MOUNT_MUTAGEN_SAFETY_GUARD，不允许执行 sync"

### Q3.3 — 50MB 白名单触发逻辑？

| Option | Description | Selected |
|--------|-------------|----------|
| 本地 `du -sb {cwd}` 检查，>50MB 拒绝 + 自动降级 sshfs + ignore 配置建议 | 检查在本地完成快，提示包含 top3 子目录 | ✓ |
| 远端检查 | 需挂载后才能跑，违反顺序 | |
| 不检查，依赖 Mutagen 自身限速 | Mutagen 无内置 50MB 限制，违反 REQ-F1-D | |

**[auto] 选择：** 本地 `du -sb` + 自动降级 + 智能建议
**理由：** REQ-F1-D 字面要求"自动改用 sshfs 兜底" + "明确中文提示与 ignore 配置建议"

### Q3.4 — 默认 ignore 列表？

| Option | Description | Selected |
|--------|-------------|----------|
| `.git/` `node_modules/` `target/` `dist/` `*.pyc` `.venv/` `__pycache__/` `.next/` `build/` `.cache/` `.DS_Store` | 覆盖主流语言生态产物 | ✓ |
| 仅 `.git/` `node_modules/` | 太窄，会触发 50MB 拒绝 | |
| 让用户自配置 `.mutagen.yml` | 首次使用零成本不可达成 | |

**[auto] 选择：** 11 条默认 ignore（写入 `~/.cloud-claude/mutagen-defaults.yml`）
**理由：** ROADMAP scope 字面列出 5 条 + 研究 PITFALLS 推荐扩展；用户工程级 `.mutagen.yml` 优先级仍最高

---

## 降级状态机 + 错误码命名空间 + banner UI（REQ-F2 + M13 + F8）

### Q4.1 — `--mount-mode` flag 四档语义？

| Option | Description | Selected |
|--------|-------------|----------|
| `auto` 状态机降级 / `full`/`mutagen-only`/`sshfs-only` 单档失败即退出 | 严格分离自动 vs 强制；满足 REQ-F2-A | ✓ |
| 所有模式都尝试降级（包括 `full`） | 削弱 `full` 的语义，用户无法强制四档完整模式 | |

**[auto] 选择：** auto 降级 / 其它三档严格
**理由：** REQ-F2-A 字面要求四档语义独立；`full` 的存在意义就是"我要完整三层不接受降级"

### Q4.2 — 降级触发的最大耗时？

| Option | Description | Selected |
|--------|-------------|----------|
| 每档启动 ≤2s 超时（context.WithTimeout）触发降级 | REQ-F2-B 字面 2 秒 | ✓ |
| ≤5s | 偏宽，降级不够灵敏 | |
| 无超时，等到 mount 结果（成功 / 错误） | 错误返回慢的场景下违反 ≤2s | |

**[auto] 选择：** 2s 超时
**理由：** REQ-F2-B 字面 2 秒；errgroup + context.WithTimeout 标准实现

### Q4.3 — 降级历史持久化？

| Option | Description | Selected |
|--------|-------------|----------|
| 写入 `~/.cloud-claude/last-session.json` 含 intended_mode / actual_mode / downgrade_chain | 给 Phase 34 doctor 第一屏读取（M13 终验数据源） | ✓ |
| 仅 stderr 输出，不持久化 | doctor 无法回放降级历史，违背 M13 验收预期 | |

**[auto] 选择：** 持久化 last-session.json
**理由：** PITFALLS M13 终验由 Phase 34 验证，本阶段必须产出数据源

### Q4.4 — 错误码命名空间格式？

| Option | Description | Selected |
|--------|-------------|----------|
| `<DOMAIN>_<KIND>_<NUM\|TAG>` 大写下划线（`MOUNT_MUTAGEN_VERSION_SKEW` 等） | 与 ROADMAP §Phase 34 一致；与 v2.0 小写下划线无冲突 | ✓ |
| 数字编码（`E1001` 等） | 不可读，违背 REQ-F8-B 中文 next_action 易用性 | |
| 全 snake_case 与 v2.0 一致 | 命名空间冲突风险（PITFALLS C8） | |

**[auto] 选择：** `<DOMAIN>_<KIND>_<NUM\|TAG>` 大写
**理由：** ROADMAP §Phase 31 / 34 一致；PITFALLS C8 防御

### Q4.5 — banner UI 颜色与降级提示？

| Option | Description | Selected |
|--------|-------------|----------|
| `[full]` 绿色 / `[mutagen-only]` 黄色 / `[sshfs-only]` 黄色 + 降级原因行；NO_COLOR 关色 | REQ-F2-C 字面要求 + NO_COLOR 标准 | ✓ |
| 三档全绿色 | 模糊降级状态，违背 M13 | |
| ASCII 符号代替颜色 | 退化体验 | |

**[auto] 选择：** 绿/黄分级 + NO_COLOR
**理由：** REQ-F2-C 字面要求；OOS-A15 禁 emoji 但允许 ANSI 色

### Q4.6 — 三段式中文进度文案？

| Option | Description | Selected |
|--------|-------------|----------|
| `初始化文件映射 (1/3) 热同步源码中…` / `(2/3) 启动冷兜底…` / `(3/3) 合并视图…` | REQ-F1-B 字面文案 | ✓ |
| 英文 progress | 违背项目中文沟通约定 | |
| 仅一行 spinner | 违背 REQ-F1-B 三段式要求 | |

**[auto] 选择：** ROADMAP scope 字面文案
**理由：** REQ-F1-B + ROADMAP 字面一致；模式跳过场景文案变体由 D-18 给出

---

## OAuth 过期检查 + 并发挂载时序（REQ-F7-C + PITFALLS C3）

### Q5.1 — OAuth 检查时机？

| Option | Description | Selected |
|--------|-------------|----------|
| SSH 握手成功 + connections 建立后、Mutagen/sshfs 启动前并发执行 | 早期发现，避免挂载完才报错；并发不阻塞 ≤8s | ✓ |
| 进入 claude 进程前最后一刻 | 浪费 mount 资源，且违背 REQ-F7-C「禁止 claude 报错」 | |
| 不检查，让 claude 自己报错 | 直接违反 REQ-F7-C | |

**[auto] 选择：** 握手后 + mount 前并发
**理由：** REQ-F7-C 字面"禁止 claude 进程先报错"；errgroup 与 mount 并发不阻塞

### Q5.2 — OAuth 检查的远程命令与三态？

| Option | Description | Selected |
|--------|-------------|----------|
| 远程 `cat /home/claude/.claude/.credentials.json`，三态：not_found / expired / expiring_soon (<5min) | 简单且准确；`expiresAt` 是 Mutagen 标准字段 | ✓ |
| host-agent 新 endpoint 探测 | 违背 D-04 "不扩展 host-agent" 决策 | |
| 容器 entrypoint 启动时 export env var | 镜像侧改动违背"零增量特权"且非 cloud-claude 自治 | |

**[auto] 选择：** 远程 cat + 三态分支
**理由：** D-04（不扩展 host-agent）+ REQ-F7-C 字面要求；2s 超时保护

### Q5.3 — claude_account_id 缺失时如何处理？

| Option | Description | Selected |
|--------|-------------|----------|
| 跳过 OAuth 检查 + warning + 不阻塞 mount | 兼容 v2.0 旧 gateway / 新装环境 | ✓ |
| 直接退出非 0 | 破坏向后兼容（Phase 30 D-08 允许 omitempty） | |
| 静默跳过 | 违背 M13（应当告知用户） | |

**[auto] 选择：** 跳过 + warning
**理由：** Phase 30 D-08 字段 omitempty；M13 要求显式告知

### Q5.4 — SSH connection 数与并发模型？

| Option | Description | Selected |
|--------|-------------|----------|
| 双 connection（A 控制 / B 数据）+ Mutagen ‖ sshfs goroutine 并发，mergerfs 在 conn-A 串行 | 避免 SSH multiplexing 弱网拖垮；ROADMAP scope 字面"独立 SSH channel" | ✓ |
| 单 connection 多 channel | multiplexing 抖动相互拖垮，违背 PITFALLS | |
| 三 connection（mutagen / sshfs / 控制各一） | 资源浪费，认证三次 | |

**[auto] 选择：** 双 connection + 并发 mount
**理由：** ROADMAP §Phase 31 scope 字面"启动并发改造：Mutagen sync create ‖ sshfs mount 走独立 SSH channel"

### Q5.5 — sshfs 抖动监控？

| Option | Description | Selected |
|--------|-------------|----------|
| watcher 每 5s `mountpoint -q`，3 次连续失败（15s）→ mergerfs runtime branch 摘除 + warning | 防御 PITFALLS C3；不自动重挂避免无限循环 | ✓ |
| 不监控，让 mergerfs 自然 hang | 直接违背 ROADMAP §Phase 31 Success Criteria 第 10 条 | |
| 失败立即 cleanup + 重启整个 mount | 重连成本高，影响运行中 claude | |

**[auto] 选择：** watcher + 主动摘除 + warning
**理由：** ROADMAP §Phase 31 scope 字面"CLI 监测 sshfs 异常时主动从 mergerfs branch 摘除以避免整体挂死"；恢复留 Phase 34 doctor `--fix`

### Q5.6 — Mutagen conflict 冒泡时机？

| Option | Description | Selected |
|--------|-------------|----------|
| 启动 banner 之后立即输出 + `cloud-claude sync conflicts` 子命令查看清单 | 接近 REQ-F1-E "下次回车前"语义；PTY 拦截留 v3.1 | ✓ |
| 完整 PTY 拦截「下次回车前」严格语义 | 复杂度高（需拦截 stdin），留 v3.1 | |
| 仅写日志，不在 banner 区显示 | 用户感知不到，违背 REQ-F1-E | |

**[auto] 选择：** banner 之后 + 子命令
**理由：** REQ-F1-E 接近实现；严格 PTY 拦截 deferred；`sync conflicts` 子命令最小可行版本足够覆盖 success criteria

---

## Claude's Discretion

由 planner / executor 自由选择的实现细节（详见 CONTEXT.md `<decisions>` § Claude's Discretion）：

- 二进制 fetch 脚本是否纳入 CI
- `cloud-claude sync conflicts` 的 cobra 注册位置
- `errcodes` 包的中文 i18n 形态（本阶段硬编码即可）
- watcher goroutine 的退出协议（context cancel 标准实现）
- macOS APFS 检测 `diskutil` 不可用时的 fallback
- 测试矩阵 mock vs testcontainers-go 真实容器的比例
- `MountConfig` struct 的字段扩展粒度

## Deferred Ideas

详见 CONTEXT.md `<deferred>` 段。要点：

- `--mutagen-force` flag 覆盖 50MB（v3.1）
- `cloud-claude sync resolve <pattern>` 自动解决冲突（v3.1）
- 「下次回车前 prompt 上方插入」严格 PTY 拦截（v3.1）
- Mutagen daemon 退出 GC（v3.1）
- 错误码 i18n 英文版本（v3.1）
- doctor mount 维度真实实现（Phase 34）
- arm64 真机集成测试（Phase 35 / v3.1）
- mergerfs 3 路 branch（v3.1）
