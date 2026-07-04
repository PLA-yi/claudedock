---
phase: quick-260506-ty7
plan: 01
subsystem: controlplane
key-files:
  created:
    - internal/controlplane/credgen/credgen.go
    - internal/controlplane/credgen/credgen_test.go
    - internal/controlplane/app/seed_admin.go
    - internal/controlplane/app/seed_admin_test.go
  modified:
    - internal/controlplane/http/admin_users.go
    - internal/controlplane/http/admin_hosts.go
    - internal/controlplane/http/ssh_keys.go
    - internal/controlplane/http/ssh_keys_test.go
    - internal/controlplane/app/app.go
tech-stack:
  added:
    - internal/controlplane/credgen (零外部状态凭据生成包)
  patterns:
    - app 包不反向依赖 http 包：凭据生成下沉到独立 credgen 包
    - seedAdminRepo 私有 interface 注入，便于单测 stub
    - planSeedAdminCredentialFix 纯函数，三态独立
    - fail-fast：密钥/指纹生成失败或写库失败立即返回 error，控制面启动失败
    - 幂等补齐：复用 users 表现有 pub/priv 写 ssh_keys，避免两表密钥分裂
decisions:
  - 保留 ssh_keys_test.go 内 validateSSHKeyPair 测试，改用 credgen.GenerateSSHKeyPair 生成密钥
  - RotatePassword 内 inline 20 字符随机密码块保持原样不动（语义独立，与 LoginPassword 16 位不同）
  - 当 users 表已有密钥但 ssh_keys 表缺 auto-generated 行时，CreateSSHKey 复用 users 表现有密钥而非新生成
  - GetUserByLoginIdentifierForAuth 仅返回基础列，已存在用户路径必须再调 GetUser(id) 拿完整凭据列
---

# Phase quick-260506-ty7 Plan 01: 修复种子 admin 凭据缺口

**一句话总结**：将凭据生成函数从 http 包抽至独立 credgen 包，重写 ensureSeedAdmin 使其在全新部署时生成全套 entry_password + ed25519 SSH 密钥对并写入 ssh_keys 表，同时对已存在但缺凭据的种子 admin 做幂等补齐，任何凭据生成或写库失败均 fail-fast。

## Commit 列表

| # | Commit | 类型 | 说明 |
|---|--------|------|------|
| 1 | `5b0105b` | refactor | 抽出 internal/controlplane/credgen 包并迁移 http 调用点 |
| 2 | `7bfc626` | fix | 重写 ensureSeedAdmin（全套凭据 + 幂等补齐 + fail-fast）+ 单测 |
| 3 | `docs commit` | docs | 本 SUMMARY.md |

## 修改文件清单

- **新建**：
  - `internal/controlplane/credgen/credgen.go`（6 个公开 API）
  - `internal/controlplane/credgen/credgen_test.go`（7 条单测）
  - `internal/controlplane/app/seed_admin.go`（seedAdminRepo interface + planSeedAdminCredentialFix + ensureSeedAdminWithRepo）
  - `internal/controlplane/app/seed_admin_test.go`（7 条单测）
- **修改**：
  - `internal/controlplane/http/admin_users.go`（删除 4 个私有 generate* 函数，调用点改 credgen.*）
  - `internal/controlplane/http/admin_hosts.go`（generateShortID → credgen.GenerateShortID）
  - `internal/controlplane/http/ssh_keys.go`（删除 4 个私有 compute/generate 函数，调用点改 credgen.*）
  - `internal/controlplane/http/ssh_keys_test.go`（改用 credgen.GenerateSSHKeyPair）
  - `internal/controlplane/app/app.go`（ensureSeedAdmin 缩为薄壳，清理 imports）

## 测试结果

```
ok  github.com/claudedock/claudedock/internal/controlplane/credgen    0.010s  (7 tests)
ok  github.com/claudedock/claudedock/internal/controlplane/app         0.329s  (7 tests)
ok  github.com/claudedock/claudedock/internal/controlplane/http        0.327s  (2 tests + 既有 admin_users_test 无回归)
ok  github.com/claudedock/claudedock/internal/store/repository         0.011s
ok  github.com/claudedock/claudedock/internal/runtime/tasks            0.203s
```

- `go build ./...` 通过
- `grep` 扫库：http 包外无残留 `func generateEntryPassword` / `func generateSSHKeyPair` / `func computeFingerprint` 等私有函数定义

## 需求完成度

| 需求 ID | 描述 | 状态 |
|---------|------|------|
| QUICK-260506-TY7-01 | ensureSeedAdmin 必须生成 entry_password / ssh_public_key / ssh_private_key | 完成 |
| QUICK-260506-TY7-02 | ensureSeedAdmin 必须写一行 ssh_keys（purpose=inbound, label=auto-generated） | 完成 |
| QUICK-260506-TY7-03 | 已存在但缺凭据的种子 admin 启动时被幂等补齐，不重复插入 ssh_keys 行 | 完成 |
| QUICK-260506-TY7-04 | 凭据生成失败时启动 fail-fast（return error，控制面退出） | 完成 |

## Plan Deviations

### 偏差 1：ssh_keys_test.go 改用 credgen 公开 API
- **发现时机**：Task 1 迁移时
- **原因**：ssh_keys_test.go 原直接调用 http 包内私有 `generateEd25519KeyPair`，迁移后该函数已删除，测试编译失败（Rule 3 阻塞修复）
- **调整**：将测试内 `generateEd25519KeyPair` 替换为 `credgen.GenerateSSHKeyPair("ed25519", ...)`，验证逻辑不变
- **影响范围**：仅测试文件，零行为变更

### 偏差 2：TestEnsureSeedAdmin_PreservesExistingSSHKeyWhenOnlyRowMissing 使用真实 ed25519 密钥
- **发现时机**：Task 2 首次运行测试时
- **原因**：原测试使用假公钥字符串 `ssh-ed25519 AAAAExistingPub admin`，`credgen.ComputeFingerprint` 解析失败返回空，触发 ensureSeedAdminWithRepo 的 fingerprint empty fail-fast 守卫
- **调整**：测试 setup 改用 `credgen.GenerateSSHKeyPair("ed25519", "preexisting")` 生成真实密钥对，确保 ComputeFingerprint 非空
- **影响范围**：仅测试文件，零行为变更

## 待人工验证（线上 smoke）

以下两项需要真实 Docker 环境，无法在本机无 docker daemon 场景下自动化验证：

1. **全新部署**：设置 `ADMIN_USERNAME` / `ADMIN_PASSWORD` 启动后，用 admin 账号在后台创建一个主机，观察是否从"创建中"走到 `running`（不再因 EntryPassword=="" 被 worker 拒绝）。
2. **幂等重启**：升级到本 commit 后重启控制面，对已有 admin 账号执行：
   ```sql
   SELECT entry_password IS NOT NULL, ssh_public_key IS NOT NULL FROM users WHERE role='admin' AND short_id=$ADMIN_USERNAME;
   -- 应全为 t
   SELECT count(*) FROM ssh_keys WHERE user_id=(SELECT id FROM users WHERE short_id=$ADMIN_USERNAME) AND purpose='inbound' AND label='auto-generated';
   -- 应为 1
   ```

## Carry-over

无。全仓目标包测试全部通过，无已知阻塞。
