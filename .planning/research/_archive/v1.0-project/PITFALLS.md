# Pitfalls Research

**Domain:** cloud-claude 透明远程 CLI + 本地目录到远端容器实时映射（在现有 SSH Proxy、sing-box tun + nftables、`--network=none` 受管容器上叠加）
**Researched:** 2026-04-15
**Confidence:** MEDIUM（核心约束来自本项目 `PROJECT.md` 与公开文档/社区报告；部分为场景归纳，需在集成验证中落地）

## Critical Pitfalls

### Pitfall 1: 在 `--network=none` 的容器里假设「能连上用户本机」或「能随意开第二条 TCP」

**What goes wrong:**
目录映射若在**容器内**发起 `sshfs`/额外 SSH/Mutagen agent，却未显式设计**控制面注入的可达路径**（例如宿主机侧 Unix socket、已打通的 `host-gateway`、或经 Proxy 的单一信令通道），会出现挂载超时、间歇性断连，或误把同步流量绕到错误接口。项目已采用 `--network=none` 从根上砍掉 Docker 默认网卡旁路；若再叠一层「容器内自连外网」的假设，会与该约束直接冲突。

**Why it happens:**
开发者在笔记本上调试时习惯「容器能访问宿主机 `host.docker.internal`」；在 Linux 裸机 Docker 与本项目「无默认网络栈」模型下不成立。文件同步方案文档常按「有 IP 可达」写，未区分**信令/数据平面**。

**How to avoid:**
- 在设计阶段固定「映射数据走哪条 pipe」：仅 SSH 子系统、反向转发、`cloud-claude`↔Proxy 已有连接复用，或宿主机 host-agent 显式开的 socket；禁止依赖容器内自发现路由。
- 若采用 Mutagen 等需在远端跑 agent 的方案，明确 agent 的启动方式与网络可达性由**谁**保证（宿主机注入 vs 用户态转发），并写清与 sing-box/nftables 的边界。

**Warning signs:**
挂载命令长时间卡在握手；仅在「有默认 bridge」的环境能复现成功；抓包发现同步尝试走非预期接口。

**Phase to address:**
**目录映射方案选型与设计评审**（在写任何挂载代码之前）

---

### Pitfall 2: Docker 内 FUSE/sshfs 权限与发行版安全模块「看起来加了 `--device /dev/fuse` 仍失败」

**What goes wrong:**
镜像里装了 `sshfs`，运行时仍出现 `Permission denied`、`fusermount` 失败、或仅在部分宿主机（常见为启用 AppArmor 的 Ubuntu）上失败。团队误以为「`--cap-add SYS_ADMIN` + `/dev/fuse`」永远足够。

**Why it happens:**
除 `CAP_SYS_ADMIN` 与 `/dev/fuse` 外，默认 Docker 的 **AppArmor/seccomp** 可能拦截 mount 相关行为；不同内核与 moby 版本对 FUSE 的宽松策略不一致（社区长期讨论见 docker/for-linux、moby issue）。生产宿主机与开发机策略不一致时问题延后暴露。

**How to avoid:**
- 在**目标发行版**上用最小复现容器验证 FUSE（记录是否需 `security-opt`、是否仅限特定内核版本）。
- 将「允许的挂载选项、是否 `allow_other`、是否依赖 `/etc/fuse.conf` 的 `user_allow_other`」写进镜像与运行契约，并在 CI 用同款内核/安全模块矩阵做一次。

**Warning signs:**
本地 macOS Docker Desktop 正常，Linux 裸机失败；`dmesg`/audit 中出现 mount 被拒；仅升级 Docker 后行为变化。

**Phase to address:**
**受管镜像与容器创建参数硬化**（与 host-agent 创建逻辑同一阶段）

---

### Pitfall 3: sshfs 的 UID/GID、`allow_other` 与现有 `/workspace` bind 模型混用导致「能跑 claude 但写坏权限」

**What goes wrong:**
已有设计将宿主机 `homeDir` bind 到容器 `/workspace`。若再叠 sshfs 到同一路径或子路径，可能出现**双源写入**、权限位不一致、或容器内进程 UID 与 FUSE `user_id`/`default_permissions` 不一致，表现为偶发 `Permission denied`、可执行位丢失、或 `claude`/git 在远端创建的文件回到本机后属主错乱。

**Why it happens:**
sshfs 对权限的语义依赖挂载选项与 OpenSSH 服务端配置；FUSE 与 bind mount 的叠加顺序容易被忽略；「实时映射」需求压力下容易选「先挂载能用再调权限」。

**How to avoid:**
- 明确**单一写路径**：要么以 bind 为主、同步为辅，要么以同步工具为权威，避免两处同时作为「真相」。
- 为 sshfs 固定一组挂载选项（如 `default_permissions`、`uidfile`、`gid` 等）并与镜像内运行用户对齐；在文档中列出**禁止**的选项组合。
- 用自动化用例验证：创建文件、chmod、git clone、可执行脚本、符号链接。

