package repository

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrSystemBypassPresetImmutable 表示尝试删除或修改 is_system=true 的预设。
// 数据层做最小拦截，Phase 46 handler 会把它翻译成 HTTP 403 + 错误码
// BYPASS_PRESET_IMMUTABLE。SQL 层在 updateBypassPresetSQL / deleteBypassPresetSQL
// 同时附加 `is_system = FALSE` WHERE 兜底，即使绕过 Go 层校验也不会误删。
var ErrSystemBypassPresetImmutable = errors.New("bypass preset is system preset and cannot be deleted or modified")

// _ 让 stub 阶段的 import 不被编译器抱怨（Task 2b 会替换 stub 体并真正使用 ctx / json）。
// stub 文件无 db 调用，但保留 import 让 Task 2b 不需要重新整理依赖。
var (
	_ context.Context
	_ json.RawMessage
)

// ---------------------------------------------------------------------------
// SQL 常量（包级 const，供 queries_bypass_test.go 文本断言锁定）
// 命名规范沿用 queries.go 已有模式（listHostsByUserIDSQL 等）。
// 所有 SELECT 用 `id::text` 把 UUID 转为 string；所有占位符使用 $N，禁止 fmt.Sprintf 拼接。
// ---------------------------------------------------------------------------

const listBypassPresetsSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets
	ORDER BY is_system DESC, slug ASC
`

const getBypassPresetBySlugSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE slug = $1
`

const getBypassPresetByIDSQL = `
	SELECT id::text, slug, name, COALESCE(description, ''),
	       is_system, is_force_on, is_active, rules, created_at, updated_at
	FROM host_bypass_presets WHERE id = $1
`

const createBypassPresetSQL = `
	INSERT INTO host_bypass_presets (slug, name, description, is_force_on, is_active, rules)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

// updateBypassPresetSQL 用 COALESCE($N, col) 实现部分字段更新；
// 关键防御：`AND is_system = FALSE` 兜底，系统预设永不被改（即使 Go 层漏了）。
const updateBypassPresetSQL = `
	UPDATE host_bypass_presets SET
		name        = COALESCE($2, name),
		description = COALESCE($3, description),
		is_force_on = COALESCE($4, is_force_on),
		is_active   = COALESCE($5, is_active),
		rules       = COALESCE($6, rules),
		updated_at  = NOW()
	WHERE id = $1 AND is_system = FALSE
	RETURNING id::text, slug, name, COALESCE(description, ''),
	          is_system, is_force_on, is_active, rules, created_at, updated_at
`

// deleteBypassPresetSQL 同样附加 `AND is_system = FALSE` 兜底。
const deleteBypassPresetSQL = `DELETE FROM host_bypass_presets WHERE id = $1 AND is_system = FALSE`

// checkBypassPresetIsSystemSQL 供 Go 层先查 is_system 标志，决定返回
// ErrSystemBypassPresetImmutable 还是 ErrNoRows。
const checkBypassPresetIsSystemSQL = `SELECT is_system FROM host_bypass_presets WHERE id = $1`

// listBypassRulesGlobalOnlySQL 仅返回 scope='global' 的规则（hostID 入参为 nil 时使用）。
const listBypassRulesGlobalOnlySQL = `
	SELECT id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global'
	ORDER BY created_at ASC
`

// listBypassRulesGlobalOrHostSQL 返回 scope='global' 或 scope='host' 且 host_id=$1 的规则。
const listBypassRulesGlobalOrHostSQL = `
	SELECT id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	       is_risky, created_at, updated_at
	FROM host_bypass_rules
	WHERE scope = 'global' OR (scope = 'host' AND host_id = $1)
	ORDER BY scope DESC, created_at ASC
`

const createBypassRuleSQL = `
	INSERT INTO host_bypass_rules (scope, host_id, rule_type, value, note, is_risky)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const updateBypassRuleSQL = `
	UPDATE host_bypass_rules SET
		value      = COALESCE($2, value),
		note       = COALESCE($3, note),
		is_risky   = COALESCE($4, is_risky),
		updated_at = NOW()
	WHERE id = $1
	RETURNING id::text, scope, host_id::text, rule_type, value, COALESCE(note, ''),
	          is_risky, created_at, updated_at
`

const deleteBypassRuleSQL = `DELETE FROM host_bypass_rules WHERE id = $1`

const listBypassBindingsByHostSQL = `
	SELECT id::text, host_id::text, preset_id::text, rule_id::text,
	       enabled, source, created_at
	FROM host_bypass_bindings
	WHERE host_id = $1
	ORDER BY created_at ASC
`

