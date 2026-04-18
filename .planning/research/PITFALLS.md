# Pitfalls Research — v3.0 远端开发体验升级

**Domain:** 单宿主机容器化 SSH 云主机 + 远程透明 CLI（Mutagen + sshfs + mergerfs 三层文件系统、tmux 会话恢复、FUSE/AppArmor/Docker 叠加）
**Researched:** 2026-04-18
**Confidence:** HIGH（全部来源均为公开 GitHub issue、官方文档、维护者声明或已发布的 blog/SO 答复）

> 本文覆盖 v3.0 8 项 feature（F1–F8）对应的社区真实踩坑。每条 pitfall 均给出症状、根因、预防措施（映射到具体 phase 的测试 / 配置 / 代码审查清单）、来源 URL。
> **阅读顺序建议**：`Critical（会让 v3.0 整体失败）→ Moderate（显著降低体验）→ Minor（需要在文档层对齐预期）`。
> 下游：`gsd-research-synthesizer / gsd-roadmapper / gsd-planner` 应将每条 Critical pitfall 转化为 PLAN 中的必做防御任务或 UAT 用例。

---

## Critical Pitfalls（一旦命中，v3.0 主线失败）

### Pitfall C1：mergerfs + sshfs 组合下 `ls` / `rg` 卡到 90s 以上

**What goes wrong:**
把 Mutagen 热分支 + sshfs 冷分支 + 本地覆盖分支合并到 `/workspace` 以后，Claude Code 或用户执行 `ls -R`、`rg .` 时整个进程挂 90 秒以上；strace 显示 mergerfs 在对 sshfs 分支做串行 `readdir` + `getattr`。用户会直接判定"v3.0 比 v2.0 还慢"。

**Why it happens:**
- mergerfs 的默认 `func.readdir=seq` 会依次遍历每个 branch。
- sshfs 本身每次 `readdir` 都是一个网络 round-trip，高延迟直接拖垮跨 branch 合并。
- 默认 `cache.attr=1` / `cache.entry=1` 几乎等于无缓存，重复调用不会命中。

**How to avoid:**
- 容器启动参数里必须显式设置：`cache.readdir=true,cache.entry=30,cache.attr=30,func.readdir=cosr,cache.statfs=10,dropcacheonclose=true`。
- sshfs 端显式配置 `Ciphers=aes128-gcm@openssh.com,Compression=no`（替换已废弃的 arcfour）、`reconnect,ServerAliveInterval=15,ServerAliveCountMax=3`。
- 验收基线（已在 PROJECT.md 里承诺）`rg .` / `ls -R` 10k 文件 ≤ 本地 1.5×，直接进入 UAT 脚本，在 mergerfs 首次挂载后立即执行一次"冷"跑一次"热"。

**Warning signs:**
- `docker exec <ct> strace -c -p $(pidof mergerfs)` 大量 `readdir` / `getattr`。
- `iftop` 显示 sshfs 通道在 `ls` 时持续灌包。

**Phase to address:** F1 mergerfs 容器镜像 + 挂载参数阶段、F6 doctor mount 维度；UAT 覆盖 10k 文件目录基线。

**Source:**
- <https://github.com/trapexit/mergerfs/issues/893>（"90+ seconds → <5 seconds after cache 参数"，trapexit 亲自确认）
- <https://trapexit.github.io/mergerfs/latest/config/options>（`func.readdir=cosr` 文档）
- <https://github.com/libfuse/sshfs/issues/3>（sshfs hang + `ServerAliveInterval` 机制）

---

### Pitfall C2：mergerfs v2.41 默认 `category.create=pfrd`，新建文件随机落盘

**What goes wrong:**
v2.0 里只有纯 sshfs，用户写入总是落到容器 `/workspace` → 通过 SFTP 回到本地。v3.0 叠加 mergerfs 后如果直接使用 2.41 以上的默认 `pfrd`（按自由空间加权随机）策略，写入可能"误中"sshfs 冷分支或本地覆盖分支，导致：
- 文件出现在用户没预期的位置；
- 热/冷分支出现同名不同内容的文件；
- 后续 Mutagen 发生"alpha 有 beta 无"的冲突，被 safety mode 拦下同步。

**Why it happens:**
mergerfs v2.41.0（2025-11-12 发布）把默认 create 策略从 `epmfs` 改成 `pfrd`——这是 trapexit 为了降低新手支持负担刻意做的，但对严格分层的 v3.0 是反直觉的。

**How to avoid:**
- 三层挂载必须显式 `category.create=ff` 或自定义策略：写入强制落到"本地覆盖分支"，Mutagen 热分支设为 `NC`（no-create），sshfs 分支设为 `RO`（read-only）。branch 定义示例：`/overlay=RW:/hot=NC:/cold=RO`。
- 在 PLAN 里加"branch mode 矩阵表"作为必检查项（code review checklist）。
- CI 跑 `getfattr -n user.mergerfs.category.create /workspace/.mergerfs` 断言拿到的是配置里的策略名。

**Warning signs:**
文件在 `docker exec <ct> ls /hot/foo.txt` 出现，但用户本地 `./foo.txt` 没同步——说明 create 策略把文件写到了错误分支。

**Phase to address:** F1 三层结构定义 phase（必须在设计文档里写清 branch mode 矩阵）、F6 doctor 的 mount 维度补一项 create policy 断言。

**Source:**
- <https://github.com/trapexit/mergerfs/releases/tag/2.41.0>（"create policy default changed from `epmfs` to `pfrd`"）
- <https://github.com/trapexit/mergerfs/discussions/1571>（trapexit 解释改默认的动机）
- <https://trapexit.github.io/mergerfs/latest/config/functions_categories_policies>（branch mode `RO/NC/RW` 官方文档）

---

### Pitfall C3：sshfs 冷分支网络抖动导致整个 mergerfs 挂死

**What goes wrong:**
sshfs 冷 branch 默认没有 `ServerAliveInterval` 时，只要链路临时抖动，所有访问该路径的进程会永久阻塞（包括 `cd`、`ls`、shell 本身），mergerfs 整体也会 hang；`sudo killall -9 sshfs` + `umount` 是唯一解。

**Why it happens:**
sshfs 只在收到 TCP RST 或对端明确断开时才会解除 I/O；如果是 NAT/VPN 中间切换或弱网丢包，ssh 会话挂在 kernel 侧，上层 FUSE 请求被锁住，级联让 mergerfs 也无法完成 `readdir`。

**How to avoid:**
- sshfs 挂载参数强制：`-o reconnect,ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10,ConnectionAttempts=1`。过 45s 未响应时 sshfs 会主动 I/O 错误，mergerfs 可以路由到下一 branch 或标记 F2 自动降级。
- 在 cloud-claude 侧实现"sshfs 超时监控 goroutine"，侦测到 `Transport endpoint is not connected` 则触发 F2 降级到 `mutagen-only`，并给用户明确中文提示。
- UAT 脚本里新增"拔网 30s → 恢复"用例，验证会话、Mutagen、sshfs 三层独立恢复。

**Warning signs:**
`dmesg` 出现 `fuse: request timeout`；用户反馈"终端卡住但 Ctrl-C 也救不回来"。

**Phase to address:** F2 降级策略 phase、F3 SSH 弱网容忍 phase、F6 doctor mount 维度。

**Source:**
- <https://github.com/libfuse/sshfs/issues/3>（"processes waiting on the mount will hang indefinitely"）
- <https://serverfault.com/questions/6709/sshfs-mount-that-survives-disconnect>（`ServerAliveInterval=15,ServerAliveCountMax=3` 是未公开文档但被维护者确认的最佳实践）
- <https://askubuntu.com/questions/791002/how-to-prevent-sshfs-mount-freeze-after-changing-connection-after-suspend>（sshfs-reconnect 脚本方案及局限性）

---

### Pitfall C4：Mutagen agent 协议版本不匹配导致全部 session 崩溃

**What goes wrong:**
用户本地装了 Mutagen 0.18.0，v3.0 镜像里内置了 0.18.1 的 agent，创建 sync session 时抛出 `unable to handshake with agent process: server magic number incorrect`，Mutagen 热分支直接起不来；F2 降级逻辑如果判据不准，会让用户以为"mutagen 永久坏了"。

**Why it happens:**
- Mutagen 的 daemon/agent/CLI 三者协议并非跨版本稳定，上下游版本偏差一个 patch 就可能 handshake 失败（ddev 2025-03 大规模中招）。
- 用户本地通过 brew 自动升级 → 与容器镜像锁定版本分裂。

**How to avoid:**
- cloud-claude 启动时必须调用 `mutagen version` 并与容器内 agent 做版本比较；不一致时**显式**输出中文提示 + 自动降级到 `sshfs-only` + 给出升级命令，而不是 crash。
- 容器镜像 build 时写入 `/etc/cloud-claude/mutagen.version` 元数据；cloud-claude release note 中同步声明要求的本地版本范围。
- 提供 `cloud-claude doctor --fix` 的"一键重装本地 Mutagen"路径（brew / direct download）。

