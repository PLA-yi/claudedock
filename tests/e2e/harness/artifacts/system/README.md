# system/ —— 宿主机系统状态（Phase 52 OBS-01..03 收集）

## 里面是什么

宿主机操作系统快照与 e2e harness 自带的备忘文件：

- `uname.txt` — `uname -a`（kernel 版本 / 架构）
- `free.txt` — `free -m`（Linux）或 `vm_stat`（darwin 兜底）
- `df.txt` — `df -h`（磁盘使用率）
- `dmesg-tail.txt` — `dmesg --time-format=iso | tail -n 100`（内核日志最近 100 行）
- `wait-timeout.txt` — Phase 45 ArtifactDumper 的 WaitFor 超时备忘录（由 `harness.WaitFor` 在超时时追加写入；非脚本采集结果）

## 采集命令

```bash
uname -a > system/uname.txt
free -m  > system/free.txt   # 或 vm_stat（darwin）
df -h    > system/df.txt
dmesg --time-format=iso | tail -n 100 > system/dmesg-tail.txt
# wait-timeout.txt 由 Go 侧 harness/artifacts.go OnWaitForTimeout 写入，不在脚本采集范围
```

## 典型排障场景

1. **内核版本不兼容 sing-box tun** → 看 `uname.txt` kernel 版本。sing-box tun 需要 ≥ 5.6（user namespace tun）。
2. **testcontainer 起来后内存挤爆** → 看 `free.txt`，`available` < 100MB 是危险水位。
3. **host disk 满导致 docker layer 写入失败** → 看 `df.txt`，`/` 和 `/var/lib/docker` 使用率 > 95% 要警惕。
4. **nft / iptables / tun 模块加载失败** → 看 `dmesg-tail.txt`，grep `nft_chain` / `nf_tables` / `tun:` 关键字。
5. **e2e WaitFor 超时（Phase 45 / 46 / 47 各用例）** → 看 `wait-timeout.txt`，每行格式：
   ```
   <RFC3339 timestamp> name=<wait target> last_err=<最后一次断言失败信息>
   ```
   多行追加（多次超时），按时间顺序排查。

## darwin 上为空怎么办

- `uname.txt` / `df.txt` / `free.txt`（vm_stat 输出）darwin 上都有内容，格式与 Linux 不同。
- `dmesg-tail.txt` 在 darwin 上**通常会拒读**（macOS 限制 dmesg 给 root），落 `"dmesg failed (permission denied?)"` 占位。OK，不算 bug。
- `wait-timeout.txt` 仅在 e2e WaitFor 真实超时时才存在；空目录是预期。

## 与 Phase 45 ArtifactDumper 的关系

`wait-timeout.txt` 由 Phase 45 Plan 04 的 `(*ArtifactDumper).OnWaitForTimeout` 写入，文件路径与本目录**重合**。Phase 52 OBS-03 后，Go 侧 `Collect()` 会先建 5 子目录（含本目录），写入 `wait-timeout.txt`，再调脚本子进程把 `uname.txt` / `dmesg-tail.txt` 等填进来。两次写入互不覆盖。

## 备忘录格式扩展

如需在 `wait-timeout.txt` 之外引入新备忘录文件（如 `panic-stacks.txt` / `goroutine-dump.txt`），请：

1. 文件名用 `<kebab-case>.txt`，避免与脚本采集结果重名（脚本结果都是 `<command>.txt` 命名）。
2. 在 PLAN.md / SUMMARY.md 中显式记录，避免「不知道哪来的」垃圾文件。
3. 隐私守护：备忘录内容不允许含真实凭据、个人路径。