**Warning signs:**
仅部分文件权限错误；本地 `ls -l` 与容器内不一致；CI 与用户机器行为不一致。

**Phase to address:**
**目录映射与权限集成测试**（与 E2E 同一阶段，早于「体验打磨」）

---

### Pitfall 4: SSH 多路复用（ControlMaster）与「僵死 master」——文件通道与交互会话抢同一条命

**What goes wrong:**
为加速连接启用 `ControlMaster`/`ControlPersist` 后，网络闪断或宿主机休眠后出现 **mux 客户端挂死**、`Broken pipe`、`mux_client_request_session` 超时；或 **MaxSessions** 达到上限导致新会话（含 sftp/子通道）被拒。若 `cloud-claude` 与 sshfs/Mutagen 复用连接，单点故障会同时拖垮 TTY 与目录同步。

**Why it happens:**
多路复用把多条逻辑会话绑在同一 TCP 上；master 进程异常退出后控制套接字残留；OpenSSH 服务端默认 `MaxSessions` 较小；部分环境上控制套接字放在 **overlayfs** 上会与 Unix domain socket 行为不睦（社区常用改 `ControlPath` 到 `shm` 等路径缓解）。

**How to avoid:**
- 为「交互 TTY」与「大流量/长驻同步」设计**连接策略**：共享 master 或独立 TCP，二选一写清；若共享，必须配置 **ServerAliveInterval/CountMax**、控制 `ControlPersist`、并实现**陈旧 socket 检测与强制重建**（如超时后 `ssh -O exit` 等价逻辑）。
- 若自建 Proxy（非本机 OpenSSH 客户端）：在 Go 侧实现等价策略，避免假设 OpenSSH 已替你处理所有 mux 边界。
- 服务端按需调高 `MaxSessions` 并理解**仅新连接**生效，旧 master 仍可能占满旧限制（参见常见 mux 文章与 OpenSSH 行为）。

**Warning signs:**
偶发「第二次打开必卡」；断网恢复后必须删 socket 才能恢复；并发 `git`/同步时突增会话失败。

**Phase to address:**
**连接与会话管理设计**（`cloud-claude` 核心实现阶段）

---

### Pitfall 5: 实时双向同步的「一致性幻觉」——git 切换、容器重建与冲突队列爆炸

**What goes wrong:**
用户 `git checkout`、本地批量格式化或容器侧构建产物写入后，出现**反向覆盖**、幽灵冲突、或「列表里全是 unresolved conflict」。产品看起来像实时，实则**合并语义**与**会话生命周期**未与容器生命周期对齐。

**Why it happens:**
双向同步类工具（如 Mutagen）用三向合并；若会话 ancestor 与真实文件树脱节（容器重建、volume 清空、会话未重置），两侧会同时表现为「新建」，冲突激增。社区 issue 中常见「git checkout 后从 beta 拉回」类报告，根因常是**会话与目标生命周期不匹配**。

**How to avoid:**
- 为「容器重建 / 用户主机重启 / 会话 reset」定义明确状态机：何时 `flush`、何时重建同步会话、是否默认忽略 `.git`/`node_modules` 等。
- 选定默认同步模式（如以本地为权威 vs 真正双向）并在 CLI 暴露**可理解的冲突诊断**（路径、建议操作），避免 silent data loss。

**Warning signs:**
小团队单机难复现；用户切换分支后目录「回滚」；`sync list` 中长期堆积冲突。

**Phase to address:**
**同步语义与运维可观测性**（与 `cloud-claude` 配置模型同一阶段）

---

### Pitfall 6: TTY/信号/退出码「像 ssh」与「像本地 claude」之间的缺口

**What goes wrong:**
未分配 PTY 时 **Signal 转发行为与 OpenSSH 服务端策略不一致**；未处理 **SIGWINCH** 导致全屏 TUI、进度条、表格错位；raw mode 未在异常路径恢复，终端留在乱码状态。用户感知为「偶尔 Ctrl+C 停不下来」「resize 后界面坏掉」「退出后 shell 坏了」。

**Why it happens:**
`golang.org/x/crypto/ssh` 提供 `RequestPty`、`WindowChange` 等，但**调用方**必须接本地 `SIGWINCH`、在退出路径 `term.Restore`；信号与 PTY、stderr 合流等行为需与远端 `sshd` 配置对齐。部分 issue 讨论无 PTY 时 signal 语义受限。