const createBypassBindingSQL = `
	INSERT INTO host_bypass_bindings (host_id, preset_id, rule_id, enabled, source)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id::text, host_id::text, preset_id::text, rule_id::text,
	          enabled, source, created_at
`

const deleteBypassBindingSQL = `DELETE FROM host_bypass_bindings WHERE id = $1`

const listBypassSnapshotsByHostSQL = `
	SELECT id::text, host_id::text, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, created_by::text, created_at
	FROM host_bypass_snapshots
	WHERE host_id = $1
	ORDER BY version DESC
	LIMIT $2
`

const createBypassSnapshotSQL = `
	INSERT INTO host_bypass_snapshots
		(host_id, version, config_hash, whitelist_cidrs_json, whitelist_domains_json, created_by)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id::text, host_id::text, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, created_by::text, created_at
`

const updateBypassSnapshotStatusSQL = `
	UPDATE host_bypass_snapshots SET applied_status = $2 WHERE id = $1
	RETURNING id::text, host_id::text, version, config_hash,
	          whitelist_cidrs_json, whitelist_domains_json,
	          applied_status, created_by::text, created_at
`

// getLatestAppliedBypassSnapshotSQL 返回 host 最近一次 applied_status='applied' 的 snapshot；
// version DESC + LIMIT 1 决定语义（Phase 47 rollback 需要它来定位回滚目标）。
const getLatestAppliedBypassSnapshotSQL = `
	SELECT id::text, host_id::text, version, config_hash,
	       whitelist_cidrs_json, whitelist_domains_json,
	       applied_status, created_by::text, created_at
	FROM host_bypass_snapshots
	WHERE host_id = $1 AND applied_status = 'applied'
	ORDER BY version DESC
	LIMIT 1
`

const insertBypassAuditLogSQL = `
	INSERT INTO host_bypass_audit_log
		(actor_id, actor_ip, action, target_kind, target_id, before, after, note)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id::text, created_at
`

const listBypassAuditLogByTargetSQL = `
	SELECT id::text, actor_id::text, COALESCE(actor_ip, ''), action, target_kind,
	       target_id::text, before, after, COALESCE(note, ''), created_at
	FROM host_bypass_audit_log
	WHERE target_kind = $1 AND target_id = $2
	ORDER BY created_at DESC
`

// ---------------------------------------------------------------------------
// 18 个 Repository 方法 — Task 2a 阶段全部为 panic stub。
// 真实 pgx v5 方法体由 Task 2b 替换；签名与本文件 SQL 常量在 Task 3
// 已经通过反射 + 文本断言 lock 死，Task 2b 不允许改签名也不允许改 SQL 常量。
// ---------------------------------------------------------------------------

func (r *Repository) ListBypassPresets(ctx context.Context) ([]BypassPreset, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) GetBypassPresetBySlug(ctx context.Context, slug string) (BypassPreset, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) GetBypassPresetByID(ctx context.Context, id string) (BypassPreset, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) CreateBypassPreset(ctx context.Context, params CreateBypassPresetParams) (BypassPreset, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) UpdateBypassPreset(ctx context.Context, id string, params UpdateBypassPresetParams) (BypassPreset, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) DeleteBypassPreset(ctx context.Context, id string) error {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) ListBypassRules(ctx context.Context, hostID *string) ([]BypassRule, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) CreateBypassRule(ctx context.Context, params CreateBypassRuleParams) (BypassRule, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) UpdateBypassRule(ctx context.Context, id string, params UpdateBypassRuleParams) (BypassRule, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) DeleteBypassRule(ctx context.Context, id string) error {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) ListBypassBindingsByHost(ctx context.Context, hostID string) ([]BypassBinding, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) CreateBypassBinding(ctx context.Context, params CreateBypassBindingParams) (BypassBinding, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) DeleteBypassBinding(ctx context.Context, id string) error {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) ListBypassSnapshotsByHost(ctx context.Context, hostID string, limit int) ([]BypassSnapshot, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) CreateBypassSnapshot(ctx context.Context, params CreateBypassSnapshotParams) (BypassSnapshot, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) UpdateBypassSnapshotStatus(ctx context.Context, id string, status string) (BypassSnapshot, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) GetLatestAppliedBypassSnapshot(ctx context.Context, hostID string) (BypassSnapshot, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) InsertBypassAuditLog(ctx context.Context, params InsertBypassAuditLogParams) (string, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}

func (r *Repository) ListBypassAuditLogByTarget(ctx context.Context, targetKind, targetID string) ([]BypassAuditLog, error) {
	panic("not implemented in 45-03 Task 2a; see Task 2b")
}