**Warning signs:**
`Error: unable to connect to beta: ... server magic number incorrect`。

**Phase to address:** F1 CLI 集成 Mutagen 的 phase、F2 降级路径、F6 doctor 的 auth+mount 维度、F8 错误码体系（新加 `ENMUT_VERSION_SKEW`）。

**Source:**
- <https://github.com/mutagen-io/mutagen/issues/531>（ddev v0.18.0 → v0.18.1 大规模 handshake 失败，rfay 确认需要双端升级）
- <https://github.com/mutagen-io/mutagen/issues/267>（`mutagen-agents.tar.gz` 缺失导致的 handshake 失败，NixOS 案例）
- <https://github.com/mutagen-io/mutagen/issues/183>（`client/daemon version mismatch` 在 logout/login 后出现）

---

### Pitfall C5：非 root 容器用户导致 Mutagen 首次同步清空 `/workspace`

**What goes wrong:**
v2.0 镜像里 Claude Code 以 `claude` 用户（UID 1000）跑，但 Mutagen sidecar / agent 默认以 root 跑；首次同步时 beta 端对 `/workspace` 只读，Mutagen 触发 safety check 失败，甚至在错误配置下反方向删掉 alpha 侧文件。

**Why it happens:**
- Docker 初始化 named volume 时以 root 所有，容器 USER 改为 1000 后默认无权写入；Mutagen 的 `--default-owner-beta/--default-group-beta` 默认值是 root。
- 如果未显式标明 `mode: two-way-resolved` 且 alpha 是 macOS APFS 大小写不敏感，容易触发 mutagen-alpha-destruction 场景：alpha 被视为"空"，beta 的完整内容反推回去删光原目录。

**How to avoid:**
- 挂载前 entrypoint 先执行 `chown -R 1000:1000 /workspace /hot /cold /overlay`，并且 Dockerfile 里预建这些目录（避免 named volume 初始化时 root 覆盖）。
- Mutagen session 创建命令必须显式加：`--default-owner-beta=id:1000 --default-group-beta=id:1000 --default-file-mode-beta=0644 --default-directory-mode-beta=0755 --mode=two-way-resolved`。
- cloud-claude 启动前检测 alpha 目录是否存在 + 非空才发起同步；若检测到"beta 非空 + alpha 空"异常，拒绝创建 session 并报 `ENMUT_SAFETY_GUARD`。

**Warning signs:**
- `Error: unable to relocate staged file: permission denied`。
- 本地目录突然变空（mutagen-alpha-destruction issue #275 的信号）。

**Phase to address:** F1 Mutagen 配置 phase、F7 Claude Code 登录态持久化（独立 volume 同样需要 chown）、F8 错误码。

**Source:**
- <https://github.com/mutagen-io/mutagen/issues/46>（non-root 用户无法 handshake，havoc-io 解释 `/var/www` root 所有是常态）
- <https://github.com/mutagen-io/mutagen/issues/224>（Docker 初始化 volume 用 root 所有，需要手工 chmod 777 或在 Dockerfile 预创建目录）
- <https://github.com/mutagen-io/mutagen/issues/247>（root ownership on beta 触发 `unable to relocate staged file: permission denied`，推荐以 root 跑 agent + 显式 `--default-owner`）
- <https://github.com/mutagen-io/mutagen/issues/275>（Mutagen 可以在 beta 容器被重建后反向清空 alpha，作者 rfy 提供 repro）
- <https://selfhosting.sh/foundations/docker-volume-permissions/>（UID 映射系统化总结）

---

### Pitfall C6：Ubuntu 25.04 + AppArmor + FUSE 组合全面阻断容器 mount

**What goes wrong:**
生产宿主机一旦升级到 Ubuntu 25.04（默认 AppArmor 策略更严），即使容器加了 `--cap-add SYS_ADMIN --device /dev/fuse --security-opt apparmor=unconfined`，mergerfs / sshfs / mutagen FUSE mount 依然报 `fusermount3: mount failed: Permission denied`；v2.0 已经通过 `apparmor=unconfined` 绕过 docker-default，但 25.04 把"非 confined 的进程能挂载 FUSE"这条默认规则移除了。

**Why it happens:**
Ubuntu 25.04 / fuse3 3.14.0-10 里，host 侧 AppArmor 对 `fusermount3` 二进制的 `capability dac_override` 默认不再放行。privileged mode 也无效。

**How to avoid:**
- `host-preflight.sh` 增加一项"AppArmor + fusermount3 放行"检测：尝试在容器内挂一个空 tmpfs+FUSE，失败则要求运维部署 `/etc/apparmor.d/local/fusermount3` 文件：`capability dac_override,` + `apparmor_parser -r`。
- v2.0 已有的 `verify-fuse-compat.sh` 必须扩展覆盖：mergerfs 挂载、mutagen sidecar 挂载、sshfs 挂载三条路径，每条都跑真实 mount。
- 镜像文档明确支持的宿主 OS 矩阵（Ubuntu 22.04/24.04 原生支持；25.04 需要 AppArmor local override）。

**Warning signs:**
`dmesg` 出现 `apparmor="DENIED" operation="mount" profile="docker-default" ... fstype="fuse.sshfs"`。

**Phase to address:** F1 镜像构建 phase、F6 doctor 的 mount 维度、部署文档更新（v2.0 已有 FUSE/AppArmor 章节，需要追加 25.04 case）。

**Source:**
- <https://github.com/moby/moby/issues/50013>（Ubuntu 25.04 + rclone FUSE 即使 privileged 也失败，社区给出 `/etc/apparmor.d/local/fusermount3` fix）
- <https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105>（Canonical 官方 bug tracker 确认 fuse3 与 AppArmor 冲突）
- <https://github.com/moby/moby/issues/16233>（2015 年的经典案例：`apparmor:unconfined` 是已有答案）

---

### Pitfall C7：tmux session 被 systemd-logind 在 SSH 登出时杀掉

**What goes wrong:**
F4 承诺"断网回到原 shell 不丢进程"，但如果宿主/容器内使用了 `systemd-logind` 且 `KillUserProcesses=yes`（Ubuntu 22.04+ 默认），用户 SSH 断开后 1-2 秒内 tmux server 也被 SIGKILL，所有会话销毁，Claude Code 丢失。

**Why it happens:**
`systemd-logind` 默认把 SSH session 当 user scope 管理；session 关闭时整个 cgroup 被 kill。tmux 的 `SIGHUP` hook 不会触发持久化，直接没了。

**How to avoid:**
- 容器内 entrypoint 必须：要么 `systemd-run --scope --user tmux new-session -d -s default` 启动 server，要么写一个 `tmux@.service` 由 systemd 管控（与 SSH session 解耦）。
- 或者更简单：v3.0 容器不跑 systemd（目前也是），直接让 tmux server 以 PID 1 的 `tini` 子进程启动、不依赖 logind；这是 v2.0 已有的路径，F4 要明确 reaffirm "不引入 logind"。
- UAT：`ssh container 'tmux new -d -s test; sleep 1; pkill -SIGHUP sshd'` 后重连，`tmux attach -t test` 必须成功。

**Warning signs:**
`journalctl -u systemd-logind` 出现 `Removed session c1` + `tmux: killed`。

**Phase to address:** F4 tmux/dtach 会话恢复 phase。

**Source:**
- <https://unix.stackexchange.com/questions/171503/tmux-session-killed-when-disconnecting-from-ssh>（`KillMode` / `KillUserProcesses` 机制与 fix）
- <https://unix.stackexchange.com/questions/659150/tmux-sessions-get-killed-on-ssh-logout>（`systemd-run --scope --user tmux` + `loginctl enable-linger` 综合方案）
- <https://github.com/systemd/systemd/issues/14497>（即使 `KillUserProcesses=no + enable-linger`，特定版本 systemd 依然 kill；说明 v3.0 不应依赖 logind）

---

### Pitfall C8：错误码命名空间与 v2.0 已有 7 个错误码冲突

**What goes wrong:**
F8 把新架构错误纳入 v2.0 错误码体系，但 v2.0 bootstrap_errors.go 的 7 个码若未做命名空间划分，v3.0 新增的 `mutagen sync failed` / `mergerfs mount failed` / `session attach conflict` 很容易与现有 `host_action_failed` 等语义混淆。v1.0 复盘已有教训：worker 统一返回 `host_action_failed`，导致 bootstrap 期望的细粒度码被吞掉。

**Why it happens:**
- 错误码在定义端（bootstrap_errors.go）和消费端（cloud-claude / doctor / worker）各写各的 string，没有集中注册表。
- 单一 package 下的 int constant 会因重复数字或字符串拼写差异造成静默冲突。

