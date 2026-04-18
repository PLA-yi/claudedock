# v3.0 远端开发体验升级 · 研究综述（SUMMARY）

**Project:** Cloud CLI Proxy
**Milestone:** v3.0 远端开发体验升级（F1–F8）
**Domain:** 容器化远程开发 CLI / SSH 云主机 + Mutagen+sshfs+mergerfs 三层文件系统 + tmux 会话恢复
**Researched:** 2026-04-18
**Confidence:** HIGH（4 份子研究全部 HIGH，关键决策均有官方文档或 GitHub issue 直接交叉验证）

> 本文档是 STACK.md / FEATURES.md / ARCHITECTURE.md / PITFALLS.md 的"决策提炼版"，
> 下游 agent（roadmapper / REQUIREMENTS drafter / planner）应直接消费本文，
> 仅在需要原始证据链时再回查对应子文档（已保留全部 URL 锚点）。

---

## 1. Executive Summary

v3.0 是一次**纯粹的体验升级**：v2.0 已经把 cloud-claude 跑通（Go 二进制 + sshfs slave + Entry API + sing-box tun + AppArmor unconfined），v3.0 要把它从"能跑起来"升级到"能成为日常开发主战场"。8 项 feature 围绕三条主线：**主线 A（F1/F2）解决文件系统性能与降级**、**主线 B（F3/F4/F5）解决会话可靠性与多端**、**主线 C（F6/F7/F8）解决可观测、状态持久化和错误体系**。

**推荐技术路径完全收敛**，没有真正的"二选一"分歧：Mutagen + sshfs + mergerfs 三层是 union FS 领域唯一同时满足"双向 + 实时 watcher + 跨 FUSE union + 活跃维护"的组合；tmux 是唯一同时支持"持久 session + 多 client 共享 + 跨版本兼容"的成熟方案；SSH 弱网容忍坚决**不引入** mosh / autossh / Eternal Terminal（原因见 §4.4）。所有新增组件都能复用 v2.0 已经放开的 `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined`，**零增量特权**。

最大风险集中在**三层 mount 的正确性**和**降级路径的可见性**：mergerfs 2.41 默认 `category.create=pfrd` 会让新文件随机落 branch、Mutagen 双向同步在权限/大小写不当组合下会反向清空本地目录、sshfs 抖动会让 mergerfs 整体挂死、Ubuntu 25.04 AppArmor 默认禁止嵌套 FUSE。这 4 条 + tmux 被 systemd-logind 杀掉、Mutagen agent 版本漂移、错误码命名空间冲突共同构成 **TOP 8 Critical Pitfalls**，必须在 Phase 1（镜像）和 Phase 3（CLI 文件映射）一次性防御到位，否则 v3.0 发布即翻车。

---

## 2. 关键技术决策表（来自 STACK.md，每行 1 条结论）

### 2.1 新增 / 升级组件

| 组件 | 唯一推荐 | 版本 | 集成形式 | 与 v2.0 兼容性 | STACK 出处 |
|------|---------|------|----------|----------------|------------|
| 热同步引擎 | **Mutagen** | v0.18.1 | CLI 端 `go:embed` 二进制 + 容器预放 agent tarball；走 SSH transport 复用 v2.0 通道 | 零增量特权、零端口、与 v2.0 嵌入式 SFTP server 互不干扰 | STACK §1，<https://github.com/mutagen-io/mutagen> |
| 联合视图 | **mergerfs** | 2.41.1 | 容器内预装（GitHub static .deb，**不走 apt**）+ entrypoint 显式锁定参数 | 复用 v2.0 已开的 SYS_ADMIN + /dev/fuse + apparmor=unconfined | STACK §2，<https://github.com/trapexit/mergerfs/releases/tag/2.41.0> |
| 冷兜底网络 FS | **sshfs**（升级） | 3.7.5 | 容器内已装；从 v2.0 任意版本升至 3.7.5 修补 readdir 缓存泄漏 | ABI 与 3.7.3 相同，slave 模式行为不变 | STACK §3，<https://github.com/libfuse/sshfs/releases/tag/sshfs-3.7.5> |
| 会话恢复 | **tmux** | 3.6a | 容器内 `apt install tmux`（v2.0 镜像**已预装**，仅版本核对）+ entrypoint `new-session -A` 自动包一层 | 全用户态，零增量；socket 在 `/tmp/tmux-${UID}` 不受 sing-box tun 影响 | STACK §4，<https://github.com/tmux/tmux/releases/tag/3.6a> |
| 弱网容忍 | **OpenSSH `ServerAliveInterval` + cloud-claude 自实现 reconnect** | 复用 v2.0 OpenSSH 10.2p1 / `golang.org/x/crypto/ssh` | CLI 端代码扩展，**不引入新组件** | 零兼容性风险 | STACK §5 |
| 状态持久化 | **Docker named volume** | Docker Engine 28.x（v2.0 已有） | worker 创建容器时 `--mount type=volume,src=claude-state-{account_id},dst=/home/claude/.claude` | volume 与 network namespace 正交，与 `--network=none + tun` 完全兼容 | STACK §6 |
| 错误码体系 | **沿用 v2.0 编码 + 分类前缀** | - | 纯 Go `errors.Is` + 自定义 error type | 不新增依赖；退出码新区段 50–69 | STACK §7 |
| doctor | **复用 cobra** | - | `cloud-claude doctor {network,auth,ssh,mount,disk}` + `--fix` | 不新增依赖 | STACK §8 |

