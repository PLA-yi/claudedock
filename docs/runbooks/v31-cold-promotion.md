# 冷文件晋升机制运维手册（v3.1+）

> 适用版本：v3.1 起；对应阶段 Phase 37（冷文件读触发晋升 + e2e UAT）
> 关联需求：REQ-MOUNT-V31-07 / 08 / 09 / 10 / 11 / 12 / 13 / 14

---

## 1. 概述与原理

### 1.1 什么是冷文件晋升

在 v3.1 的 Full 模式三层文件系统中，hot 分支（热同步）只同步源码和 .gitignore 未忽略的文件，大文件和二进制文件通过 cold sshfs 分支提供只读访问。但每次读取 cold 分支的文件都需要通过 sshfs 回源到远端容器，对于频繁读取的二进制文件（如模型权重、数据集），这会带来不可忽视的延迟。

**冷文件晋升（cold promotion）** 解决了这个问题：在用户首次读取 cold 分支的文件时，自动将该文件从 cold 分支通过 SFTP 拉取到 hot 分支。后续读取时，mergerfs 会优先命中 hot 分支（`category.create=ff`），从而消除 sshfs 回源延迟。

### 1.2 数据流

```
用户 cat /workspace/model.bin
        │
        ▼
  mergerfs（分支顺序: hot=RW, cold=RO）
        │
        ▼
  cold sshfs 分支（首次读取）
        │
        ▼
  inotify watcher 检测到 IN_OPEN 事件
        │
        ▼
  PromotionEngine 异步入队
        │
        ▼
  SFTP 拉取 cold:/workspace-cold/model.bin → hot:/tmp/.claudedock-mounts/<hash>/hot/model.bin
        │
        ▼
  晋升完成 → 下次读取 mergerfs 命中 hot 分支（零 RTT）
```

### 1.3 关键组件

| 组件 | 位置 | 职责 |
|------|------|------|
| cold-promoter 进程 | 容器内 goroutine | inotify 监听 cold 分支根目录，捕获读事件 |
| PromotionEngine | cold-promoter 内部 | 异步 SFTP 拉取 + 去重 + 重试 + 熔断 |
| PID file | `~/.claudedock/cold-promoter.pid` | 进程存活标记 |
| last-session.json | `~/.claudedock/last-session.json` | 晋升统计（promotion_count / promotion_bytes / promotion_failed_count） |

---

## 2. 启动与关闭

### 2.1 自动启动

Full 模式 mount 就绪后，cold-promoter 自动启动：
- watcher 开始监听 cold 分支根目录（`/tmp/.claudedock-mounts/<hash>/cold/`）
- PID file 写入 `~/.claudedock/cold-promoter.pid`
- 启动失败时 stderr 输出 `[MOUNT_PROMOTER_FAILED]` 但不阻断 mount

### 2.2 自动回收

mount cleanup 时按 LIFO 顺序回收：
1. cancel watcher ctx → drain 事件队列
2. 等待 PromotionEngine 完成进行中的拉取
3. 清理 PID file
4. 进行后续的 mergerfs / sshfs / hot_sync 清理

### 2.3 异常退出清理

若 claudedock 异常退出（SSH 断连 / panic），下次 mount 启动前自动清理残留：
- 检测 PID file → `kill -0 <pid>` 判断进程是否存活
- 存活则 `kill <pid>` + `rm <pidfile>`
- 不依赖 `pkill -f cold-promoter`（避免多用户环境误杀）

### 2.4 手动关闭

```bash
# 方式 1：环境变量（推荐）
CLOUD_CLAUDE_NO_PROMOTION=1 claudedock

# 方式 2：运行时终止 watcher
kill $(cat ~/.claudedock/cold-promoter.pid)
```

关闭后 cold 分支仍可正常读取（每次回源 sshfs），功能不受影响。

---

## 3. 晋升失败排障

### 3.1 常见失败模式

| 现象 | 可能原因 | 诊断命令 |
|------|---------|---------|
| watcher 不启动 | inotify watch 耗尽 | `cat /proc/sys/fs/inotify/max_user_watches` |
| watcher 不启动 | ~/.claudedock/ 无写权限 | `ls -la ~/.claudedock/cold-promoter.pid` |
| 晋升一直失败 | SFTP 连接断开 | `claudedock doctor mount \| grep sshfs` |
| 晋升一直失败 | hot staging 磁盘满 | `df -h /tmp/.claudedock-mounts/` |
| 特定文件无法晋升 | 文件在熔断列表中 | `cat ~/.claudedock/last-session.json \| jq '.promotion_failed_count'` |

### 3.2 熔断机制

同一文件连续 3 次 SFTP 拉取失败（按 1s/2s/4s 退避重试）后，该文件被永久加入熔断列表。本次会话不再尝试晋升该文件。stderr 输出：