**How to avoid:**
- 把错误码改造成**分段命名空间**：`ENBOOT_*`（v2.0 启动）、`ENMUT_*`（F1-F2 文件映射）、`ENSES_*`（F3-F5 会话）、`ENDOC_*`（F6 doctor）、`ENVOL_*`（F7 volume）。每段有编号表 + 中文摘要 + 建议动作 + 回退码。
- PLAN 里必须有一项"错误码表单元测试"：遍历所有码，断言无重复 + 每条都有中文消息 + 每条都给出 `next_action`。
- F6 doctor 输出所有当前命中的错误码（不光是结果 pass/fail），在 human-readable 输出里始终带中文 + 修复命令（违反 Pitfall M13 的反例）。

**Phase to address:** F8 错误码统一、F6 doctor、所有 F1-F7 phase 在 task 结尾统一注册码表。

**Source:**
- `.planning/RETROSPECTIVE.md` v1.0 "error_code 粗粒度"教训（本仓库已有）
- <https://jmmv.dev/2013/08/cli-design-error-reporting.html>（Julio Merino CLI 错误设计的分类标准）
- <https://www.grizzlypeaksoftware.com/library/cli-error-handling-and-user-friendly-messages-qgugu9kg>（错误分类 + `next_action` 模式）

---

## Moderate Pitfalls（体验显著降级，但不会让产品失败）

### Pitfall M1：mergerfs 在 NFS/noforget 组合下 inode 爆内存

**症状：** 容器运行几小时到一天内宿主机 `fuse_inode` slab 涨到 10 万以上，kernel 内存吃满，NFS 客户端出现 stale file handle。
**根因：** `noforget` + 默认 `inodecalc=hybrid-hash` 让 inode 号漂移、无法被内核回收。
**预防：**
- 挂载参数固定 `inodecalc=path-hash`，**不要**加 `noforget`。
- 宿主 sysctl `vm.vfs_cache_pressure=200`，配部署脚本自动写入 `/etc/sysctl.d/40-cloud-claude.conf`。
- F6 doctor 增加 `slabinfo` 检查项，阈值 `fuse_inode > 50000` 告警。
**Phase:** F1 挂载参数、F6 doctor disk 维度。
**来源：** <https://www.diymediaserver.com/post/how-i-fixed-my-24-hour-nfs-crash-loop-with-mergerfs-lxc-and-proxmox/>；<https://github.com/trapexit/mergerfs/issues/1468>（cache.files=full + writeback + dropcacheonclose 竞态 segfault）

---

### Pitfall M2：mergerfs 高并发写 segfault（2.40.2 + cache.files=full）

**症状：** 重度下载 / 大文件写入触发 mergerfs 段错误，挂载点消失，进程报 `Transport endpoint is not connected`。
**根因：** `cache.files=full` + `cache.writeback=true` + `dropcacheonclose=true` 在高线程数下对 `posix_fadvise` 出现竞态，trapexit 在 #1468 里建议改用 `cache.files=off` / `direct_io`。
**预防：**
- 镜像固定 `cache.files=off` 或 `cache.files=partial`（开发场景，不需要 mmap 时）。
- mergerfs 锁版本 ≥ 2.41.0（2025-11-12 后），避免 2.40.2 的已知 segfault 路径。
- crash-loop 自愈：systemd timer 每分钟 `mountpoint -q /workspace || remount`。
**Phase:** F1 镜像构建、F6 doctor mount。
**来源：** <https://github.com/trapexit/mergerfs/issues/1468>（trapexit 亲自确认 "direct_io is overall more performant"）

---

### Pitfall M3：Debian/Ubuntu 源仓里的 mergerfs 版本太旧

**症状：** 容器镜像 `apt install mergerfs` 得到 2.33.5（Debian bookworm）或 2.31.0（Ubuntu 22.04），缺少 `func.readdir=cosr`、inode 稳定化、oom_score_adj 等 v3.0 必需能力。
**根因：** Debian stable 落后主线 2-3 大版本；Ubuntu 22.04/24.04 在 universe 里也只到 2.33.5。
**预防：**
- Dockerfile 里明确从 GitHub Releases 下载 `.deb`：`mergerfs_2.41.x.debian-bookworm_amd64.deb`，或者用 static 构建。
- `image.lock` 中锁定 mergerfs SHA256，拒绝任何 apt 源版本。
- 构建脚本里 `mergerfs -v | grep 2.41` 作为 build-time assert。
**Phase:** F1 镜像构建。
**来源：** <https://tracker.debian.org/mergerfs>；<http://old-releases.ubuntu.com/ubuntu/pool/universe/m/mergerfs/>；<https://github.com/trapexit/mergerfs/releases/tag/2.41.0>（官方 .deb 下载）

---

### Pitfall M4：entrypoint 顺序错误，FUSE 挂载在 sshd 启动后才完成

**症状：** 用户 `cloud-claude` 连上后，前几秒 `/workspace` 还是空的，Claude Code 启动时读不到源文件；偶发"第一次失败、重跑就 OK"。
**根因：** Docker 镜像 entrypoint 顺序写错或并发起了 sshd + mount；SSH session 建立早于 FUSE mount 完成。
**预防：**
- entrypoint 串行化：`prepare-fuse → chown workspace → start mutagen-agent → start mergerfs → wait mountpoint /workspace → exec sshd -D`。
- 使用 `mountpoint -q /workspace` 循环 wait，超时 10s 退出 + 发 doctor 日志。
- 在 worker.go 里对 `--health-cmd` 加 `mountpoint -q /workspace && pgrep sshd`，未健康的容器不允许对外宣告 ready。
**Phase:** F1 镜像 entrypoint phase、F6 doctor mount 维度。
**来源：** <https://stackoverflow.com/questions/38469569/docker-mount-happens-before-or-after-entrypoint-execution>（mount 在 entrypoint 前，但 FUSE mount 是由 entrypoint 自己做的，需要显式等待）；<https://github.com/panubo/docker-sshd/blob/main/entry.sh>（entry.sh 里 `/etc/entrypoint.d` 顺序模式）

---

### Pitfall M5：Mutagen 双向同步在 macOS APFS 触发无穷冲突

**症状：** Mac 用户跑 v3.0，后端 Linux 容器；本地有 `README.md`，远程重建了一个 `readme.md`，同步进入永久冲突循环，Mutagen `sync list` 里 `Conflicts:` 不断累积。
**根因：** macOS APFS 默认 case-insensitive，Linux 端是 case-sensitive；Mutagen 视为两个文件，alpha/beta 各有一份。
**预防：**
- cloud-claude 初始化时检测本地文件系统 case-sensitivity（通过创建 `.cloud-claude.CHECK` + `.cloud-claude.check` 验证）；不敏感时强制 `--ignore-vcs + --mode=two-way-resolved + --symlink-mode=portable` + 启动时警告用户。
- 文档里明确 v3.0 推荐使用独立的"DevCS" APFS 案例。
- 自动给用户生成 `.mutagen-ignore` 模板（含 `.DS_Store`、`node_modules`、Python `__pycache__` 等）。
**Phase:** F1 Mutagen 配置 phase。
**来源：** <https://mutagen.io/documentation/synchronization/>（four modes 语义）；<https://www.reddit.com/r/MacOS/comments/1ha36y5/>（APFS case-insensitive 官方影响）；<https://medium.com/@shyamtala003/fix-macos-case-insensitive-file-system-for-developers-step-by-step-guide-6d3b1eae13ec>（DevCS volume 官方做法）

---

### Pitfall M6：Mutagen ignore 与 .gitignore 语义差异误导用户

**症状：** 用户在 `.gitignore` 加 `dist/`，期望 Mutagen 不同步 dist，但实际 dist 被同步；或反向：Mutagen 忽略了 git 里追踪的文件。
**根因：** Mutagen 的 ignore 语法与 gitignore 类似但**不一样**：
- Mutagen 不会读取 `.gitignore` 文件；
- Mutagen 遇到 ignore 父目录后**不会**遍历子目录，即使子目录有 `!` 重新 include 也不生效（必须显式 `!parent/` + `!parent/**`）。
**预防：**
- 提供 `cloud-claude mutagen gen-ignore` 命令：把 `.gitignore` 转换成 Mutagen 配置（由一个受管 convert 脚本做）。
- 文档与 `cloud-claude doctor` 都明确输出"当前 ignore 规则"和每条 ignore 的来源，避免用户以为 `.gitignore` 自动生效。
- 默认 `ignore: { vcs: true }` + `node_modules` / `__pycache__` / `.venv` / `target` 白名单。
**Phase:** F1 Mutagen 配置 phase、F6 doctor。
**来源：** <https://mutagen.io/documentation/synchronization/ignores/>；<https://mutagen.io/documentation/synchronization/version-control-systems/>；<https://github.com/mutagen-io/mutagen/issues/159>（新用户反复踩同一个坑）；<https://github.com/mutagen-io/mutagen/issues/237>（38 个 👍 要求 `git_aware: true`，至今未实现）