**沿用 v2.0 不变（`image.lock` 必须凸 v3.0.0 大版本）：** Go 1.26.1、PostgreSQL 18.x、Docker Engine 28.x、sing-box 1.13.3、OpenSSH 10.2p1、libfuse3 3.18.x、`pkg/sftp`、cobra、shellescape、React 19/Vite 8。

### 2.2 明确不引入（每行有 PITFALLS 反证）

| 不要引入 | 反证（PITFALLS 锚点） | 替代方案 |
|----------|----------------------|----------|
| **mosh** | 走 UDP 60000-61000，与 sing-box tun + nftables 默认拒绝模型直接冲突；mosh 1.4.0 自 2022 起无新 release | OpenSSH `ServerAliveInterval` + cloud-claude 自实现 reconnect（PITFALLS M10） |
| **autossh** | 只解决"ssh 死了重启"，与 cloud-claude wrapper 套娃；重连后 sshfs/Mutagen 子 channel 仍僵死（PITFALLS M9） | cloud-claude 自管 3 个逻辑 channel 各自 re-dial |
| **Eternal Terminal** | 替换 SSH 协议体，破坏 v2.0 单 SSH 通道模型 | tmux + ssh 重连 |
| **zellij** | 官方明确 "sessions have never been backwards compatible"，镜像升级会丢用户运行中状态 | tmux 3.6a |
| **dtach / abduco** | 不支持多 client 共享 session，无法满足 F5 | tmux 3.6a |
| **OverlayFS（kernel）** | 不支持 FUSE-on-FUSE，lower 必须同一 FS | mergerfs |
| **unison / syncthing / lsyncd / rsync+inotify** | watcher 不及时 / 无冲突合并 / 多年无 release（详见 STACK §1） | Mutagen |
| **mergerfs 2.41 默认 `category.create=pfrd`** | 按剩余空间随机分 branch，违反"写入必落 hot"设计（PITFALLS C2） | 必须显式 `category.create=ff` + sshfs branch 标 `=NC,RO` |
| **`func.readdir=seq`（默认）** | sshfs branch 串行 readdir，10k 文件 `ls -R` >90s（PITFALLS C1） | 必须 `func.readdir=cor:4` + cache.attr/entry/readdir 调优 |
| **以 root 直接挂 named volume 不 chown** | UID 1000 容器用户写不进去，Mutagen handshake 失败甚至反向清空 alpha（PITFALLS C5） | Dockerfile 预建目录并 chown + entrypoint 二次 chown 兜底 |
| **依赖 systemd-logind 守 tmux** | `KillUserProcesses=yes`（Ubuntu 22.04+ 默认）会在 SSH 登出时杀掉 tmux server（PITFALLS C7） | 容器不跑 systemd，tmux 由 tini PID 1 间接守护 |
| **Mutagen 静默升级 / 跨版本 handshake** | client/agent patch 版本差就 handshake 失败（PITFALLS C4） | cloud-claude 启动时强制版本比对 + 不一致直接降级到 sshfs-only |

---

## 3. 每个 F 项的"必做行为清单"（来自 FEATURES.md，可直接转写为 REQ-ID）

> 每条都是用户感知层面的可观察行为；推荐 REQ-ID 命名沿用 FEATURES.md 已经给出的 `REQ-F<n>-<A/B/C>` 格式。

### F1 · 三层文件系统架构（Mutagen + sshfs + mergerfs）

- **REQ-F1-A** 容器内只暴露**单一 `/workspace` 路径**，用户和 Claude Code 不感知 hot/cold 分层。
- **REQ-F1-B** 首次连接到 prompt 可输入必须 ≤ 8s（含首轮 Mutagen 同步），过程中显示三段式进度 `初始化文件映射 (1/3) 热同步源码中…`。
- **REQ-F1-C** 在 10k 文件源码树执行 `rg .` / `ls -R` 的延迟必须 ≤ 本地 1.5×。
- **必做配置**：mergerfs 显式 `category.create=ff`、`func.readdir=cor:4`、`cache.attr=30`、`cache.entry=30`、`cache.readdir=true`、`cache.files=off`、`inodecalc=path-hash`、`branch: /workspace-hot=RW:/workspace-cold=NC,RO`（STACK §2 / PITFALLS C1+C2+M1）。
- **白名单守门**：cloud-claude 在创建 Mutagen session 前 `du -sb` 检查候选目录，> 50MB 拒绝 + 自动改用 sshfs 兜底（STACK §1 末段）。
- **冲突冒泡**：Mutagen 出现 conflict 时下次回车前在 prompt 上方插入中文警告 `⚠ 有 N 个文件同步冲突，运行 cloud-claude sync conflicts 查看`（FEATURES M2）。

### F2 · 降级路径与 `--mount-mode` 手动切换

- **REQ-F2-A** CLI 支持 `--mount-mode=auto|full|mutagen-only|sshfs-only`，默认 `auto`。
- **REQ-F2-B** 三层任一失败必须在 2 秒内降级到下一档，**不允许静默**——stderr 中文清晰输出当前模式 + 错误码（PITFALLS M13）。
- **REQ-F2-C** 当前 mount mode 必须在每次连接 banner 显示彩色标签（尊重 `NO_COLOR`）。
- **不做**：运行时自动"升级"模式（动态切挂载点会让进行中的 claude 进程看不到文件）；只允许下次会话尝试升级。
- **不做**：`--mount-mode=none`（等于回到原生 ssh，破坏 cloud-claude 核心承诺）。

### F3 · SSH 会话稳定性与自动重连

