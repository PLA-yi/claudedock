---
status: complete
phase: 36-sshfs
source:
  - 36-01-SUMMARY.md
  - 36-02-SUMMARY.md
  - 36-03-SUMMARY.md
  - 36-04-SUMMARY.md
  - 36-05-SUMMARY.md
  - 36-06-SUMMARY.md
started: 2026-04-24T06:16:25Z
updated: 2026-04-24T06:16:25Z
---

## Current Test

[testing complete]

## Tests

### 1. 错误码注册与 explain 输出
expected: |
  cloud-claude explain MOUNT_REQUIRE_GIT_REPO 和 MOUNT_OVERSIZED_FILE_SKIPPED 子进程退出码为 0，
  输出中文长说明 ≥200 字，且不含 "unknown code" 或英文 fallback。
result: pass

### 2. Config.HotSyncMaxFileMB 默认值与 accessor
expected: |
  不设置 hot_sync_max_file_mb 时，EffectiveHotSyncMaxFileMB() 返回 50；
  设置为 100 时返回 100；设置为 0 或负数时返回 50。
result: pass

### 3. LastSessionSnapshot.OversizedFiles 序列化
expected: |
  写入含 OversizedFiles 的 LastSessionSnapshot 后，LoadLastSession 能正确反序列化；
  旧版不含 oversized_files 的 last-session.json 反序列化不报错（向后兼容）。
result: pass

### 4. HotSyncEngine 单文件熔断（initialSync）
expected: |
  60MB 未 ignore 文件在 initialSync 后被标记为 oversized 并 delete，不进入 hot tree；
  30MB 文件正常通过不触发熔断。
result: pass

### 5. HotSyncEngine 单文件熔断（syncOnce 静默跳过）
expected: |
  syncOnce 遇到超阈文件时静默 delete，不写入 oversized 记录，不刷屏 stderr。
result: pass

### 6. mount_strategy 注入 MaxFileBytes 与 stderr 提示
expected: |
  tryModeReal 在 HotOnly 和 Full 两条路径都注入 MaxFileBytes；
  mount 成功后 stderr 一次性输出「[!] 跳过大文件 N 个（>NMB），由 cold 兜底」+ 前 5 条文件列表。
result: pass

### 7. 非 git 目录拒绝挂载
expected: |
  cd /tmp && cloud-claude 立即拒绝挂载，stderr 含 MOUNT_REQUIRE_GIT_REPO + 中文 next_action，
  退出码 = 4（exitConfigError），不发起任何 SSH 连接。
result: pass

### 8. git 检测时序（先于 AuthenticateAndWait）
expected: |
  main.go 中 os.Getwd() 和 requireGitRepo 的调用行号 < AuthenticateAndWait 的调用行号，
  确保 git 检测在发起任何网络请求之前完成。
result: pass

### 9. sshfs 命令含 4 个 FUSE page cache 参数
expected: |
  mount_sshfs.go 生成的 sshfs 命令字面量含 cache=yes,kernel_cache,auto_cache,cache_timeout=300，
  且顺序在 ConnectTimeout=10 之后、-f 之前。
result: pass

### 10. doctor mount 新增 5 项 check
expected: |
  cloud-claude doctor mount --json 输出中 checks 数组比 v3.0 多 5 项：
  require_git_repo / oversized_files_count / sshfs_cache_args / git_proxy_enabled / default_ignore_loaded。
result: pass

### 11. doctor mount check 矩阵测试
expected: |
  go test ./internal/cloudclaude/doctor/... 全部 PASS，含 13 条新增矩阵测试覆盖 pass/warn/fail/skip 全分支。
result: pass

### 12. CI 闸门通过
expected: |
  make ci-gate 全部 PASS，含 go test ./... -count=1 -short + ci-doctor-grep.sh 三段断言通过。
result: pass

## Summary

total: 12
passed: 12
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