---

### Pitfall M7：多端 attach 时 tmux 把窗口缩到最小客户端尺寸

**症状：** 用户在 Mac（200×50）和 Linux（120×30）同时 attach，tmux 窗口被强制压到 120×30，大屏侧有大量黑边，Claude Code 渲染错位。
**根因：** tmux 默认 `window-size=smallest`；3.3 以后 `latest` 模式下最后有输入的 client 决定尺寸，但同时 attach 时仍会频繁 jitter。
**预防：**
- F5 默认 `aggressive-resize on` + `window-size latest`（v3.0 里把这个写进 `/etc/tmux.conf` 随镜像出厂）。
- `cloud-claude --new-session` 独占路径已经规避此问题，默认 attach 路径要在连接时广播提示"当前与另一个客户端协作，窗口以最新活动客户端尺寸为准"。
**Phase:** F5 多端连接。
**来源：** <https://askubuntu.com/questions/1405802/tmux-does-not-size-down-to-smallest-client>；<https://tmuxai.dev/tmux-window-size/>；<https://github.com/tmux/tmux/issues/2657>（`latest` mode 在 3+ clients 时仍有 bug，官方已 fix，需要 tmux ≥ 3.2）

---

### Pitfall M8：Claude Code 在 tmux 里颜色变灰（`$TMUX` 被检测到就降级）

**症状：** 用户进入 F4 的默认 tmux 包装后，Claude Code logo、Thinking 动画、theme 颜色全部变灰；相同容器、相同 SSH 连接，裸 shell 下颜色正常。
**根因：** Claude Code `cli.js` 函数 `IB3()` 显式检测 `$TMUX` 环境变量，一旦存在就把 chalk color level 降到 2（256 色），即使终端支持 truecolor。
**预防：**
- F4 镜像中 tmux 默认 `/etc/tmux.conf` 加 `set -ga terminal-overrides ",*:RGB"` + 暴露 `CLAUDE_CODE_TMUX_TRUECOLOR=1` 作为默认环境变量（在 `/etc/profile.d/cloud-claude.sh`）。
- cloud-claude 侧也 `ssh -o 'SendEnv=CLAUDE_CODE_TMUX_TRUECOLOR'` 把变量透传过去。
- 文档说明 v3.0 已绕开 Anthropic 未修的 #35148。
**Phase:** F4 tmux 默认配置、F5 多端。
**来源：** <https://github.com/anthropics/claude-code/issues/35148>（社区反查出 `cli.js` 的 IB3 函数行为）；<https://github.com/anthropics/claude-code/issues/46146>（`CLAUDE_CODE_TMUX_TRUECOLOR` 官方未公开但功能可用）；<https://github.com/anthropics/claude-code/issues/32365>（status line 颜色差同一根因）

---

### Pitfall M9：autossh 重连后 sshfs/mutagen 状态僵死

**症状：** autossh 在 tcp 断线后成功重连，但 sshfs 的 channel 已失效，Mutagen 也显示 "Disconnected"，需要用户手动重启。
**根因：** autossh 只重连主 SSH 隧道，子 channel（sshfs 的 passive SFTP、Mutagen 的 agent 通信）都属于原 ssh 会话的 multiplex，对端进程已退，新的 ssh 不会自动重开 channel。
**预防：**
- F3 不要单纯依赖 autossh；cloud-claude 侧自己管理三个逻辑 channel（主 shell、sshfs、mutagen）的 re-dial，断线时单独重建每个 channel 而不是整体 ssh 连接。
- 或者使用 Mosh/ssh ControlMaster + 应用层 ping。在 30s 断网恢复后，首先 `ssh -O check` 断定 master 存活；不存活则重建 master 然后同步重建 sshfs/mutagen。
- UAT：拔网 10s、30s、2min 三个场景分别验证。
**Phase:** F3 弱网容忍、F4 会话恢复。
**来源：** <https://github.com/libfuse/sshfs/issues/3>（sshfs + reconnect 的根本局限）；<https://serverfault.com/questions/1002279/ssh-multiplexing-hangs-if-not-gracefully-shutdown>（ControlMaster hang 场景）

---

### Pitfall M10：Mosh 与 sing-box tun 全隧道防火墙冲突

**症状：** v3.0 把 Mosh 列为可选弱网方案；但用户开启后 Mosh 客户端报 `Nothing received from server on UDP port 60001`，原因是宿主机的 nftables 默认拒绝 UDP，并且 sing-box tun 的默认策略不放行 Mosh 用的 60000-61000 UDP。
**根因：** sing-box tun 把 UDP 当作 egress 路由走代理，但 Mosh 是 **ingress** 到容器的 UDP；另 nftables 默认 `ct state new udp drop`。
**预防：**
- 如果 F3 明确纳入 Mosh，必须在 nftables 规则里白名单 Mosh 端口（建议缩小到 60050-60060，降低扫描面）。
- sing-box tun 配置 `inbound rule` 单独放行 60050-60060/udp → 直连容器，不走代理。
- 如果范围放行对威胁模型不接受，v3.0 可以**默认不启用 Mosh**，把 tmux + autossh + ControlMaster 作为弱网主路径，Mosh 留作文档建议。
**Phase:** F3 SSH 弱网容忍、网络约束一致性（沿用 v1.1 的 nftables 规则）。
**来源：** <https://github.com/mobile-shell/mosh/issues/1039>（UDP 60001 被防火墙拦掉，需要显式 ufw/nftables 放行）；<http://snippets.khromov.se/iptables-rules-for-mosh-connections/>（60000-61000 官方范围）；<https://oneuptime.com/blog/post/2026-03-20-allow-ssh-traffic-nftables/view>（nftables 规则顺序必须 established 在前）

---

### Pitfall M11：ServerAliveInterval 过小触发宿主 IDS/运营商 DPI 误报

**症状：** 为了弱网容忍把 `ServerAliveInterval=5`，一旦宿主机开了 IDS/fail2ban 或上游运营商 DPI，会把短周期 keepalive 当做异常流量，临时封 IP。
**根因：** 5s 的 null packet 频率远超正常 SSH 交互模型。
**预防：**
- 默认 `ServerAliveInterval=30, ServerAliveCountMax=3`（90s 总窗口），既能覆盖常见 NAT keepalive timeout，又不会过度触发。
- 宿主机 `sshd_config` 同步 `ClientAliveInterval=60 ClientAliveCountMax=3`（保证上下游对齐）。
- 文档里建议用户不要把 `ServerAliveInterval` 调到 < 15s。
**Phase:** F3 SSH 弱网容忍。
**来源：** <https://unix.stackexchange.com/questions/318089/behavior-of-serveraliveinterval-with-ssh-connection>（`too many logins` 场景直接由频繁 keepalive + MaxSessions 冲突导致）；<https://dohost.us/index.php/2025/08/29/troubleshooting-common-ids-false-positives-and-false-negatives/>（IDS 对频繁 null packet 的误判分类）

---

### Pitfall M12：ControlMaster 多路复用限制在 MaxSessions=10（OpenSSH 默认）

**症状：** v3.0 的 cloud-claude 同时开 main shell + tmux session + sshfs + mutagen channel + doctor probe，5 个就已经用掉一半 session 额度；并发多开几个 `--new-session` 后 `no more sessions` 报错。
**根因：** OpenSSH `MaxSessions` 默认 10，ControlMaster 共享一条 tcp 下每多一个逻辑 channel 就占 1 session。
**预防：**
- 宿主镜像 `sshd_config` 调 `MaxSessions=30 MaxStartups=60:30:120`。
- cloud-claude 内部限制：每个 ControlMaster 最多 8 个 channel，超过时自动开新 master（不同 ControlPath）。
- F6 doctor 的 ssh 维度增加 `MaxSessions` 值自检。
**Phase:** F3、F5、F6。
**来源：** <https://unix.stackexchange.com/questions/22965/limits-of-ssh-multiplexing>（`MaxSessions` 默认 10，`sshd[...]: error: no more sessions` 确认信号）

---

### Pitfall M13：静默降级让用户以为"一切正常"

**症状：** F2 降级逻辑在 Mutagen 起不来时自动切到 `sshfs-only`，但如果只是 debug log 写了一行英文，CLI 默默继续，用户会误判"v3.0 性能和 v2.0 差不多"，并在 issue 里抱怨。
**根因：** 静默 fallback 是经典 CLI UX 反例；LlamaIndex 最近的"本地优先库偷偷发 OpenAI"事件就是典型教训。
**预防：**
- F2 每次降级必须：
  1. stderr 中文清晰说明**当前模式**（`⚠ 已降级到 sshfs-only，性能较慢；原因：mutagen agent 版本不匹配（EN MUT_VERSION_SKEW）`）；
  2. `cloud-claude status` 永远显示当前 mount 模式 + 任意降级原因；
  3. 退出码非 0（根据策略选择 warning 级别或强制失败），脚本化消费者能识别；
  4. doctor 的第一屏展示所有曾发生过的降级事件（最近 10 次）。
