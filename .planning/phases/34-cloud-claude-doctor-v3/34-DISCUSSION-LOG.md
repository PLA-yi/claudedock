# Phase 34: cloud-claude doctor v3 + 错误码统一 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-21
**Phase:** 34-cloud-claude-doctor-v3
**Mode:** `--auto`（Claude 自动选定每个 gray area 的推荐选项；用户未交互回答）
**Areas discussed:** 错误码命名空间补全 / doctor 子命令结构 / `--fix` 幂等性 / 第一屏布局 / explain 数据源 / `--json` schema / 远端 vs 本地拆分 / 测试矩阵

---

## GA-1：错误码命名空间补全（DOMAIN 闭合粒度）

| Option | Description | Selected |
|--------|-------------|----------|
| 8 域闭合：`MOUNT/SESSION/NET/STATE/SYSTEM/SSH/AUTH/DISK` | 既有 3 域 + 新增 5 域，每域语义边界清晰，grep 友好 | ✓ |
| 全合并到 `SYSTEM_*` 单域 | 命名简单，但 grep 粒度太粗、与 v2.0 多前缀风格不一致 | |
| 按 phase 分前缀如 `P34_*` | 与 phase 数字耦合，未来重排 / 删除 phase 时破坏字面量 | |
| `<DOMAIN>_<KIND>_<SEQ_NUM>`（带数字尾缀如 `_001`） | 已被 Phase 33 admin handler `STATE_VOLUME_IN_USE_001` 部分采用，但全局推广噪音大；本阶段允许但不强制 | |

**User's choice (auto):** 8 域闭合
**Notes:** Phase 31/32 已有 MOUNT/SESSION/NET 三域；本阶段补 STATE/SYSTEM/SSH/AUTH/DISK 五域，达到 8 域闭合。Phase 33 已硬编码的 `STATE_VOLUME_IN_USE_001` 字面量不变（D-27 补 Registry 注册），数字尾缀允许但非强制（D-23）。

---

## GA-2：`cloud-claude doctor` 子命令结构

| Option | Description | Selected |
|--------|-------------|----------|
| `doctor [domain]` 子命令 + 顶层 flag | 与 `brew doctor` / `cargo check` 行业惯例一致；`cobra ValidArgs` 内置校验；与现有 `cloud-claude ssh doctor` 双入口共存 | ✓ |
| `doctor --domain=mount` flag 风格 | 长 flag 啰嗦；与 cobra ValidArgs 不直接契合 | |
| 强制每维度独立子命令 `doctor-mount` / `doctor-network` | 顶层命令爆炸 7 个，UX 差；与现有 `env check` / `ssh doctor` 风格不一致 | |

**User's choice (auto):** subcommand 风格
**Notes:** ValidArgs={network,auth,ssh,mount,disk,all}，默认 all（D-05）。`--fix`/`--verbose`/`--json`/`--yes` 是顶层 flag 便于其它子命令复用（D-05 第 4 条）。`cloud-claude ssh doctor`（v2.0 quick task 260417-0w4 已 ship）保持兼容（D-04）。

---

## GA-3：`--fix` 自动修复幂等性边界（Q9）

| Option | Description | Selected |
|--------|-------------|----------|
| 默认全幂等 + 危险操作 stdin `y/N` + CI `--yes` 跳过 | Q9 倾向；最小必要安全门；CI 友好 | ✓ |
| 全部需二次确认（即使重启 mutagen daemon） | 增加用户操作负担；与「优雅好用」目标背离 | |
| 全部静默自动（无确认） | DNS flush / FUSE 解挂等系统级动作有副作用，不应静默 | |

**User's choice (auto):** 默认全幂等 + 危险操作 stdin y/N
**Notes:** 5 类必交付（D-09 表）：mutagen daemon restart（无确认）/ FUSE 残留解挂（**y/N**）/ known_hosts 清条目（无确认）/ Entry token refresh（无确认）/ DNS flush（**y/N**）。`confirmDestructive` helper 在 `--yes` / `--json` 模式下行为见 D-10。

---

## GA-4：第一屏布局（M13 终验：降级历史可见）

| Option | Description | Selected |
|--------|-------------|----------|
| 第一屏先打印降级历史 banner，再打印 5 维度检查矩阵 | M13 验收锚点；用户必须能立刻看到「上次连接已降级」 | ✓ |
| 检查矩阵在前，降级历史末尾汇总区 | 用户在矩阵滚屏过去后才看到降级，违反 M13 字面要求 | |
| 不展示降级历史，要 `--verbose` 才显示 | 直接破坏 M13；OOS | |

**User's choice (auto):** 第一屏先打降级历史
**Notes:** 渲染顺序：标题 → 上次会话快照（含 DowngradeChain）→ 5 维度检查矩阵 → 汇总（D-13）。last-session.json 缺失时输出 `STATE_LAST_SESSION_MISSING` informational，不算 fail。

---

## GA-5：`cloud-claude explain <code>` 数据源

| Option | Description | Selected |
|--------|-------------|----------|
| Registry Entry + 同包 `ExtendedExplanations map[Code]string` | 单文件管理；Go map 与 Registry 同步演进；CI 测试覆盖天然 | ✓ |
| 每 Code 一个 markdown 文件 + `go:embed` | 引入 markdown parser 复杂度；文件管理负担 | |
| 仅输出 `Format(code)` 两段（不扩展长说明） | 与 `rustc --explain` 期待差距大；REQ-F8-C 字面要求详细说明 | |
| 读取 `.planning/research/PITFALLS.md` 直出 | 把 planning 文档与运行时绑定，破坏「planning 仅文档」边界 | |