**How to avoid:**
- 明确 **Claude Code 是否必须 PTY**；若必须，接受 stderr 合流等限制或设计替代 IPC。
- 实现：监听 `SIGWINCH` → `session.WindowChange`；统一 defer 恢复终端；对退出码从 `Session.Wait()` 取远端状态并映射到本地 `os.Exit`。
- **跨平台：** Windows 上无 `SIGWINCH`，需降级策略（固定尺寸或控制台 API），避免 Linux 专用逻辑静默失败。

**Warning signs:**
仅交互场景失败、脚本模式正常；resize 必现；特定 `sshd` 版本上 signal 异常。

**Phase to address:**
**TTY 与信号透传专项**（`cloud-claude` 与 Proxy 联调阶段）

---

### Pitfall 7: 网络隔离策略与「文件通道」竞争——误把同步当一般出网流量

**What goes wrong:**
团队尝试让容器内工具经 **sing-box tun** 访问「用户笔记本」或任意公网 endpoint 做同步，与产品「受控出口 IP」叙事混淆；或 nftables 规则误伤 **宿主机注入通道**（例如到 host-agent 的 unix socket 或已允许的转发），表现为间歇性同步失败而 SSH 交互仍存活。

**Why it happens:**
出网门禁与「到控制面的可信路径」是两条故事线；若未在架构图上标出**文件数据平面**，容易在加规则时一刀切。

**How to avoid:**
- 在架构文档中单独画 **文件/同步平面**：与 tun 出口的关系（正交 / 复用 / 显式例外）。
- 变更 nftables/sing-box 规则时增加回归：**仅验证**「代理出口 IP」不够，须加「映射通道可用」探测。

**Warning signs:**
升级网络模块后映射才坏；与出口测试 API 结果无相关性却仍失败。

**Phase to address:**
**网络与映射集成回归**（任何 RoutingProvider/防火墙变更的 gate）

---

### Pitfall 8: 跨平台（macOS vs Linux）——Docker、文件系统语义与监视能力差异

**What goes wrong:**
- **Docker Desktop（macOS）** 与 **Linux 裸机**：路径、VM 边界、性能与 inotify 行为不同；同一同步工具在 mac 上「能跑」在 Linux CI 暴露事件丢失或超高延迟。
- **大小写 / 符号链接 / xattr**：Linux 与 macOS 默认文件系统语义不同，双向同步时易出现「一端改名、另一端删建」类冲突。
- **FUSE：** macOS 用户空间与 Linux 容器内 FUSE 不是同一套运维故事；在 Mac 上验证通过不能替代 Linux 宿主上的受管容器验证。

**Why it happens:**
远程开发工具链普遍以「先在 Mac 上好用」为优先级；本项目部署基线是 **Linux 单宿主机**，若验证矩阵缺 Linux，问题会在客户环境爆发。

**How to avoid:**
- 将 **Linux 宿主 + 受管镜像** 作为权威测试平台；macOS 仅作为开发者 CLI 体验测试。
- 增加「大仓库 + 分支切换 + 构建产物目录忽略」类场景；对 ignore 规则与 case-sensitivity 做专项用例。

**Warning signs:**
仅 Mac 复现的 bug；用户报告「Linux 上慢 10 倍」；`.git` 相关诡异冲突。

**Phase to address:**
**跨平台 CI 与发布前 checklist**（Beta 前必须完成）

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| 容器内 sshfs 为赶进度临时 `--privileged` | 最快打通演示 | 安全基线崩塌，与「最小权限」运维承诺冲突 | **永不**；仅允许在隔离开发机上的单次 PoC |
| 复用 OpenSSH mux 但不实现陈旧 socket 清理 | 代码少 | 现场「随机卡死」，排障成本极高 | MVP 可短期存在，但必须配监控与文档化 workaround |
| 双向同步默认不忽略构建产物 | 配置简单 | 冲突与 IO 放大，体验差评 | 可在内测期接受，发布前必须有合理默认 ignore |
| Linux 上不测仅在 Docker Desktop 上测 | 迭代快 | 生产 Linux 翻车 | 内测可，**对外发布前不可** |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| sshfs in Docker | 只加 `/dev/fuse` 不加能力/安全模块 | 按目标宿主机验证 `SYS_ADMIN` + 设备 + 必要时安全 profile；记录矩阵 |
| Mutagen / 类同步 | 会话不随容器生命周期重置 | 容器重建时结束/重建会话或显式 `reset`，避免 ancestor 错乱 |
| golang.org/x/crypto/ssh | 未转发 WINCH / 未恢复 raw 终端 | `RequestPty` + `WindowChange` + defer `Restore`；非 POSIX 平台降级 |
| 现有 SSH Proxy | 假设与 OpenSSH 客户端 mux 行为 1:1 | 明确自建客户端的 keepalive、重连、会话上限策略 |
| sing-box + nftables | 把「同步流量」误当泄漏去封 | 区分控制面/同步平面与 tun 出口；规则变更走联合回归 |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| sshfs + 大量小文件（node_modules） | IO 延迟极高、claude 卡顿 | 忽略目录、改用打包同步、或单向同步构建依赖 | 中型前端仓库即暴露 |
| 多路复用单 TCP 扛大文件与交互 | 互相阻塞、延迟尖刺 | 分离数据连接或限流/排队 | 同步与 TTY 同时高负载 |
| macOS 宿主文件轮询 | CPU 与延迟差 | Linux 为主验证路径；Mac 上降低预期 | 开发者本机「感觉慢」 |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| 为 FUSE 长期放宽 `privileged` 或过大 seccomp | 容器逃逸面增大 | 最小权限迭代；特权范围可审计 |
| 同步通道未鉴权或误绑公网 | 用户目录暴露 | 仅走已认证 SSH/控制面；二次校验路径与身份 |
| 在日志中打印同步路径与密钥材料 | 凭据与隐私泄漏 | 结构化日志脱敏 |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| 冲突时 silent 丢一边 | 用户不信任「透明」 | 明确提示与可重试操作 |
| 断线后终端未恢复 | 认为「坏了」 | 保证 defer 恢复 tty；提示重连命令 |
| 「实时」实际延迟数秒 | 与本地 claude 预期不符 | 产品文案诚实；大仓库提示 |