- 测试用例：强制 kill 容器内 mutagen-agent，cloud-claude 必须在 stderr 打出指定中文 + 错误码。
**Phase:** F2 降级策略、F6 doctor、F8 错误码。
**来源：** <https://uxdesign.cc/fail-early-a-hidden-design-principle-of-good-products-and-services-b23af66e0247>（"fail early/visibly" 原则）；<https://www.reddit.com/r/LocalLLaMA/comments/1ro71ou/the_silent_openai_fallback_why_llamaindex_might/>（真实事故：LlamaIndex 静默回退到 OpenAI 差点泄漏本地数据）；<https://www.grizzlypeaksoftware.com/library/cli-error-handling-and-user-friendly-messages-qgugu9kg>（非零退出码必要性）

---

### Pitfall M14：doctor 报错不给修复命令

**症状：** 用户运行 `cloud-claude doctor`，看到 5 个红色 FAIL，但每条只写"mount failed"，不给任何下一步动作。用户要么放弃、要么在群里 ping 作者。
**根因：** 经典 CLI 反例——错误被检测到但没有"建议动作"字段。
**预防：**
- F6 doctor 输出结构：`[FAIL] 标题 | 原因 | 建议命令 | 文档链接 | 错误码`。参考 v2.0 `verify-fuse-compat.sh` 已有的 `[PASS]/[FAIL]/[WARN]` 格式，但升级为每条**必须**有 `建议` 字段。
- CI 有一个专门测试：遍历所有 doctor check 函数，断言任何 FAIL 返回的结构里 `Suggestion != ""`。
**Phase:** F6 doctor、F8 错误码。
**来源：** <https://jmmv.dev/2013/08/cli-design-error-reporting.html>（Julio Merino: 每条错误必须给出 "how to fix"）；<https://www.grizzlypeaksoftware.com/library/cli-error-handling-and-user-friendly-messages-qgugu9kg>（CLI error 必须分类 + 给建议）

---

### Pitfall M15：同账号多端同时写文件触发 Mutagen 冲突累积

**症状：** 用户在 Mac 和 Linux 同时编辑同一个文件（协作观察），Mutagen 两边 alpha，冲突堆到 conflicts 列表里，同步停止。
**根因：** v3.0 的 F5 多端模型默认让两个客户端都成为 Mutagen alpha（或各自建 sync session）时，双向同步对同一文件的变更无法自动合并。
**预防：**
- 每账号同时只允许**一个** Mutagen sync session（用 `claude_account_id` 做锁）；后连的客户端只 attach tmux（观察），不参与文件同步。
- 如果用户坚持需要"双端都能写"，必须显式 `--new-session`，此时后连端走独立的 sync session（不同目录或显式用 git/分支）。
- UAT：两端同时 `echo X >> foo.txt`，确认后端只有一条 sync session 活跃。
**Phase:** F5 多端、F7 账号级资源隔离。
**来源：** <https://mutagen.io/documentation/synchronization/>（`two-way-safe` 冲突累积语义）；<https://github.com/mutagen-io/mutagen/issues/533>（`two-way-resolved` 模式下也可能残留 conflict 的真实案例）

---

### Pitfall M16：Docker named volume 残留导致磁盘吃满

**症状：** 管理员删除 claude_account，但对应的 `~/.claude` 持久化 volume 没被清理；数月后宿主机 `/var/lib/docker/volumes` 撑到 GB 级。
**根因：** Docker 默认 `docker volume prune` 只删 anonymous volume；named volume 需要显式 `-a` 或指定名称。
**预防：**
- 所有持久化 volume 名称加**强命名规范**：`ccp_claude_<account_id>_home` / `ccp_claude_<account_id>_cache`；带 label：`com.cloud-cli-proxy.account_id=<uuid>`、`com.cloud-cli-proxy.created_at`。
- claude_account 的 DELETE handler 在同事务内发起 `host-agent DeleteVolumes(account_id)`，并记录 event。与 v1.0 复盘 key lesson #2 对齐（新增横切关注点必须全量覆盖所有 handler）。
- 运维手册新增章节"孤儿 volume 审计 + 定期清理"，脚本用 label filter：`docker volume ls --filter label=com.cloud-cli-proxy.account_id -q | ...`。
**Phase:** F7 持久化 volume phase、v2.0 admin API 联动。
**来源：** <https://oneuptime.com/blog/post/2026-02-08-how-to-clean-up-orphaned-docker-volumes/view>；<https://docs.docker.com/reference/cli/docker/volume/prune>（named volume 默认不清理）

---

### Pitfall M17：bind mount 与 named volume 在 UID 映射上的差异

**症状：** F7 持久化 volume 里 `~/.claude` 以 root 所有，容器里 claude 用户（UID 1000）起不来 Claude Code，报 `permission denied on ~/.claude/config`。
**根因：** named volume 由 Docker 首次挂载时用镜像里 `~/.claude` 的权限初始化；如果镜像里这个目录不存在或所有者是 root，初始化后 claude 用户永远进不去。
**预防：**
- Dockerfile 里预先 `RUN mkdir -p /home/claude/.claude /home/claude/.cache/claude && chown -R 1000:1000 /home/claude`。named volume 首次挂载会继承这个权限。
- 容器 entrypoint 开头再做一次 `chown -R 1000:1000 /home/claude` 以防 volume 是在旧镜像版本下初始化的。
- UAT：删除 volume → 冷启动容器 → `docker exec -u 1000 test id ~/.claude` 必须成功。
**Phase:** F1 镜像 + F7 持久化 volume。
**来源：** <https://selfhosting.sh/foundations/docker-volume-permissions/>（named volume 继承镜像目录权限）；<https://fixdevs.com/blog/docker-volume-permission-denied/>（UID mismatch 的完整排查）

---

### Pitfall M18：镜像体积膨胀（mergerfs + mutagen-agent + tmux 叠加超过 800MB）

**症状：** v2.0 镜像约 ~400MB。v3.0 叠加 mergerfs、mutagen-agent、tmux、claude 登录态后如果不做多阶段构建，容器很容易冲到 1.2GB，冷启动拉镜像慢。
**根因：**
- `apt-get install` 默认带 recommends 拉入大量可选包；
- mutagen-agent 为多架构 tar 包一次装进去会占 ~80MB；
- mergerfs 从 Debian 默认源装会拖整个依赖树。
**预防：**
- Dockerfile 必须启用 BuildKit cache mount，`--mount=type=cache,target=/var/cache/apt`。
- 全部 `apt install` 用 `--no-install-recommends` + 单层 RUN + `rm -rf /var/lib/apt/lists/*`。
- mergerfs 用 static .deb（见 Pitfall M3），mutagen-agent 只保留 linux/amd64 + linux/arm64 两个架构。
- 镜像大小作为 CI gate：超过 700MB 直接 fail build。
**Phase:** F1 镜像构建。
**来源：** <https://oneuptime.com/blog/post/2026-03-02-how-to-write-efficient-dockerfiles-for-ubuntu-based-images/view>（分层/layer caching 最佳实践）；<https://docs.docker.com/build/cache/optimize>（BuildKit cache mount 官方教程）

---

## Minor Pitfalls（文档层面需要对齐的预期）

### Pitfall m1：Linux 上 Mutagen 只对最近 50 个目录做 inotify 热点监控

**根因/症状：** Mutagen 在 Linux 用 polling + 最近 50 个目录 inotify 的 hybrid 模型；大仓库里新打开的文件延迟可能到 10s（poll interval）。
**预防：** 文档说明"Linux 端 watch 延迟 ≤ 10s，可以 `watch: { polling-interval: 2s }` 调小但占 CPU"；`/proc/sys/fs/inotify/max_user_watches` 建议 ≥ 524288 并在 host-preflight.sh 检测。
**Phase:** F1 Mutagen 配置 + 文档。
**来源：** <https://github.com/mutagen-io/mutagen/issues/45>（havoc-io 亲自说明 50 个 inotify watch 限制）；<https://mutagen.io/documentation/synchronization/watching/>

---

### Pitfall m2：ControlMaster 在 overlayfs 上的 socket 失效

**根因/症状：** 如果 cloud-claude 把 ControlPath 写到 `~/.ssh/control:*`，而用户 `$HOME` 在 overlayfs（部分 CI / 宿主 runner）上，Unix socket 失效，每次都变成 stale master。
**预防：** 默认 ControlPath 写到 `$XDG_RUNTIME_DIR/cloud-claude/control-%h-%p-%r`；fallback 到 `/tmp/cloud-claude-$USER`。
**Phase:** F3 SSH 弱网容忍的实现细节。
**来源：** <https://stackoverflow.com/questions/36459785/shared-ssh-connection-with-control-master-not-working>（overlayfs + Unix socket 的已知不兼容）