```
[!] 晋升失败 path/to/file.bin: sftp open failed
```

### 3.3 调大 inotify watch 限制

```bash
# 临时调整（重启后失效）
echo 65536 > /proc/sys/fs/inotify/max_user_watches

# 永久调整
echo "fs.inotify.max_user_watches=65536" >> /etc/sysctl.conf
sysctl -p
```

---

## 4. 与 mergerfs / hot_sync 协同

### 4.1 三层文件系统职责

| 层 | 路径 | 权限 | 职责 |
|----|------|------|------|
| hot（hot_sync） | `/tmp/.claudedock-mounts/<hash>/hot/` | RW | 源码双向同步 + 已晋升文件存储 |
| cold（sshfs） | `/tmp/.claudedock-mounts/<hash>/cold/` | RO | 完整文件系统只读镜像 |
| merge（mergerfs） | 用户 cwd | — | union mount（hot=RW 在前，cold=RO 在后） |

### 4.2 晋升文件生命周期

1. inotify 捕获到 cold 分支读事件
2. PromotionEngine 通过 SFTP 从 cold 拉取文件到 hot 分支
3. 文件出现在 hot 分支后，mergerfs 天然命中（`category.create=ff` 优先返回 hot 层结果）
4. 晋升文件在 hot 分支存活到 mount cleanup（会话退出时清理）

**注意：** v3.1 不持久化 hot 分支。会话退出后 hot staging 目录被清理，下次 claudedock 需要重新晋升。跨会话持久缓存为 v3.2 评估项。

### 4.3 不与 hot_sync 轮询冲突

- hot_sync 轮询只同步 hot 分支与本地目录之间的差异（双向）
- hot_sync 的 ignore 规则和单文件大小熔断不影响 PromotionEngine
- PromotionEngine 的写入不触发 hot_sync 的反向同步（hot → local 方向已在 hot_sync 的 last map 中无记录，按 `chooseConflictWinner` 逻辑处理）

### 4.4 mergerfs 命中验证

```bash
# 首次读取：触发晋升
cat model.bin
# 等待晋升完成（最多 5s debounce + 拉取时间）
sleep 6
# 第二次读取：确认 mergerfs 命中 hot 分支
# 通过检查 SFTP read count 不变来验证（e2e UAT 自动化）
```

---

## 5. 错误码反查

本机制涉及的 5 个错误码快速索引：

### MOUNT_PROMOTER_FAILED
- **Severity:** WARN
- **触发:** cold-promoter watcher 启动失败（inotify init / PID file 写失败）
- **影响:** 降级为无晋升模式，cold 仍可读
- **修复:** 检查 inotify 限制 / 目录权限 / 或设 CLOUD_CLAUDE_NO_PROMOTION=1

### MOUNT_HOT_SYNC_FAILED
- **Severity:** ERROR
- **触发:** 热同步 SFTP 初始化或同步失败
- **影响:** 整个 mount 可能降级或失败
- **修复:** 检查当前目录、远端 staging 路径

### MOUNT_SSHFS_FAILED
- **Severity:** ERROR
- **触发:** sshfs 挂载失败
- **影响:** cold 分支不可用
- **修复:** 检查 /dev/fuse 可用性

### MOUNT_SSHFS_DISCONNECTED
- **Severity:** WARN
- **触发:** sshfs 断开 >=15 秒
- **影响:** cold 分支被从 mergerfs 摘除
- **修复:** 网络恢复后运行 `claudedock doctor mount --fix`

### MOUNT_MERGERFS_FAILED
- **Severity:** ERROR
- **触发:** mergerfs 挂载失败
- **影响:** 三层文件系统无法启动
- **修复:** 检查 SYS_ADMIN + /dev/fuse

---

## 6. 快速诊断命令

```bash
# 1. 检查 cold-promoter 是否存活
pgrep -f cold-promoter && echo "watcher 存活" || echo "watcher 未运行"

# 2. 查看晋升统计（从 last-session.json）
cat ~/.claudedock/last-session.json | jq '{promotion_count, promotion_bytes, promotion_failed_count}'

# 3. 查看 hot 分支中已晋升的文件
docker exec $(docker ps --filter label=com.claudedock.managed=true -q | head -1) \
  ls /tmp/.claudedock-mounts/*/hot/ 2>/dev/null

# 4. 查看晋升相关 doctor check
claudedock doctor mount --json | jq '.checks[] | select(.name | startswith("promotion"))'

# 5. 查看 5 个关联错误码的详细说明
claudedock explain MOUNT_PROMOTER_FAILED
claudedock explain MOUNT_HOT_SYNC_FAILED
claudedock explain MOUNT_SSHFS_FAILED
```

---

*文档版本: v1.0 | 最后更新: 2026-04-24 | 对应 Phase 37*
