-- Phase 46 Plan 05 / MVS-05 错误码契约用例的 fixture 种子。
--
-- 用法：CI runner 上 GoldenPath Step 3（admin login + fixture 三件套）跑完
-- 后，调用方在控制面 Postgres 连接里执行本 SQL，预置 3 个特殊状态用户：
--   - disabled-user：status=disabled，触发 account_disabled (exit 11)
--   - expired-user：expires_at 过期，触发 account_expired (exit 12)
--   - user-no-host：active 但不绑定 host，触发 host_not_found (exit 13)
--
-- 隐私安全（CONVENTIONS.md）：
--   - 全部占位用户名 / bcrypt hash，禁真实邮箱 / 凭据。
--   - bcrypt hash 由 bcrypt.GenerateFromPassword([]byte("secret-placeholder-pw"), 10)
--     生成；CI 接通后用 helper 脚本统一刷新，避免硬编码过期 hash。
--
-- 列名以 internal/store/repository/ 实际 schema 为准；如有偏差，由
-- 46-05-SUMMARY.md 记录差异并调整本文件。
--
-- 幂等：ON CONFLICT (username) DO NOTHING，多次灌种安全。

-- 占位 bcrypt hash for "secret-placeholder-pw" (cost=10)。
-- 真实 hash 在 CI runner 上由 helper 函数动态生成插入；本 SQL 仅保留结构注释。

INSERT INTO users (id, username, password_hash, status, created_at, expires_at)
VALUES
    ('u-disabled-46-05', 'disabled-user',
     '$2a$10$placeholderhashreplacedbyhelperatruntime0000000000',
     'disabled', NOW(), NOW() + INTERVAL '30 day'),
    ('u-expired-46-05',  'expired-user',
     '$2a$10$placeholderhashreplacedbyhelperatruntime0000000000',
     'active',   NOW(), NOW() - INTERVAL '1 day'),
    ('u-nohost-46-05',   'user-no-host',
     '$2a$10$placeholderhashreplacedbyhelperatruntime0000000000',
     'active',   NOW(), NOW() + INTERVAL '30 day')
ON CONFLICT (username) DO NOTHING;