---

### Pitfall m3：tmux server 崩溃后丢 socket 无法自愈

**根因/症状：** tmux server 自身进程崩溃（OOM、bug），`tmux attach` 报 `no server running`，所有 session 丢失。
**预防：** F4 把 tmux server 包在 `tini` 或 systemd user unit 里，exit 非 0 自动重启；tmux server 启动时开启 `set -s exit-empty off` `exit-unattached off`。cloud-claude 在 doctor 里加"tmux server PID + socket 路径"检查。
**Phase:** F4。
**来源：** <https://github.com/tmux/tmux/issues/1174>（server 被 SIGHUP 杀死时 hook 不执行）；<https://unix.stackexchange.com/questions/659150/tmux-sessions-get-killed-on-ssh-logout>（tmux server 守护服务化模板）

---

### Pitfall m4：多端连接鉴权 token 刷新 race

**根因/症状：** Mac + Linux 同时触发 Entry API 轮询，token 并发刷新时后发请求拿到旧 token，认证失败。
**预防：** Entry API client 内部 token 缓存加 mutex；cloud-claude 本地 token 存储用 `flock`；服务端侧接受 grace period（旧 token 失效后 30s 内仍可完成刷新）。
**Phase:** F5 多端、复用 v1.2 用户认证体系。
**来源：** 对齐 `.planning/RETROSPECTIVE.md` v1.1 key lesson "停机路径清理对称性"（并发 race 同理需要在双端路径都落位）。

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| 在 `/workspace` 下直接跑 mergerfs，不分 branch mode | 少一个配置项，镜像参数短 | 写入路由不稳定，Mutagen safety check 失败（Pitfall C2） | **从不可接受**，v3.0 首日就必须完整定义 branch mode |
| 把 sshfs `reconnect` 参数缺省，依赖 autossh 重建整条隧道 | 代码少一点 | 抖动 → sshfs 挂死 → 所有进程 block（Pitfall C3） | 仅在"绝对不允许网络抖动"的 LAN 场景，v3.0 显然不适用 |
| tmux 用 `systemd --user` 守护 | 写起来优雅 | 依赖 logind，一旦配置漂移就整包丢 session（Pitfall C7） | 只在 v3.0 之后如果引入 systemd 容器内管理才重新评估 |
| F2 静默 fallback 到 sshfs-only，只打英文 log | 减少用户打扰 | 用户永远不知道自己在降级模式下跑（Pitfall M13） | 从不可接受 |
| 复用 v2.0 的 7 个错误码字符串直接扩写 | 改动小 | 错误码含义漂移，外部脚本无法稳定解析（Pitfall C8） | 仅在短期 hotfix，v3.0 必须切到命名空间分段 |
| 镜像里 `apt install mergerfs` 不锁版本 | Dockerfile 一行 | 碰到 Debian bookworm 只有 2.33.5，缺关键功能（Pitfall M3） | 从不可接受；lock `.deb` SHA256 |
| Mutagen mode 用默认 `two-way-safe` 不显式设 | 默认值最安全 | 在 APFS case-insensitive 下持续堆 conflict（Pitfall M5） | 从不可接受，必须按平台自动选择 |
| doctor 仅输出 PASS/FAIL 无建议 | 开发快 | 用户无法自救（Pitfall M14） | 从不可接受 |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Mutagen + Docker named volume | volume 首次挂载以 root 所有，agent 无法写入 | Dockerfile 预建目录 + chown；session 显式 `--default-owner-beta=id:1000`（Pitfall C5） |
| mergerfs + sshfs | 直接串联没有调缓存，90s readdir | `cache.readdir=true,cache.entry=30,cache.attr=30,func.readdir=cosr`（Pitfall C1） |
| sshfs + reconnect | 只写 `-o reconnect`，进程挂死 | 必须叠加 `ServerAliveInterval=15,ServerAliveCountMax=3,ConnectTimeout=10`（Pitfall C3） |
| Docker + FUSE + AppArmor (Ubuntu 25.04) | 依赖 `--security-opt apparmor=unconfined` | 额外部署 `/etc/apparmor.d/local/fusermount3`（Pitfall C6） |
| Claude Code + tmux | 不做任何处理导致颜色变灰 | 预置 `CLAUDE_CODE_TMUX_TRUECOLOR=1`（Pitfall M8） |
| Mosh + sing-box tun | Mosh UDP 被全隧道吞掉 | nftables 白名单 60050-60060/udp + sing-box inbound 例外（Pitfall M10） |
| autossh + sshfs + Mutagen | 只重建 SSH，子通道僵死 | 每个子通道独立 re-dial（Pitfall M9） |
| ControlMaster + MaxSessions | 默认 10 session 上限 | sshd `MaxSessions=30`，cloud-claude 内部限 8（Pitfall M12） |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| mergerfs 串行 readdir | `ls -R` 10k 文件 > 30s | `func.readdir=cosr` + `cache.readdir=true` | ≥ 3000 文件 + sshfs branch（Pitfall C1） |
| mergerfs inode 爆涨 | `slabinfo.fuse_inode > 100000`，OOM | `inodecalc=path-hash` + 不用 `noforget` | 容器持续运行 > 24h 或通过 NFS 暴露（Pitfall M1） |
| Mutagen polling 过慢 | 编辑后 10s 才同步 | `watch-polling-interval=2s` 或确保 alpha 在 macOS（原生 recursive watch） | 大 repo + Linux alpha |
| tmux 窗口频繁 resize | 字符错位、颜色重置 | `window-size=latest` + `aggressive-resize=on` | 多端并发 attach（Pitfall M7） |
| 镜像拉取慢 | 冷启动 > 30s | BuildKit cache + `--no-install-recommends` + 静态 .deb | 镜像 > 700MB（Pitfall M18） |
| ServerAliveInterval 过小 | fail2ban / DPI 封 IP | ≥ 30s（Pitfall M11） | 每日自动运行的 CI/CD 任务 |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `--security-opt apparmor=unconfined` 未限制到必要 capability | 容器越权 mount 宿主目录 | 保留 apparmor unconfined 但用 seccomp profile + 只 `--cap-add SYS_ADMIN,CAP_DAC_READ_SEARCH`；v2.0 现有方案延用 |
| sshfs 弱 cipher（arcfour） | 若链路被嗅探可能被快速破解 | 2026 年替换为 `aes128-gcm@openssh.com` / `chacha20-poly1305@openssh.com` |
| Mutagen sync 同步 `.git` 目录 | hook script 在远端执行 + 上传到 beta 可能泄漏 credentials | 默认 `ignore: { vcs: true }`；文档警示（Pitfall M6） |
| persistent volume 未加 account 标签 | 误删其他账号的 Claude 登录态 | label + 命名规范 + admin DELETE 事务联动（Pitfall M16） |
| Mosh UDP 范围过大 | 60000-61000 一共 1001 个端口对外暴露 → 扫描面大 | 缩小到 10 个端口 60050-60060（Pitfall M10） |
| 错误消息回显全部 stacktrace | 泄漏容器内路径、PID、内部 API URL | `--verbose` 默认关闭；错误码 + 简短中文（Pitfall C8 / M14） |

---

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| 英文 stacktrace 直接透传 | 用户不敢继续用，找不到下一步 | 所有错误路径都有中文错误码 + `next_action`（F8） |
| 静默降级 | 用户以为在 full 模式下跑，性能差怪 v3.0 | 降级必须 stderr 显式提示 + status 命令常驻展示（M13） |
| doctor 仅 FAIL/PASS 不给命令 | 用户无法自救 | 每条 FAIL 必须带 `Suggestion` 字段（M14） |
| Mutagen 冲突堆积但 CLI 不提示 | 用户几小时后才发现文件没同步 | cloud-claude 启动时 `mutagen sync list` 检测 conflict，直接阻断并中文说明（M15） |
| 多端 attach 尺寸跳动 | 字符错位 | 默认 `aggressive-resize on` + 连接时广播中文提示（M7） |
| 第一次连接 > 8s 但无进度条 | 用户以为卡死 | 和 v1.0 bootstrap 一样的 stage progress：connecting → mounting → mergerfs ready → ssh handshake → mutagen first sync（性能基线已在 PROJECT.md 承诺） |

---

## "Looks Done But Isn't" Checklist