- **REQ-F3-A** 客户端 `ServerAliveInterval=15s`、`ServerAliveCountMax=4`（60s 客户端断）；服务端 `ClientAliveInterval=15s`、`ClientAliveCountMax=8`（120s 容忍，让客户端先反应）。**禁止 < 15s**（PITFALLS M11）。
- **REQ-F3-B** 断网期间用户键入字符必须**本地缓冲并以"未确认"灰色样式显示**，重连后按序提交（对标 Mosh 本地 echo）。
- **REQ-F3-C** 重连失败时 prompt 必须显示具体失败原因 + 下一步操作（按 Enter 重试 / 运行 doctor）。
- **重连退避**：1s → 2s → 4s → 8s → 30s 上限，复用本地缓存 Entry API token，**不重新弹密码**。
- **TCP 层**：socket 启用 `SO_KEEPALIVE`、Linux `TCP_USER_TIMEOUT=30000` / macOS `TCP_KEEPALIVE`（STACK §5）。
- **UX 阈值**（FEATURES M3）：`>1.5s RTT` 灰色 `…`；`>8s` 黄色 `网络抖动中（12 秒未响应）`；`>30s` 红色 `网络已断 35s，正在自动重试…` + 输入暂存。

### F4 · 会话恢复（tmux 默认包装）

- **REQ-F4-A** 容器内 SSH 会话默认用 tmux 包装（`exec tmux new-session -A -s claude`），断网后重连必须恢复同一会话。
- **REQ-F4-B** 用户可通过 `cloud-claude sessions ls/attach` 管理多个并行会话。
- **REQ-F4-C** tmux 不可用时 cloud-claude 不得阻塞启动，但必须明确告知 `[!] 容器内 tmux 不可用，会话恢复已禁用`。
- **必做配置**：`/etc/tmux.conf` 写 `set -ga terminal-overrides ",*:RGB"`（避免 Claude Code 颜色变灰，PITFALLS M8）+ `/etc/profile.d/cloud-claude.sh` 暴露 `CLAUDE_CODE_TMUX_TRUECOLOR=1`。
- **必做配置**：`window-size latest` + `aggressive-resize on`（多端尺寸不抖，PITFALLS M7）。
- **必做架构**：tmux 由 PID 1 `tini` 间接守护，**不依赖 systemd-logind**（PITFALLS C7）。

### F5 · 多端同账号 attach 同一 session

- **REQ-F5-A** 默认行为是多端**共享 attach** 同一 session，不踢人不报错。
- **REQ-F5-B** 第二端 attach 成功后必须在 banner 显示其它 client 的来源和活跃时间（中文）：`✓ 已 attach 到会话 claude-proj（另 1 个会话正在共享：mac-home / 5 分钟前活跃）`。
- **REQ-F5-C** `--new-session` 创建独立 session（命名 `claude-<short_id>`）；`--take-over` 强制独占并通知其它端。
- **多端 Mutagen 锁**：每 claude_account 同时只允许 **1 个** Mutagen sync session（避免双向冲突累积，PITFALLS M15）。后连端只 attach tmux + 观察，不参与文件同步。

### F6 · `cloud-claude doctor` 全面升级

- **REQ-F6-A** 必须覆盖 5 维度：**network / auth / ssh / mount（mutagen+sshfs+mergerfs 三层）/ disk**。
- **REQ-F6-B** 每项检查输出必须包含：符号 `[✓][!][✗]` + 简短原因 + **中文修复建议** + 错误码（4 项缺一不可，PITFALLS M14）。
- **REQ-F6-C** `doctor --fix` 能自动修复至少 5 类常见失败：mutagen agent 无响应、FUSE 残留挂载、known_hosts 冲突、token 过期需刷新、DNS 缓存污染。
- **必做**：`--verbose` 展开探测细节、`--json` 供脚本消费、`NO_COLOR` 关色、退出码 0/1/2 对标 brew doctor。
- **必做**：doctor 完全本地 + SSH 实现，**不给 host-agent 加 endpoint**（ARCHITECTURE §6）。

### F7 · Claude Code 状态持久化

- **REQ-F7-A** `~/.claude/` 必须通过独立 Docker named volume 持久化，volume 命名粒度 = 单个 claude_account（建议 `claude-state-{claude_account_id}`，带 label `com.cloud-cli-proxy.account_id`）。
- **REQ-F7-B** 容器重建后未过期的 OAuth credentials 必须保留，用户**无需重新登录**。
- **REQ-F7-C** credentials 过期时 cloud-claude 必须在连接建立**前**给出明确中文提示，不能让 claude 进程进入报错后才发现。
- **必做**：同时持久化 `~/.cache/claude`；entrypoint `chown -R 1000:1000 /home/claude` 兜底（PITFALLS M17）。
- **必做**：删除 claude_account 时同事务删 volume（按 label 过滤，PITFALLS M16）。
- **不做**：用 `ANTHROPIC_API_KEY` 替代 OAuth（官方明确互斥，FEATURES F7 anti-feature A13）。
- **不做**：容器内做 `claude login` 交互 OAuth（全隧道出网会打断回调）。

### F8 · 错误码与中文提示统一升级

- **REQ-F8-A** v3.0 所有新错误路径必须纳入统一错误码体系，code 格式 `<DOMAIN>_<KIND>_<NUM>`。新增分类前缀：`MOUNT_*`（三层 FS）/ `SESSION_*`（会话/多端）/ `NET_*`（弱网/重连）/ `STATE_*`（持久化 volume）。
- **REQ-F8-B** 每条错误输出必须包含 code + 中文原因 + 中文下一步建议，**三项缺一不可**。
- **REQ-F8-C** `cloud-claude explain <code>` 子命令对每个 code 给出详细中文说明和常见修复步骤（对标 `rustc --explain`）。
- **必做单元测试**：CI 遍历所有 code，断言无重复 + 每条都有中文消息 + 每条都有 `next_action` 字段（PITFALLS C8）。
- **不做**：错误码全局纯数字（如 `E1001`，可读性差）、emoji 替代符号（终端宽度/CI 日志兼容差）、stacktrace 默认透传给用户。