## "Looks Done But Isn't" Checklist

- [ ] **目录映射：** 是否在「Linux 裸机 Docker + `--network=none` + 当前 nftables/sing-box」下测过，而非仅在 Docker Desktop？
- [ ] **FUSE：** 是否在**启用 AppArmor 的 Ubuntu 类宿主**上测过挂载与卸载（含异常退出后 `fusermount -u`）？
- [ ] **权限：** 容器内 `claude` 创建的文件回到本机后，属主/权限是否与预期一致（含可执行位、符号链接）？
- [ ] **SSH：** 断网、休眠、服务端 `sshd` 重启后，**无需用户手动删 socket** 能否恢复？若不能，是否有明确报错与一键恢复？
- [ ] **同步：** `git checkout`/大规模删除后是否出现反向覆盖或冲突积压？`git status` 在两侧是否一致？
- [ ] **TTY：** resize、Ctrl+C、管道/非交互模式是否与官方 `claude` 行为一致？
- [ ] **网络：** 变更防火墙或 sing-box 出站规则后，映射与出口 IP 测试是否**同时**绿？
- [ ] **跨平台：** macOS 上开发的 `cloud-claude` 是否在 Linux 上做过集成测试？

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| 僵死 mux socket | LOW | 提供等价 `ssh -O exit` 或删已知路径 socket；文档化 |
| FUSE 挂载泄漏（异常退出） | MEDIUM | 容器内 watchdog 清理；重启容器 |
| 同步祖先错乱 | MEDIUM–HIGH | 重置同步会话 + 可选全量对齐；用户确认冲突策略 |
| 终端 raw 未恢复 | LOW | 用户 `reset`；客户端修复 defer |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| `--network=none` 与映射路径 | 目录映射方案设计 | 架构图 + 无默认网卡下的连通性证明 |
| FUSE/安全模块 | 镜像与容器参数硬化 | 多宿主矩阵挂载测试 |
| UID/权限与 bind 混用 | 权限与集成测试 | 自动化权限与 git 用例 |
| SSH mux 稳定性 | 连接与会话管理实现 | 断网/休眠/并发会话压测 |
| 同步一致性 | 同步语义与可观测性 | git/分支/重建容器场景 |
| TTY/信号 | TTY 专项与联调 | 交互脚本、resize、信号对比 |
| 网络策略与文件通道 | 网络变更联合回归 | 规则变更前后映射+出口双测 |
| 跨平台 | CI 与发布 checklist | Linux-first 流水线 |

## Sources

- 项目上下文：`.planning/PROJECT.md`（`--network=none`、sing-box tun、cloud-claude 目标）
- Docker/FUSE：`https://github.com/docker/for-linux/issues/321`、moby 社区关于 `SYS_ADMIN` + `/dev/fuse` 的讨论
- SSH 多路复用：ServerFault「Detecting stuck SSH control master sockets」、`mux_client_request_session` 社区讨论；Thomas Broadley「SSH multiplexing gotchas」（MaxSessions 等）
- 同步语义：Mutagen 官方 synchronization 文档（模式与冲突处理）；Mutagen GitHub issue 中关于会话生命周期与容器重建的讨论
- Go SSH：`golang.org/x/crypto/ssh` `Session.RequestPty` / `WindowChange`；Go issue 中关于无 PTY 时 signal 的讨论
- OpenSSH：版本发行说明（排查特定版本 mux 相关修复时需对照）`https://www.openssh.com/releasenotes.html`

---
*Pitfalls research for: cloud-claude 透明远程 CLI + 目录映射*
*Researched: 2026-04-15*