- [ ] **F1 三层挂载**：容器启动后 `mount | grep mergerfs` 必须显示 `func.readdir=cosr,cache.readdir=true,cache.entry=30,cache.attr=30`，否则 Pitfall C1 已命中。
- [ ] **F1 branch mode**：`getfattr -n user.mergerfs.branches /workspace/.mergerfs` 必须返回三条，对应模式分别是 `RW/NC/RO`。
- [ ] **F2 降级**：强杀容器内 mutagen-agent 后，cloud-claude stderr 必须在 5s 内打出中文降级提示 + 错误码。
- [ ] **F3 弱网**：拔网 30s 后恢复，tmux session 不丢失、sshfs 自动恢复、Mutagen 自动重连，无需用户操作。
- [ ] **F4 tmux**：`docker stop $(docker ps -q) && docker start <ct>` 重启后，`tmux attach -t default` 能恢复（即使 tmux server 曾崩溃）。
- [ ] **F4 颜色**：在 tmux 内跑 Claude Code，Logo 颜色应当与裸 shell 一致（`echo $CLAUDE_CODE_TMUX_TRUECOLOR` == 1）。
- [ ] **F5 多端**：两端同时连接同账号，默认 attach 同一 tmux，Mutagen 只有一个 session active。
- [ ] **F5 new-session**：`--new-session` 成功独占，旧 session 继续独立运行。
- [ ] **F6 doctor**：每个 FAIL 项都有 `Suggestion`，通过 `grep -L "Suggestion:" doctor-output.txt` 应无命中。
- [ ] **F6 doctor 五维度**：network + auth + ssh + mount（三层分别检查）+ disk 都跑到，缺一 CI fail。
- [ ] **F7 volume**：删除 claude_account 后 30s 内对应 volume 在 `docker volume ls` 中消失（事务联动）。
- [ ] **F7 volume label**：`docker volume inspect` 看得到 `com.cloud-cli-proxy.account_id` label。
- [ ] **F8 错误码**：`cloud-claude internal list-error-codes` 输出所有码，每条都有命名空间前缀（ENMUT/ENSES/...）+ 中文消息 + `next_action`。
- [ ] **宿主机前置**：`host-preflight.sh` 在 Ubuntu 25.04 上必须给出 AppArmor local override 的中文提示。
- [ ] **镜像大小**：CI build 输出镜像 ≤ 700MB。
- [ ] **版本声明**：release notes 同步声明本地 Mutagen 所需版本范围，cloud-claude 启动时自动校验（Pitfall C4）。

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| C1 readdir 慢 | LOW | 重新挂载 mergerfs 并更新参数；`fusermount -u` → 使用新参数重新 mount |
| C2 create 策略错 | MEDIUM | 停容器 → 把误落盘的文件手动搬回正确 branch → 重启 |
| C3 sshfs 挂死 | LOW | `fusermount -u -z /workspace/.cold` + 自愈 remount（systemd timer） |
| C4 Mutagen 版本漂移 | LOW | cloud-claude 检测到后自动切换到 sshfs-only；提示用户 `brew upgrade mutagen` 并重跑 |
| C5 Mutagen 权限 | HIGH | 可能已有数据丢失，需要备份恢复；生产环境必须在 F1 落地前就写好预防 chown 逻辑 |
| C6 AppArmor 阻断 | MEDIUM | 根据 doctor 指示手动部署 `/etc/apparmor.d/local/fusermount3` + reload |
| C7 tmux 被 kill | LOW | 本次 session 丢失；F4 改为非 logind 方案后不复发 |
| C8 错误码冲突 | MEDIUM | 发布 hotfix 重新划分命名空间 + 通知下游脚本消费者 |
| M1 inode 爆涨 | MEDIUM | 重启容器（内存立刻释放）+ 更新挂载参数 |
| M2 mergerfs segfault | MEDIUM | mount 自愈脚本；升级到 2.41.x |
| M5 APFS 冲突 | LOW | `mutagen sync terminate <id>` + 按平台启用独立 DevCS volume 后重建 |
| M13 静默降级 | LOW | 重新启动客户端；F2 升级版本后每次降级都会明确提示 |
| M16 volume 残留 | MEDIUM | 按 label 批量 `docker volume rm`；事务化 admin DELETE handler |

---

## Pitfall-to-Phase Mapping

| Pitfall | Severity | Prevention Phase | Verification |
|---------|----------|------------------|--------------|
| C1 readdir 90s | CRITICAL | F1 | UAT：10k 文件 `rg .` ≤ 本地 1.5× |
| C2 create pfrd 乱写 | CRITICAL | F1 | CI：`getfattr` 断言 create policy；branch mode 矩阵表 |
| C3 sshfs 级联挂死 | CRITICAL | F1 + F2 + F3 | UAT：拔网 30s 无挂起进程 |
| C4 Mutagen 版本漂移 | CRITICAL | F1 + F8 | UAT：本地装旧版 Mutagen → cloud-claude 应显示 ENMUT_VERSION_SKEW |
| C5 Mutagen 权限清空 | CRITICAL | F1 + F7 | UAT：非空本地目录首次同步无数据丢失 |
| C6 Ubuntu 25.04 AppArmor | CRITICAL | F1 镜像 + host-preflight | 部署文档 + verify-fuse-compat.sh 扩展 |
| C7 tmux 被 logind kill | CRITICAL | F4 | UAT：SSH 断开 5s 后 `tmux attach` 仍成功 |
| C8 错误码冲突 | CRITICAL | F8 | CI：`internal list-error-codes` 唯一性断言 |
| M1 inode 爆涨 | MODERATE | F1 + F6 | doctor disk 维度：fuse_inode 阈值告警 |
| M2 mergerfs 高并发 segfault | MODERATE | F1 | 镜像 lock mergerfs ≥ 2.41.0 |
| M3 Debian 源版本太旧 | MODERATE | F1 | CI build-time `mergerfs -v` 断言 |
| M4 entrypoint 顺序 | MODERATE | F1 | healthcheck + UAT 冷启动顺序 |
| M5 APFS 冲突 | MODERATE | F1 | cloud-claude 启动检测 case-sensitivity |
| M6 ignore 语义差异 | MODERATE | F1 + F6 | doctor 输出当前生效 ignore 规则 |
| M7 多端尺寸 | MODERATE | F5 | /etc/tmux.conf 固化 `window-size latest` |
| M8 Claude 颜色灰 | MODERATE | F4 | UAT：tmux 内 Claude Code 颜色与裸 shell 一致 |
| M9 autossh 僵死 | MODERATE | F3 + F4 | UAT：三时长（10s/30s/2min）网络抖动场景 |
| M10 Mosh + sing-box | MODERATE | F3 | nftables 规则 + sing-box inbound 放行 |
| M11 IDS 误报 | MODERATE | F3 | 默认 `ServerAliveInterval=30` |
| M12 MaxSessions 10 | MODERATE | F3 + F6 | sshd_config 覆盖 + doctor ssh 维度 |
| M13 静默 fallback | MODERATE | F2 + F6 + F8 | 每次降级都有 stderr + status + 错误码 |
| M14 doctor 无建议 | MODERATE | F6 + F8 | CI：所有 FAIL 必须有 Suggestion |
| M15 多端 Mutagen 冲突 | MODERATE | F5 | 账号级 sync session 唯一锁 |
| M16 volume 残留 | MODERATE | F7 + 运维 | label + DELETE handler 事务 |
| M17 volume UID 差异 | MODERATE | F1 + F7 | 镜像 Dockerfile chown + entrypoint fallback |
| M18 镜像体积 | MODERATE | F1 | CI gate ≤ 700MB |
| m1 inotify 50 | MINOR | F1 + 文档 | host-preflight.sh 检查 max_user_watches |
| m2 ControlMaster + overlayfs | MINOR | F3 | ControlPath 写到 `$XDG_RUNTIME_DIR` |
| m3 tmux server 崩溃 | MINOR | F4 | tini / systemd user unit 自愈 |
| m4 多端 token race | MINOR | F5 | token 缓存 mutex + 服务端 grace period |

---

## Critical Pitfalls 优先级排序给 roadmapper

Roadmapper 必须把 **C1 / C2 / C3 / C5 / C6** 作为 v3.0 第一 wave 的 phase 必做事项（对应 F1 主线 A 三层文件系统），否则 v3.0 发布后会立刻集中爆雷。**C4 / C7 / C8** 排在第二 wave（对应 F2/F4/F8），因为它们不依赖文件系统正确性，可以在 F1 稳定后并行推进。

Planner 在每个 phase 的 PLAN.md 里必须：
1. 在 goal 段引用相应 pitfall 编号；
2. 在 tasks 段为每条 Critical pitfall 创建一个显式"防御任务"（单独 commit，独立 UAT）；
3. 在 verification 段给出对应的 `mount | grep` / `getfattr` / strace / UAT 脚本命令。

---

## Sources（汇总）