---

## 4. 建议的 Phase 切分（来自 ARCHITECTURE.md，含 Depends on 矩阵）

> roadmapper 可直接套用本节 Phase 编号 / 依赖关系 / 工作量评估。
> 完整代码改动清单见 ARCHITECTURE.md §9（30+ 文件级 状态/操作 表）。

### 4.1 Phase 列表

| # | Phase 名称 | 范围 | Features | 工作量 | Depends on | 可并行 |
|---|-----------|------|----------|--------|-----------|--------|
| **P1** | **受管镜像 v3 + Worker 容器参数扩展** | Dockerfile 加 mergerfs（GitHub static .deb，**非 apt**）+ mutagen-agent tarball + tmux 版本核对；entrypoint 启动 mergerfs；Worker 支持 `Volumes` 字段 + label；`image.lock` 凸 v3.0.0；Ubuntu 25.04 AppArmor local override 部署文档 | F1 基建 / F4 基建 / F7 基建 | **M** | — | 与 P2 并行 |
| **P2** | **控制面数据模型 + Entry API 扩展** | `claude_accounts.persistent_volume_name` migration；`HostActionRequest` 新增 `ClaudeAccountID` + `Volumes []VolumeMount`；Entry API 返回 `image_version` + `supports_mutagen` + `supports_mergerfs` + `claude_account_id`（向后兼容） | F7 控制面 / F1+F2 握手字段 | **S–M** | — | 与 P1 并行 |
| **P3** | **CLI 三层文件映射重构** | cloud-claude 拆分 `mount_strategy.go` + `mount_mutagen.go` + `mount_sshfs.go`（旧 `mount.go` 重命名）+ `mount_merge.go`；实现 `--mount-mode` 降级状态机；Mutagen 白名单 + ignore 规则；并发启动（Mutagen ‖ sshfs） | F1 / F2 | **L** | P1, P2 | — |
| **P4** | **SSH 会话可靠性 + tmux 包装 + 多端** | `session.go`（tmux attach/new/conflict 决策）；SSH `KeepAlive` + 重连退避；`--new-session` / `--take-over` flag；多端 banner 中文提示；账号级 Mutagen 单例锁 | F3 / F4 / F5 | **M** | P3（复用 CLI 架构） | 与 P5 并行 |
| **P5** | **Claude Code 状态持久化（CLI + 镜像 symlink + admin GC）** | entrypoint symlink `/var/lib/claude-persist` ↔ `~/.claude`、`~/.cache/claude`；Worker `docker volume create` 幂等；admin DELETE handler 事务联动 `volume rm`；可选 admin host 详情页加 `volume_name` 展示 | F7 完整闭环 | **S–M** | P1, P2 | 与 P4 并行 |
| **P6** | **cloud-claude doctor v3 + 错误码统一** | `doctor` 5 维度子命令 + `--fix` + `--json`；错误码常量 + 中文文案；新架构所有错误路径接入 `<DOMAIN>_<KIND>_<NUM>`；`cloud-claude explain` 子命令 | F6 / F8 | **M** | P3, P4, P5 | 最后做 |
| **P7** | **E2E 稳定化 + 性能验收** | `rg`/`ls -R` 10k 文件基准测试；拔网 10s/30s/2min 三档抖动 UAT；首连 ≤ 8s 验收；APFS case-insensitive 真机；Ubuntu 25.04 AppArmor 真机；image 大小 ≤ 700MB CI gate；运维手册更新 | 验收 | **S–M** | P1–P6 | 最后做 |

**合并选项（如压到 4 个 phase）：** P1+P2 合并为"v3 基建" / P4+P5 合并为"会话与持久化" / P6+P7 合并为"收尾"。
**不建议合并：** P3 必须独立——三层 mount + 降级路径是 v3.0 最大技术风险点。

### 4.2 依赖拓扑图

```
P1 (镜像+Worker 基建) ──┐
                        ├─► P3 (CLI 文件映射重构) ──┐
P2 (控制面+Entry API) ──┘                          ├─► P4 (会话可靠性 + tmux + 多端)  ──┐
                        ├──────────────────────────┤                                    ├─► P6 (doctor + 错误码) ─► P7 (E2E)
                        └─► P5 (Claude 持久化闭环) ─┘                                    │
                                                                                       │
        F8 错误码 = 横切关注点（每个 phase 内同步落码，不单独成 phase）
```

### 4.3 v2.0 → v3.0 关键并发点

CLI 启动管线必须在 SSH 握手之后引入**唯一一处并发**：`Mutagen sync create ‖ sshfs mount`，二者对应不同目录（`/mnt/hot` vs `/mnt/cold`），独立 SSH channel 互不阻塞。其它阶段保持串行（详见 ARCHITECTURE §2.2）：

```
T=0    cloud-claude
T+0.5s Entry API ready（status + image_version）
T+1.2s SSH 握手 ready
       ┌─────────────────┬──────────────────┐
T+1.3s sshfs mount       mutagen create     （并发）
T+2.5s mountpoint -q     daemon Watching
       └─────────────────┴──────────────────┘
T+2.6s mergerfs 校验
T+2.8s tmux has-session ? attach : new
T+3s   PTY raw + 进入 claude 交互
```

