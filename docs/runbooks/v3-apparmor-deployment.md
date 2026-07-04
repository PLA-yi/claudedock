# Ubuntu 25.04 AppArmor override 部署（v3.0+）

> 适用版本：v3.0 起；对应阶段 Phase 29（v3-worker / D-23 / D-24）
> 关联需求：C6（AppArmor 嵌套 FUSE 拒绝） / SC#9（运维手册收口） / M14（doctor 排障一致性）

---

## 1. 背景

Ubuntu 25.04 起，宿主机 AppArmor 默认 profile 收紧了 `fusermount3` 的能力集，禁止其在已挂载 FUSE 的命名空间内再次嵌套挂载。这会让 v3.0 容器内的「sshfs（cold） + mutagen-agent（hot） + mergerfs（union）」三路并发挂载链中的后续两路直接 EPERM 失败，外在表现为：

- `claudedock doctor mount` 输出 `MOUNT_MERGERFS_FAILED` 或 `MOUNT_SSHFS_FAILED`
- 容器内 `mount | grep fuse` 仅看到一条 sshfs 记录，无 mergerfs / mutagen
- 控制面 audit 出现 `runtime.mount_failed` 事件，metadata.error 含 `Operation not permitted`

证据链（PITFALLS C6 一字不改引用）：Launchpad bug #2111105、moby#50013、sysbox#947、stargz-snapshotter#2144 多源一致结论 — 必须在 `fusermount3` profile（不是 `docker-default`）写入 `capability dac_override,` override。

---

## 2. 适用范围

| 场景 | 处置 |
|------|------|
| Ubuntu < 25.04 | 跳过；fusermount3 profile 默认未收紧，无需 override |
| Ubuntu ≥ 25.04 且未配置 override | **必须按 §4 写入**，否则容器内三路 FUSE 失败 |
| 非 Ubuntu（CentOS / Debian / RHEL / Arch） | 跳过；AppArmor 在这些发行版默认未启用 |
| macOS / Windows | N/A；这些平台没有 AppArmor |

OS 版本判定逻辑（与 `deploy/scripts/host-preflight.sh::check_apparmor_fusermount3` L29-35 一致）：

```bash
. /etc/os-release
[ "${ID:-}" = "ubuntu" ] && {
  ubuntu_major="${VERSION_ID%%.*}"
  ver_rest="${VERSION_ID#*.}"
  ubuntu_minor="${ver_rest%%.*}"
  [ "${ubuntu_major}" -gt 25 ] || \
    { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor:-0}" -ge 4 ]; }
}
```

> 不要用 `uname -r`（kernel 版本 ≠ OS 版本）。

---

## 3. 检测

宿主机执行（root 或 sudo）：

```bash
# 自动检测 + 失败时给出修复指令（advisory，不会自动改宿主机）
bash deploy/scripts/host-preflight.sh

# 三路 FUSE 并发挂载烟测（不依赖宿主机 override，但跑完就能证明 override 已就位）
bash scripts/verify-fuse-compat.sh
```

`host-preflight.sh` 输出含 `host-preflight: AppArmor fusermount3 override OK` 即通过。
失败时 stderr 会输出：

```
host-preflight: FAIL AppArmor override missing — /etc/apparmor.d/local/fusermount3 lacks 'capability dac_override,'
```

`verify-fuse-compat.sh` 阶段 1 会输出：

```
fusermount3 AppArmor profile 已加载 (Ubuntu 25.04+ 可能影响容器内 FUSE 操作)
```

阶段 2 真实 sshfs 挂载若失败，会出现 `[FAIL] sshfs FUSE 挂载: 失败，请检查 AppArmor 配置和 FUSE 设备权限`，此时即说明 override 缺失或失效。

---

## 4. 部署步骤（D-23 字面量，禁止任何字符变更）

宿主机以 root 执行（命令必须**逐字符**与 `host-preflight.sh::check_apparmor_fusermount3` L51-68 输出的 fix 提示一致）。

### 4.1 写入 override

```bash
sudo install -d -m 0755 /etc/apparmor.d/local
sudo tee /etc/apparmor.d/local/fusermount3 >/dev/null <<'EOF'
capability dac_override,
EOF
```

> 文件唯一一行 `capability dac_override,`（含末尾逗号）。多写、漏逗号、多余空白都会导致 `apparmor_parser` 拒绝整 profile。

### 4.2 重载 fusermount3 profile

```bash
sudo apparmor_parser -r /etc/apparmor.d/fusermount3
```

> 重载的是 **`/etc/apparmor.d/fusermount3`**（不是 `docker-default`，更不是 `local/fusermount3`）。
> AppArmor 会自动 include 同名 `local/` 子目录下的文件，所以重载主 profile 即可生效。

### 4.3 验证

