---
status: partial
phase: 35-e2e
source: [35-05-acceptance-checklist-PLAN.md, 35-05-SUMMARY.md]
started: 2026-04-23T07:08:19Z
updated: 2026-04-23T07:08:19Z
---

# Phase 35 — Human UAT (真机签字)

**Phase 35 自动化部分全部通过；以下三项关键场景需在真机环境跑 `bash scripts/v3-acceptance-checklist.sh` 并签字。**

用户已通过 `/gsd-execute-phase 35` 选择 `skip-real-hardware` 路径（2026-04-23）— 三项签字从"阻塞 phase 完成"降级为"持续跟踪 + ship v3.0 前补签"。

## Current Test

[awaiting real-hardware execution]

## Tests

### 1. M5 — macOS APFS case-insensitive 双向同步无数据丢失

expected: 在 APFS macOS 上同时创建 `Foo.txt`（内容 A）+ `foo.txt`（内容 B），Mutagen 同步窗口（≥10s）后远端容器内 `cat /workspace/Foo.txt` 与 `cat /workspace/foo.txt` 均返回非空且内容互不覆盖

how-to-verify:
```bash
# 终端 1：cloud-claude 连上 fixture 容器
cloud-claude --mount-mode=auto

# 终端 2：跑验收（M5 在 pitfalls track 中）
bash scripts/v3-acceptance-checklist.sh \
  --track=pitfalls --env=macos \
  --target-container=<容器名> \
  --report-md=docs/runbooks/v3-acceptance-report-$(date +%Y%m%d).md

# 期望报告 M5 项 → PASS
```

签字字段：
- 签字人：[待填]
- 机器：[hostname / OS 版本 / CPU / Docker 版本]
- 执行时间：[YYYY-MM-DD HH:MM]
- 报告路径：[docs/runbooks/v3-acceptance-report-YYYYMMDD.md]
- 证据：[Foo.txt 内容 / foo.txt 内容 截图或 git diff]

result: [pending]

### 2. BASE-03 / REQ-F3-C — 2 分钟拔网自动重连

expected: 拔网 2 分钟后，cloud-claude 触发退避序列 ≥ 3 档（1/2/4/8/30s 中至少命中 3 个），网络恢复后 60s 内 tmux session 仍可 attach 并继续输入；最终失败时 stderr 出现 `(按 Enter 重试|cloud-claude doctor)` 中文提示

how-to-verify:
```bash
# 不需要 cloud-claude 已连，脚本会自己启
bash scripts/uat-network-resilience.sh --scenario=2min --target-container=<容器名>

# 或通过主脚本（含 BASE-03 + REQ-F3-C/D 三项）
bash scripts/v3-acceptance-checklist.sh \
  --track=req-f3 --env=macos \
  --target-container=<容器名>
```

签字字段：
- 签字人：[待填]
- 机器：[hostname / 网络断网方式（tc netem / iptables / 物理拔网）]
- 执行时间：[YYYY-MM-DD HH:MM]
- 退避序列实际命中：[列出捕获到的退避秒数]
- 报告路径：[docs/runbooks/v3-acceptance-report-YYYYMMDD.md]

result: [pending]

### 3. C6 — Ubuntu 25.04 AppArmor override 后三路 FUSE 全 PASS

expected: Ubuntu 25.04 (kernel ≥ 6.12) 真机执行 `deploy/scripts/host-preflight.sh` 退出码 0（含 AppArmor override 部署），随后 `verify-fuse-compat.sh` 三路 (mergerfs / sshfs / mutagen) mount 全部 PASS

how-to-verify:
```bash
# Ubuntu 25.04 真机
sudo bash deploy/scripts/host-preflight.sh
bash scripts/v3-acceptance-checklist.sh --track=pitfalls --env=ubuntu25 --target-container=<容器名>
bash scripts/verify-fuse-compat.sh    # 期望三路 mount 全 PASS
```

签字字段：
- 签字人：[待填]
- 机器：[hostname / kernel / VERSION_ID]
- 执行时间：[YYYY-MM-DD HH:MM]
- AppArmor override 路径确认：`/etc/apparmor.d/local/fusermount3` 存在
- 三路 mount 实际状态：[mergerfs / sshfs / mutagen 各自 PASS/FAIL]
- 报告路径：[docs/runbooks/v3-acceptance-report-YYYYMMDD.md]

result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps

（任一项 result 由 pending 变为 failed 时，在此追加 gap 条目并触发 `/gsd-plan-phase 35 --gaps`）

## Resolution Path

签字回流闭环（任一项完成后）：

1. 真机跑脚本生成 `docs/runbooks/v3-acceptance-report-YYYYMMDD.md`
2. 把报告附在 v3.0 release PR description
3. 在本文件对应 `### N.` 块下：
   - `result: [pending]` → `result: passed`
   - 填写"签字字段"四件套（签字人 / 机器 / 执行时间 / 证据路径）
4. 三项全 passed 后：
   - `## Summary` 中 `passed: 3 / pending: 0`
   - 顶部 frontmatter `status: partial → resolved`
   - 提交：`gsd-sdk query commit "test(35): real-hardware UAT signoff complete" .planning/phases/35-e2e/35-HUMAN-UAT.md`
5. 任一项 FAIL 则触发：`/gsd-plan-phase 35 --gaps`（针对该 REQ-ID 单独补丁 phase）