验收基线 ≤ 8s 在此时序下可达；最大风险是 Mutagen 首轮全量扫描大仓库时间，**白名单严格性是关键**。

### 4.4 不需要扩展 host-agent 的能力（重要边界结论）

- F4（tmux）/ F5（多端 attach）：完全在 CLI ↔ 容器 SSH 层解决，host-agent **不参与**。
- F3（SSH 长心跳）：完全在 cloud-claude `ssh.ClientConfig` + `sshd_config`，host-agent **不参与**。
- F6（doctor）：完全本地 + SSH `mountpoint -q` / `mutagen sync list` / `df` / `pgrep mergerfs`，**不给 host-agent 加 endpoint**。
- host-agent 只需扩展：`Volumes []VolumeMount` 解析 + `docker volume create` 幂等（F7）。

---

## 5. TOP 10 Critical Pitfalls（来自 PITFALLS.md，标注必须在哪个 Phase 防御）

> Roadmapper 必须把 C1/C2/C3/C5/C6 作为 P1/P3 第一 wave 的必做事项；C4/C7/C8 在 P3/P4/P6 防御；M13 是 v3.0 体验生死线必须在 P3+P6 严守。

| # | Pitfall | 触发后症状 | 防御 Phase | 验证手段 |
|---|---------|-----------|-----------|----------|
| **1 (C1)** | mergerfs 默认串行 readdir，10k 文件 `ls -R` >90s | "v3.0 比 v2.0 还慢" | P1（镜像 mount 参数）+ P3（CLI 校验）+ P7 UAT | `mount \| grep mergerfs` 必须含 `func.readdir=cor:4,cache.attr=30,cache.entry=30,cache.readdir=true` |
| **2 (C2)** | mergerfs 2.41 默认 `category.create=pfrd`，新文件随机落 branch | 写入误中 sshfs 冷分支，Mutagen 反向冲突堆积 | P1（entrypoint 显式参数）+ P6（doctor 断言） | `getfattr -n user.mergerfs.branches /workspace/.mergerfs` 必须返回 `RW/NC/RO` 三 branch |
| **3 (C3)** | sshfs 抖动级联 mergerfs 整体挂死，Ctrl-C 也救不回 | 用户终端永久 hang | P1（sshfs 参数）+ P3（监控降级）+ P4 | sshfs 必须含 `reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10`；UAT 拔网 30s 无挂起 |
| **4 (C4)** | Mutagen client/agent patch 版本差导致 `server magic number incorrect` | 全部 sync session 起不来 | P3（CLI 启动比对版本）+ P6 doctor | cloud-claude 启动调 `mutagen version` 与容器内 `/etc/cloud-claude/mutagen.version` 比对，不一致直接降级到 sshfs-only + 错误码 `MOUNT_MUTAGEN_VERSION_SKEW` |
| **5 (C5)** | non-root 容器用户 + Mutagen root 默认导致首次同步**反向清空本地目录** | 数据丢失（HIGH 恢复成本） | P1（Dockerfile 预建目录 + chown）+ P3（启动检测 alpha 非空）+ P5 | session 创建强制 `--default-owner-beta=id:1000 --default-group-beta=id:1000 --mode=two-way-resolved`；alpha 空 + beta 非空时拒绝并报错 `MOUNT_MUTAGEN_SAFETY_GUARD` |
| **6 (C6)** | Ubuntu 25.04 AppArmor 默认禁止嵌套 FUSE，`--security-opt apparmor=unconfined` 也无效 | 全部 mount 失败 | P1（host-preflight + 部署文档）+ P6 doctor | `host-preflight.sh` 检测后要求部署 `/etc/apparmor.d/local/fusermount3` 含 `capability dac_override,` |
| **7 (C7)** | systemd-logind `KillUserProcesses=yes` 在 SSH 登出时杀 tmux server | F4 承诺破产，所有运行进程丢失 | P1（容器不跑 systemd）+ P4（tini PID 1 守护） | UAT：`ssh container 'tmux new -d -s test; sleep 1; pkill -SIGHUP sshd'` 后重连 `tmux attach -t test` 必须成功 |
| **8 (C8)** | v3.0 新错误码与 v2.0 已有 7 个码命名空间冲突 | 外部脚本无法稳定解析，admin 排障靠猜 | P6（统一编码）+ 所有 phase 内同步落码 | CI 单元测试遍历错误码注册表断言无重复 + 每条有中文消息 + 每条有 `next_action` |
| **9 (M13)** | F2 静默降级到 sshfs-only，用户以为在 full 模式下跑 | "v3.0 性能和 v2.0 没差" 群里炸锅 | P3（降级时 stderr 中文 + 错误码）+ P6（doctor 第一屏展示降级历史）+ P8 状态命令 | UAT：强 kill 容器内 mutagen-agent，cloud-claude 必须 5s 内 stderr 中文提示 + 退出码非 0 |
| **10 (M14)** | doctor 仅输出 PASS/FAIL 不给修复命令 | 用户看到红色 FAIL 即放弃 | P6（每条 FAIL 强制带 Suggestion 字段） | CI：`grep -L "Suggestion:" doctor-output.txt` 应无命中 |

**完整 25+ pitfall 清单与 Phase 映射见 PITFALLS.md `Pitfall-to-Phase Mapping` 表。**

### 5.1 还需特别防御的 Moderate Pitfall（罗列编号供 planner 拉清单）