### 官方文档（HIGH 信心）
- <https://trapexit.github.io/mergerfs/latest/config/options>（mergerfs 全部选项）
- <https://trapexit.github.io/mergerfs/latest/config/functions_categories_policies>（branch mode）
- <https://mutagen.io/documentation/synchronization/>（Mutagen sync modes）
- <https://mutagen.io/documentation/synchronization/ignores/>（Mutagen ignore 语义）
- <https://mutagen.io/documentation/synchronization/version-control-systems/>（vcs ignore）
- <https://mutagen.io/documentation/synchronization/watching/>（Linux watching 机制）
- <https://docs.docker.com/reference/cli/docker/volume/prune>（volume prune 默认行为）
- <https://docs.docker.com/build/cache/optimize>（BuildKit cache）
- <https://code.claude.com/docs/en/env-vars>（Claude Code 环境变量，见 #46146 要求）

### GitHub Issues / Releases（HIGH 信心）
- mergerfs：<https://github.com/trapexit/mergerfs/issues/893>、<https://github.com/trapexit/mergerfs/issues/900>、<https://github.com/trapexit/mergerfs/issues/869>、<https://github.com/trapexit/mergerfs/issues/1468>、<https://github.com/trapexit/mergerfs/releases/tag/2.41.0>、<https://github.com/trapexit/mergerfs/discussions/1571>
- Mutagen：<https://github.com/mutagen-io/mutagen/issues/46>、<https://github.com/mutagen-io/mutagen/issues/131>、<https://github.com/mutagen-io/mutagen/issues/159>、<https://github.com/mutagen-io/mutagen/issues/183>、<https://github.com/mutagen-io/mutagen/issues/194>、<https://github.com/mutagen-io/mutagen/issues/224>、<https://github.com/mutagen-io/mutagen/issues/237>、<https://github.com/mutagen-io/mutagen/issues/247>、<https://github.com/mutagen-io/mutagen/issues/267>、<https://github.com/mutagen-io/mutagen/issues/275>、<https://github.com/mutagen-io/mutagen/issues/459>、<https://github.com/mutagen-io/mutagen/issues/492>、<https://github.com/mutagen-io/mutagen/issues/531>、<https://github.com/mutagen-io/mutagen/issues/533>、<https://github.com/mutagen-io/mutagen-compose/issues/6>
- sshfs：<https://github.com/libfuse/sshfs/issues/3>、<https://github.com/libfuse/sshfs/pull/306>
- tmux：<https://github.com/tmux/tmux/issues/1174>、<https://github.com/tmux/tmux/issues/2657>
- Docker / Moby：<https://github.com/moby/moby/issues/50013>、<https://github.com/moby/moby/issues/16233>
- Claude Code：<https://github.com/anthropics/claude-code/issues/32365>、<https://github.com/anthropics/claude-code/issues/35148>、<https://github.com/anthropics/claude-code/issues/39566>、<https://github.com/anthropics/claude-code/issues/46146>
- systemd：<https://github.com/systemd/systemd/issues/14497>
- Mosh：<https://github.com/mobile-shell/mosh/issues/1039>、<https://github.com/mobile-shell/mosh/issues/1074>

### 社区与官方 bug tracker（MEDIUM-HIGH 信心）
- Ubuntu fuse3：<https://bugs.launchpad.net/ubuntu/+source/fuse3/+bug/2111105>
- Debian mergerfs tracker：<https://tracker.debian.org/mergerfs>
- Ubuntu old-releases mergerfs：<http://old-releases.ubuntu.com/ubuntu/pool/universe/m/mergerfs/>

### Blog / StackOverflow（MEDIUM 信心，多处交叉验证）
- sshfs 重连：<https://askubuntu.com/questions/791002/how-to-prevent-sshfs-mount-freeze-after-changing-connection-after-suspend>、<https://askubuntu.com/questions/716612/sshfs-auto-reconnect>、<https://serverfault.com/questions/6709/sshfs-mount-that-survives-disconnect>、<https://stackoverflow.com/questions/17686952/mounting-sshfs-on-unreliable-connection>
- ssh ControlMaster：<https://stackoverflow.com/questions/36459785/shared-ssh-connection-with-control-master-not-working/36479771>、<https://serverfault.com/questions/1002279/ssh-multiplexing-hangs-if-not-gracefully-shutdown>、<https://unix.stackexchange.com/questions/22965/limits-of-ssh-multiplexing>、<https://stackoverflow.com/questions/44492312/why-a-background-ssh-can-take-over-the-tty-from-bash>、<https://serverfault.com/questions/351162/ssh-fails-pty-allocation-request-failed-on-channel-0>
- SSH alive interval：<https://unix.stackexchange.com/questions/3026/what-do-options-serveraliveinterval-and-clientaliveinterval-in-sshd-config-do-exactly>、<https://unix.stackexchange.com/questions/318089/behavior-of-serveraliveinterval-with-ssh-connection>、<https://www.cyberciti.biz/tips/open-ssh-server-connection-drops-out-after-few-or-n-minutes-of-inactivity.html>
- tmux 与 systemd：<https://unix.stackexchange.com/questions/171503/tmux-session-killed-when-disconnecting-from-ssh>、<https://unix.stackexchange.com/questions/659150/tmux-sessions-get-killed-on-ssh-logout>、<https://superuser.com/questions/1372963/how-do-i-keep-systemd-from-killing-my-tmux-sessions>
- tmux window-size：<https://askubuntu.com/questions/1405802/tmux-does-not-size-down-to-smallest-client>、<https://www.mail-archive.com/tmux-users@googlegroups.com/msg02066.html>、<https://tmuxai.dev/tmux-window-size/>
- Claude Code truecolor：<https://ranang.medium.com/fixing-claude-codes-flat-or-washed-out-remote-colors-82f8143351ed>
- APFS case-insensitive：<https://www.reddit.com/r/MacOS/comments/1ha36y5/macos_apfs_caseinsensitive_and_handling_git_repo/>、<https://medium.com/@shyamtala003/fix-macos-case-insensitive-file-system-for-developers-step-by-step-guide-6d3b1eae13ec>
- Docker 权限：<https://selfhosting.sh/foundations/docker-volume-permissions/>、<https://fixdevs.com/blog/docker-volume-permission-denied/>、<https://www.buildwithmatija.com/blog/how-to-fix-permission-denied-when-manipulating-files-in-docker-container>、<https://dockerpros.com/container-creation-and-management/understanding-permission-issues-with-mounted-volumes/>、<https://stackoverflow.com/questions/60129247/docker-file-permissions-with-volume-bind-mount>
- Docker entrypoint 顺序：<https://stackoverflow.com/questions/38469569/docker-mount-happens-before-or-after-entrypoint-execution>、<https://stackoverflow.com/questions/45027588/docker-entrypoint-to-run-after-volume-mount/71237194>、<https://github.com/panubo/docker-sshd/blob/main/entry.sh>、<https://github.com/panubo/docker-sshd/pull/28>
- Docker volume 清理：<https://oneuptime.com/blog/post/2026-02-08-how-to-clean-up-orphaned-docker-volumes/view>、<https://virtarix.com/blog/technical-guide/how-to-delete-docker-volumes/>
- Docker 镜像优化：<https://oneuptime.com/blog/post/2026-03-02-how-to-write-efficient-dockerfiles-for-ubuntu-based-images/view>、<https://oneuptime.com/blog/post/2026-02-08-how-to-use-run-mounttypecache-for-package-manager-caching/view>、<https://docs.docker.com/build/building/best-practices/>
- CLI UX：<https://jmmv.dev/2013/08/cli-design-error-reporting.html>、<https://www.grizzlypeaksoftware.com/library/cli-error-handling-and-user-friendly-messages-qgugu9kg>、<https://uxdesign.cc/fail-early-a-hidden-design-principle-of-good-products-and-services-b23af66e0247>、<https://www.reddit.com/r/LocalLLaMA/comments/1ro71ou/the_silent_openai_fallback_why_llamaindex_might/>
- nftables / Mosh：<https://oneuptime.com/blog/post/2026-03-20-allow-ssh-traffic-nftables/view>、<http://snippets.khromov.se/iptables-rules-for-mosh-connections/>
- NFS + mergerfs inode：<https://www.diymediaserver.com/post/how-i-fixed-my-24-hour-nfs-crash-loop-with-mergerfs-lxc-and-proxmox/>
- inotify 限制：<https://unix.stackexchange.com/questions/13751/kernel-inotify-watch-limit-reached>

### 项目内部（HIGH 信心）
- `.planning/PROJECT.md`（v2.0 已交付能力 + v3.0 目标 + 性能基线）
- `.planning/MILESTONES.md`（v2.0 完成详情）
- `.planning/RETROSPECTIVE.md`（v1.0 error_code 粗粒度 / v1.1 对称设计 / v2.0 AppArmor + FUSE 教训）

---

*Pitfalls research for: Cloud CLI Proxy v3.0 远端开发体验升级*
*Researched: 2026-04-18*