```bash
sudo aa-status | grep fusermount3
# 期望: fusermount3 (enforce) 或 (complain)

bash deploy/scripts/host-preflight.sh
# 期望: host-preflight: AppArmor fusermount3 override OK

bash scripts/verify-fuse-compat.sh
# 期望阶段 2 全部 [PASS]
```

`claudedock doctor mount` 应不再输出 `SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` 或 `MOUNT_MERGERFS_FAILED`。

---

## 5. 失败场景与恢复

### 5.1 `apparmor_parser -r` 报语法错误

事件：`apparmor_parser: ERROR processing config files`

排查步骤：

1. `cat /etc/apparmor.d/local/fusermount3` 确认仅一行 `capability dac_override,`
2. 排查多余 BOM / `\r\n` / Tab：`xxd /etc/apparmor.d/local/fusermount3 | head`，应只有 `63 61 70 61 ... 2c 0a`（`capability dac_override,\n`）
3. 立即回滚：

   ```bash
   sudo rm /etc/apparmor.d/local/fusermount3
   sudo apparmor_parser -r /etc/apparmor.d/fusermount3
   ```

### 5.2 容器仍报 `Operation not permitted`

事件：`docker logs <ctr>` 出现 `mount: ... Operation not permitted` 或 `mergerfs: cannot allocate fuse device`

排查步骤：

1. 确认 docker daemon 未把容器 profile 改回 unconfined：

   ```bash
   docker info --format '{{.SecurityOptions}}'
   docker inspect <ctr> --format '{{.HostConfig.SecurityOpt}}'
   ```

2. 容器创建参数应包含 `--cap-add SYS_ADMIN --device /dev/fuse`（worker `createHost` 默认就位，可对照 `internal/runtime/tasks/worker.go`）
3. 确认 fusermount3 profile 真的被加载：`sudo aa-status | grep fusermount3` 必须有 `(enforce)` 或 `(complain)`
4. 如果是 docker engine 22.06+ 自带的 `docker-default` profile 拦截，临时绕过：

   ```bash
   docker run ... --security-opt apparmor=unconfined ...
   ```

   生产环境不建议长期 unconfined；正确做法是按 §4 配置 fusermount3 override。

### 5.3 重启宿主机后 override 失效

AppArmor profile 持久化到 `/etc/apparmor.d/`，重启会自动 include `local/`。如果失效，检查：

```bash
ls -l /etc/apparmor.d/local/fusermount3   # 文件存在
systemctl status apparmor                  # 服务 active
```

恢复：再跑一次 `sudo apparmor_parser -r /etc/apparmor.d/fusermount3`。

---

## 6. 三路并发 FUSE 回归测试

`scripts/verify-fuse-compat.sh` 阶段 2-4 是最小烟测；override 配置后必须全 [PASS]。

期望输出片段：

```
=== 阶段 2: 容器内真实 sshfs FUSE 挂载测试 ===
[INFO]  启动测试容器: fuse-verify-<pid>
[PASS]  /dev/fuse 设备: 可用
[PASS]  user_allow_other 配置: 已启用
[INFO]  执行真实 sshfs FUSE 挂载测试...
[PASS]  sshfs FUSE 挂载: 成功 (mountpoint -q 确认)
[PASS]  FUSE 挂载读取: 成功
[PASS]  FUSE 挂载写入: 成功

=== 阶段 3: 网络策略共存验证 ===
[PASS]  容器内 sshfs 命令: 可用

=== 阶段 4: 端到端流程验证 ===
[PASS]  端到端前置条件: 控制面就绪

========================================
验证结果: N PASS, 0 FAIL, 0 WARN
状态: 全部通过
```

如果阶段 2 出现 `[FAIL] sshfs FUSE 挂载: 失败`，立即回到 §3 检测，确认是否真的写入了 override。

---

## 7. 快速诊断命令

```bash
sudo aa-status | grep fusermount3
cat /etc/apparmor.d/local/fusermount3
bash deploy/scripts/host-preflight.sh
bash scripts/verify-fuse-compat.sh
docker exec <container> mount | grep -E 'fuse|mergerfs|sshfs'
```

---

## 8. 参考

- `deploy/scripts/host-preflight.sh::check_apparmor_fusermount3`（L11-73）— OS 版本闸门 + 工具闸门 + advisory 检测
- `scripts/verify-fuse-compat.sh`（L42-58 AppArmor 检测；阶段 2-4 三路 FUSE 烟测）
- `.planning/research/PITFALLS.md` C6 — Ubuntu 25.04 AppArmor 嵌套 FUSE 证据链
- `internal/claudedock/errcodes/system.go::SYSTEM_APPARMOR_FUSERMOUNT3_MISSING` — doctor 命中码
- `docs/runbooks/v3-doctor-troubleshoot.md` §3.4（mount 维度）— doctor 5 维度联动
- `docs/runbooks/v3-error-code-index.md` — 8 域错误码全集