**User's choice (auto):** Registry + ExtendedExplanations map
**Notes:** 模板见 D-18（触发场景 / 根本原因 / 复现方式 / 修复路径 / 关联文档）。informational 类（`*_BACKOFF` / `*_NOTIFIED` / `MOUNT_APFS_CASE_INSENSITIVE` 等）显式登记到 `ExplainExempt set` 豁免（D-02 第 7 条 + D-24 第 4 条）。

---

## GA-6：`--json` Schema

| Option | Description | Selected |
|--------|-------------|----------|
| 自定义 schema_version=1 + omitempty 演进 | 与 last-session.json / Phase 30 AuthResponse 风格一致；jq 友好 | ✓ |
| 直接输出 OpenAPI 风格 schema | 重复造轮子；本场景不需要严格 schema 校验 | |
| 复用 `brew doctor --json` 字段名 | brew 字段语义与本项目错误码 / 维度模型不匹配 | |
| 不提供 `--json`，只输出文本 | 违反 REQ-F6-D 字面要求 | |

**User's choice (auto):** 自定义 schema_version=1
**Notes:** schema 见 D-15；锁 v1 不动，新字段全部 omitempty；`details` 是开放 map[string]any 字段（各 check 自由放调试信息）。退出码语义对齐 brew doctor（D-16）。

---

## GA-7：远端 vs 本地检查项拆分

| Option | Description | Selected |
|--------|-------------|----------|
| 本地 + 远端混合（每维度按需选） | mergerfs xattr / Mutagen agent 必须容器内验；DNS / FUSE 残留 / disk 必须本地；混合模型 17 项 check 见 D-19 表 | ✓ |
| 全部走 SSH 远端 | 容器宕机时 doctor 完全失效；失去诊断价值 | |
| 全部本地 mock 远端 | mergerfs xattr / Mutagen agent 版本无法靠 mock 信任 | |

**User's choice (auto):** 本地 + 远端混合
**Notes:** 远端 SSH 检查统一通过 `RemoteRunner` interface（D-20）；远端连接失败时所有 RequiresRemote=true 的 check 标记 StatusSkip（D-20）。secondary 端识别（last-session.json `client_role=secondary`）跳过 Mutagen sync 状态检查（specifics 末段）。

---

## GA-8：测试矩阵

| Option | Description | Selected |
|--------|-------------|----------|
| Mock-first + 单 fixture happy path | 与 Phase 31/32/33 已建立的 t.Skip 模式一致；CI 时间可控；mock 覆盖 4 路径 + fixture 覆盖端到端 | ✓ |
| 全 docker fixture 集成测试 | CI 时间膨胀；与现有模式不一致 | |
| 纯 mock 不跑 docker | mergerfs xattr / mount 参数无法靠 mock 信任 | |
| 引入 testcontainers-go | Phase 31 D-显式拒绝；本阶段沿用 | |

**User's choice (auto):** Mock-first + 单 fixture happy path
**Notes:** Plan 02 全 mock；Plan 03 1 个 happy path + 1 个 fail 注入（kill mutagen-agent）；docker daemon 不可用时 t.Skip（D-24）。CI grep 断言（M14 终验）走 `scripts/ci-doctor-grep.sh`（specifics 第 2 段）。

---

## Claude's Discretion

以下细节用户未介入，由 planner / executor 按实现便利性决定（汇总自 CONTEXT.md `### Claude's Discretion`）：

- `RemoteRunner` 用 interface vs 包级 var（建议 interface）
- `confirmDestructive` 在非 TTY 时直接拒绝
- mergerfs xattr 解析的容错（substring 匹配 vs 严格分割）
- `--verbose` 输出冗余度
- disk 阈值（500MB/100MB/1GB）是否走 config（建议本阶段硬编码）
- `--json` 是否兼容 `jq -c` 紧凑输出
- `AUTH_OAUTH_REFRESH_FAILED` 修复路径文案
- `SSH_KNOWN_HOSTS_CONFLICT` 是否支持指定 known_hosts 路径
- explain 是否支持模糊匹配（建议本阶段不做）
- doctor 不带 init 时自动跳过远端 check（建议是）

## Deferred Ideas

discuss 过程中识别但不交付的 follow-up（汇总自 CONTEXT.md `<deferred>`）：

- 错误码 i18n（v3.1）
- doctor 历史报告归档（v3.1）
- `cloud-claude explain --list` / 模糊匹配（v3.1）
- doctor 检查项并发执行（v3.1）
- 跨主机 doctor `--gateway=...`（v3.1）
- doctor JSON schema_version=2（v3.1）
- doctor `--watch` 模式（v3.1）
- v2.0 lower-case 错误码迁移到 Registry（v3.1）
- doctor `mount.mergerfs_branches` 自动 remount 修复（Phase 35 真机后再决定）
- `cloud-claude doctor --report-bug`（v3.1，与 OOS-A11 配套）
- doctor 远端 `ssh.workspace_ssh_keys` 错误码化（v3.1，涉及 v2.0 quick task 兼容）