- M1 mergerfs noforget + inode 爆内存 → P1 锁 `inodecalc=path-hash` + 不加 `noforget`
- M2 mergerfs 2.40.x 高并发 segfault → P1 锁版本 ≥ 2.41.0
- M3 Debian 源 mergerfs 太旧（2.33.5）→ P1 必须用 GitHub static .deb，**禁止 apt install**
- M4 entrypoint 顺序错乱（sshd 先于 mount 起来）→ P1 串行 `prepare-fuse → chown → mutagen-agent → mergerfs → wait → exec sshd`
- M5 macOS APFS case-insensitive 双向冲突 → P3 cloud-claude 启动检测 + 强制 `--mode=two-way-resolved`
- M7 多端 tmux 尺寸 jitter → P1 `/etc/tmux.conf` 写 `window-size latest` + `aggressive-resize on`
- M8 Claude Code 在 tmux 内颜色变灰 → P1 `CLAUDE_CODE_TMUX_TRUECOLOR=1` + `terminal-overrides ",*:RGB"`
- M12 OpenSSH `MaxSessions=10` 默认 → P1 `sshd_config` 调 `MaxSessions=30 MaxStartups=60:30:120`
- M16 Docker named volume 残留撑爆磁盘 → P5 admin DELETE handler 事务联动 `volume rm`
- M17 named volume UID 不一致 → P1 Dockerfile 预建 `/home/claude/.claude` 并 chown
- M18 镜像膨胀 > 800MB → P1 CI gate ≤ 700MB + BuildKit cache mount + `--no-install-recommends`

---

## 6. Open Questions（研究中发现的、需要 plan-phase 决策的事项）

> 研究尚未给出唯一答案，必须在对应 Phase 的 `discuss-phase` 或 `plan-phase` 阶段决策。**不要假装已经解决**。

| # | 议题 | 涉及 Phase | 候选方案 | 决策建议 |
|---|------|-----------|----------|----------|
| Q1 | Mutagen 客户端二进制如何分发 | P3 | (a) cloud-claude `go:embed` 整个二进制（推荐，体验一致）/ (b) 检测本机是否已装 + 提示用户 brew install / (c) 首次运行自动下载 | STACK §1 倾向 (a) "embed 静态调用，避免用户机器有无的差异" |
| Q2 | Mutagen daemon 谁负责生命周期 | P3 | (a) cloud-claude 启动时 `mutagen daemon start` 退出时不停（长期复用）/ (b) 每次 cloud-claude session 起停同步 daemon | STACK §1 倾向 (a)，但需考虑多 cloud-claude 并发场景下 daemon 锁 |
| Q3 | Mutagen 同步 mode 默认值 | P3 | `two-way-safe`（最保守，冲突堆积需人工）/ `two-way-resolved`（本地优先自动覆盖远端） | FEATURES F1 anti-feature A2 倾向 `two-way-safe`；STACK §1 配置示例用 `two-way-resolved`；**两者矛盾，必须在 P3 discuss 阶段定调** |
| Q4 | persistent volume 命名规范 | P2/P5 | `claude-state-{account_id}`（STACK §6）/ `claude-creds-{account_id}` + `claude-cache-{account_id}` 双 volume（FEATURES F7）/ `ccp_claude_<account_id>_home` + `_cache`（PITFALLS M16） | 三处不一致，建议 P2 migration 前定稿；倾向 STACK 单 volume 方案（运维更简单） |
| Q5 | Entry API 扩展字段 vs 新增 endpoint | P2 | (a) 在现有 `/v1/entry/{id}/auth` 响应里加 `image_version/supports_*`（向后兼容，ARCHITECTURE §5）/ (b) 新增 `/v1/entry/{id}/capabilities` endpoint | (a) 无破坏性，倾向采纳 |
| Q6 | host-agent 是否扩展 `ContainerStatusResponse` 返回 image labels | P2/P5 | 扩展（doctor 可走 SSH 之外的途径查 image label）/ 不扩展（doctor 全走 SSH，与 ARCHITECTURE §6 一致） | 倾向不扩展，保持边界 |
| Q7 | CLI 启动时是否要做"alpha 非空 + beta 非空 + 内容差异巨大"的安全门 | P3 | 是（首次同步前强制双向 diff 摘要展示，让用户确认）/ 否（信任 Mutagen safety mode） | PITFALLS C5 倾向"是"，但会增加首连时间成本，与 ≤8s 基线冲突 |
| Q8 | 多端共享 tmux session 的命名是 per-user 还是 per-claude_account | P4/P5 | per-user（FEATURES `claude-<user>`）/ per-claude_account（与 volume 一致） | 与 F7 隔离粒度对齐；倾向 per-claude_account |
| Q9 | doctor `--fix` 自动修复操作的幂等性边界 | P6 | 全幂等（重启 mutagen / remount sshfs / 清 known_hosts）/ 部分需要二次确认（清 mutagen 残留 session） | 默认幂等；二次确认走 stdin `y/N`，CI 模式下 `--yes` 跳过 |
| Q10 | mergerfs branch 是 2 路（hot + cold）还是 3 路（hot + cold + overlay 本地覆盖） | P1/P3 | STACK §2 用 2 路 `RW:NC,RO` / PITFALLS C2 提到 "本地覆盖分支" 暗示 3 路 | 倾向 2 路简化，P3 discuss 阶段确认是否需要"用户在容器内手 patch 不上传回本地"的语义 |

---

## 7. Out-of-Scope 强化清单（"看似该做但其实不做"）

> 直接转写到 REQUIREMENTS.md `Out of Scope` 章节即可，附 FEATURES.md "Anti-Features" 编号。

| # | 不做的功能 | 涉及 F | 不做的硬理由（一句话） |
|---|-----------|--------|----------------------|
| A1 | Web UI 显示同步进度条 | F1 | cloud-claude 是 CLI 工具，引入 Web 控制面会放大网关认证/鉴权复杂度；用终端 banner + `cloud-claude status` 替代 |
| A2 | 暴露 Mutagen 5 种冲突解决模式给用户 | F1 | 99% 场景 `two-way-safe` 够用；开放选项反而误配置（Issue #533） |
| A3 | 用 bcachefs / 升级 OverlayFS 替代 mergerfs | F1 | OverlayFS 不支持 FUSE-on-FUSE；mergerfs 是当下唯一稳定方案 |
| A4 | `--mount-mode=none`（完全不挂载本地） | F2 | 等于回到原生 ssh，破坏 cloud-claude 核心承诺 |
| A5 | 运行时自动"升级"mount mode | F2 | 动态改挂载点会让进行中的 claude 进程看不到文件 |
| A6 | 用 UDP / Mosh 协议替代 SSH | F3 | 与 sing-box tun + nftables 默认拒绝模型不兼容（PITFALLS M10） |
| A7 | 断网自动杀掉 claude 进程 | F3/F4 | VS Code Remote 的踩坑（Issue #274774），用户会丢失未保存工作 |
| A8 | 会话永不过期 | F4 | 违背容器生命周期契约，会给后续资源回收埋雷 |
| A9 | 默认独占（新端踢旧端） | F5 | 违反"两个屏都想看"用户直觉；要独占必须显式 `--take-over` |
| A10 | 实时协作光标（VS Code Live Share 风格） | F5 | 超出 CLI 能力范围，tmux 做不到 |
| A11 | doctor 自动上报"诊断报告"到服务端 | F6 | 数据脱敏 / 合规风险，v3.0 不开此坑 |
| A12 | doctor 自动改用户本地 SSH config | F6 | 不可逆；VS Code Remote (Issue #8910) 大量抱怨 |
| A13 | 用 `ANTHROPIC_API_KEY` 替代 OAuth | F7 | 官方明确互斥（Issue #5767），混用会强制降级到按量付费 |
| A14 | 容器内做 `claude login` 交互 OAuth | F7 | 全隧道出网会打断 OAuth 回调 |
| A15 | 错误用 emoji 提示替代 ASCII 符号 | F8 | 终端 emoji 宽度 / CI 日志兼容性差 |
| A16 | 多宿主机编排 / 集群调度 | 全局 | 沿用 v1 single-host 约束，不在 v3.0 范围 |
| A17 | 用户预热 / 空闲回收策略 | 全局 | PROJECT.md 明确推迟到 v3.1（涉及控制面资源调度） |
| A18 | 性能 metrics 实时上报到 admin 后台 | 全局 | PROJECT.md 明确推迟到 v3.1（依赖 v3.0 稳定后） |
| A19 | admin 后台新增 mount mode / session 数管理页 | F4/F5 | ARCHITECTURE §5 明确 v3.0 不做新页面，最多在 host 详情页加 `image_version` 一行 |
| A20 | 新增"session 管理"REST endpoint | F4/F5 | CLI 完全在 SSH 层解决多端协作，不需服务端介入 |

---

## 8. Confidence Assessment

| 维度 | Confidence | 依据 |
|------|------------|------|
| Stack | **HIGH** | 所有版本号来自 GitHub release 直接核实；mergerfs 2.41 默认值变更与 mosh UDP 不兼容两条最关键风险已交叉验证 |
| Features | **HIGH** | 多端 attach UX、doctor 范式、Claude Code credentials 契约 3 个最关键决策均有 ≥3 个独立来源（tmux/tmate/ET、flutter/brew/npm、Anthropic 官方 + devcontainer 生态）|
| Architecture | **HIGH** | v2.0 全部代码路径源码级验证（`cmd/cloud-claude/main.go`、`internal/cloudclaude/*.go`、`internal/runtime/tasks/worker.go`、`deploy/docker/managed-user/Dockerfile` 等）；Mutagen / mergerfs 行为基于官方文档 |
| Pitfalls | **HIGH** | 25+ pitfall 全部有公开 GitHub issue、官方文档或维护者声明；`.planning/RETROSPECTIVE.md` v1.0/v1.1/v2.0 教训直接复用 |
| 弱网阈值数值 | **MEDIUM** | Mosh 官方未给明确秒数；30s 阈值是 conntrack UDP 默认值（基础设施层共识），不是产品官方数字 |
| mergerfs 替代方案对比 | **MEDIUM** | 基于官方文档 + 社区共识；无直接 "对比 bcachefs" 的 benchmark 数据 |

**Overall confidence: HIGH** — 可直接进入 roadmap → REQUIREMENTS → plan-phase。

### 8.1 需要在 plan-phase 验证的潜在 gap

1. **Mutagen 首轮全量同步 ≤ 8s 是否在中等仓库（5k 文件 / 200MB）真机可达** — 必须在 P3 完成后立刻 P7 真机测，否则需调整白名单策略或松开 ≤ 8s 验收。
2. **mergerfs `func.readdir=cor:4` 在容器并发场景下的内存峰值** — STACK §2 引用的 trapexit 建议是单机 NAS 场景，多容器并发未验证。
3. **Ubuntu 25.04 AppArmor local override 在生产宿主上是否有 SELinux 类似的"忘了 reload"问题** — 部署脚本必须验证 `apparmor_parser -r` 真正生效。
4. **多端共享 tmux 的 PTY 尺寸抖动是否会影响 Claude Code 流式 TUI 渲染** — 需 P4 真机测两端不同分辨率长会话。
5. **tmux 3.6a 在 Ubuntu 24.04 noble apt 仓位是否已 GA**（STACK §4 假设 apt 直装）— 若未 GA 需切换为镜像 build 时编译。

---

## 9. Sources（按 confidence 分层；所有 URL 已在 4 份子文档保留原文）

### Primary（HIGH，官方 release / 官方文档）

- Mutagen：<https://github.com/mutagen-io/mutagen>、<https://mutagen.io/documentation/synchronization/>、<https://mutagen.io/documentation/transports/ssh/>、<https://mutagen.io/documentation/synchronization/ignores/>
- mergerfs：<https://github.com/trapexit/mergerfs/releases/tag/2.41.0>、<https://trapexit.github.io/mergerfs/latest/config/options>、<https://trapexit.github.io/mergerfs/latest/config/functions_categories_policies>、<https://trapexit.github.io/mergerfs/latest/remote_filesystems/>、<https://github.com/trapexit/mergerfs/discussions/1571>
- sshfs：<https://github.com/libfuse/sshfs/releases/tag/sshfs-3.7.5>、<https://deepwiki.com/libfuse/sshfs/4.3-performance-tuning>
- tmux：<https://github.com/tmux/tmux/releases/tag/3.6a>、<https://github.com/tmux/tmux/releases/tag/3.6>
- Claude Code 持久化：<https://github.com/anthropics/claude-code/issues/1736>、<https://code.claude.com/docs/en/env-vars>
- VS Code Remote：<https://github.com/microsoft/vscode/issues/280450>（reconnectionGraceTime 3h）

### Secondary（MEDIUM，社区共识 / 多源交叉验证）

- mergerfs pitfalls：<https://github.com/trapexit/mergerfs/issues/893>、<https://github.com/trapexit/mergerfs/issues/1468>、<https://github.com/trapexit/mergerfs/issues/869>
- Mutagen pitfalls：<https://github.com/mutagen-io/mutagen/issues/46>、<https://github.com/mutagen-io/mutagen/issues/224>、<https://github.com/mutagen-io/mutagen/issues/247>、<https://github.com/mutagen-io/mutagen/issues/267>、<https://github.com/mutagen-io/mutagen/issues/275>、<https://github.com/mutagen-io/mutagen/issues/531>、<https://github.com/mutagen-io/mutagen/issues/533>
- sshfs reconnect：<https://github.com/libfuse/sshfs/issues/3>、<https://serverfault.com/questions/6709/sshfs-mount-that-survives-disconnect>
- Ubuntu 25.04 + FUSE：<https://github.com/moby/moby/issues/50013>、<https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105>
- tmux + systemd：<https://unix.stackexchange.com/questions/171503/tmux-session-killed-when-disconnecting-from-ssh>、<https://github.com/systemd/systemd/issues/14497>
- Claude Code 颜色：<https://github.com/anthropics/claude-code/issues/35148>、<https://github.com/anthropics/claude-code/issues/46146>
- Mosh / autossh / ET 不可用证据：<https://github.com/mobile-shell/mosh/releases>、<https://oneuptime.com/blog/post/2026-03-20-mosh-ipv6-configuration/view>、<https://github.com/MisterTea/EternalTerminal>
- Docker volume 权限 / 清理：<https://selfhosting.sh/foundations/docker-volume-permissions/>、<https://oneuptime.com/blog/post/2026-02-08-how-to-clean-up-orphaned-docker-volumes/view>
- CLI 错误设计范式：<https://jmmv.dev/2013/08/cli-design-error-reporting.html>、<https://www.grizzlypeaksoftware.com/library/cli-error-handling-and-user-friendly-messages-qgugu9kg>

### 项目内部（HIGH）

- `.planning/PROJECT.md`（v2.0 已交付能力 + v3.0 性能基线 + Out of Scope）
- `.planning/MILESTONES.md`（v2.0 完成详情）
- `.planning/RETROSPECTIVE.md`（v1.0 错误码粗粒度 / v1.1 对称设计 / v2.0 AppArmor + FUSE 教训）
- v2.0 真实代码路径（ARCHITECTURE §1 已列出 12 处关键文件 + 行号）

---

## 10. 与下游 agent 的对接清单

**给 gsd-roadmapper：**
- 直接采用 §4.1 的 7 phase 切分；Depends on 矩阵已就绪。
- 每个 phase 的 `goal` 段必须引用对应的 §3 REQ-ID + §5 Critical Pitfall 编号。
- 在 phase 7（E2E）的 success criteria 写入 PROJECT.md 三条性能基线。

**给 REQUIREMENTS drafter：**
- §3 的 24 条 REQ-F*-* 直接转写为 active requirements。
- §7 的 20 条 Out-of-Scope 直接追加到 PROJECT.md `### Out of Scope` 章节。
- §6 的 10 个 Open Question 全部收录为 `### Open Questions` 子章节，标注 "to be resolved in plan-phase"。

**给 gsd-planner：**
- 每个 phase 的 PLAN.md `tasks` 段为对应的 Critical Pitfall（见 §5 Phase 列）创建独立"防御任务"，单独 commit + 独立 UAT。
- 每个 phase 的 PLAN.md `verification` 段直接抄 §5 表格里的"验证手段"列。
- §6 的 Open Question 在对应 phase 的 `discuss-phase` 必须 surface 出来让用户拍板。

---

*Researched: 2026-04-18*
*Synthesized by: gsd-research-synthesizer*
*Confidence: HIGH*
*Ready for: roadmap → REQUIREMENTS → plan-phase*
