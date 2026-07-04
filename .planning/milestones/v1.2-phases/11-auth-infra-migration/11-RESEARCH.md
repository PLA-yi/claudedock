# Phase 11: 认证基础设施与数据迁移 - Research

**Researched:** 2026-03-28
**Domain:** Go JWT 认证、PostgreSQL 迁移、React 前端路由守卫
**Confidence:** HIGH

## Summary

本阶段的核心工作是将现有分散的认证机制（管理员硬编码凭证 + 用户 bootstrap bcrypt + entry 明文密码）统一为一个基于 JWT + bcrypt 的登录端点，同时在 JWT 中携带 role claim 以区分管理员和普通用户。数据模型需要扩展 users 表增加 role 列，废弃 entry_password 明文字段，并创建新的 claude_accounts 表。前端需改造登录页和路由守卫以支持角色感知的跳转逻辑。

现有代码库已经具备大部分基础设施：`golang-jwt/jwt/v5` 用于 JWT 签发和验证，`bcrypt` 用于密码哈希，pgx v5 用于数据库操作，TanStack Router 用于前端路由。改造的重点在于重构和统一，而非从零搭建。

**Primary recommendation:** 以现有 `admin_auth.go` 和 `bootstrap_auth.go` 为基础重构统一认证层，使用自定义 JWT claims 结构体携带 user_id + role，新增通用 AuthMiddleware 替代 AdminAuthMiddleware。

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions
- D-01: 合并管理员硬编码凭证和用户 bootstrap 认证为统一登录端点，所有角色通过 users 表认证并获取带 role claim 的 JWT
- D-02: 登录凭证使用 short_id + 密码，与 AUTH-01 要求一致。管理员也使用 short_id 登录（管理员在 users 表中的 short_id 可设置为如 "admin"）
- D-03: 保留环境变量管理员凭证作为超级后备通道（seed 管理员），但主流程走统一登录
- D-04: users 表添加 `role TEXT NOT NULL DEFAULT 'user'` 列，值为 `admin` 或 `user`
- D-05: 数据库迁移时自动创建种子管理员记录（从环境变量 ADMIN_USERNAME/ADMIN_PASSWORD 读取），并标记 role=admin
- D-06: 现有管理员 API 中间件改为读取 JWT 中的 role claim 鉴权，替代当前的硬编码凭证比对
- D-07: 统一使用 bcrypt + password_hash 列，废弃 entry_password 明文字段
- D-08: entry flow 改为使用 bcrypt 比对 password_hash，保持 short_id 入口不变
- D-09: 创建独立 claude_accounts 表，外键关联 users(user_id) 和 hosts(host_id)
- D-10: 基础字段：id (UUID PK)、user_id (FK)、host_id (FK nullable)、email、display_name、status、created_at、updated_at
- D-11: 管理员和用户共用同一登录页（web/admin/src/routes/login.tsx 改造），输入 short_id + 密码
- D-12: 登录后前端从 JWT 解析 role，admin 跳转 /dashboard，user 跳转 /portal
- D-13: 新增 UserAuthMiddleware，从 JWT 提取 user_id 和 role，注入请求上下文
- D-14: 用户 API 端点校验资源归属（user_id 匹配），不匹配返回 403

### Claude's Discretion
- JWT 过期时间策略（可沿用现有 24h 或调整）
- 种子管理员的 short_id 命名约定
- claude_accounts 表的索引策略
- 登录页 UI 细节（布局、配色保持现有风格即可）

### Deferred Ideas (OUT OF SCOPE)
- 用户自助面板的具体页面内容 — Phase 12
- Claude 账号 CRUD API 和前端 — Phase 13
- KasmVNC 用户直连 — Phase 14
- Bootstrap 短 URL 重设计 — Phase 15

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| AUTH-01 | 用户可使用 short_id + 密码登录，获取带 role claim 的 JWT | 统一登录端点 + 自定义 JWT claims 结构体（见 Architecture Patterns） |
| AUTH-02 | 管理员和用户使用同一登录页，登录后根据角色跳转不同面板 | 前端 JWT 解析 + TanStack Router 路由守卫改造（见前端模式） |
| AUTH-03 | 用户只能访问自己的主机、出口 IP、Claude 账号等资源 | 通用 AuthMiddleware + context 注入 user_id + 资源归属校验（见中间件模式） |
| CLAUDE-01 | 系统支持 claude_accounts 数据模型，一个用户可拥有多个 Claude 账号，每个账号对应一台主机 | 0007 迁移文件 + 新表定义 + 索引策略（见数据模型） |

</phase_requirements>

## Standard Stack

### Core（已在项目中使用）

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang-jwt/jwt/v5 | v5.3.1 | JWT 签发、验证、claims 解析 | 项目已引入，支持自定义 claims 结构体 |
| golang.org/x/crypto/bcrypt | (随 crypto v0.37.0) | 密码哈希与比对 | 项目已引入，bootstrap_auth.go 已在使用 |
| jackc/pgx/v5 | v5.7.6 | PostgreSQL 驱动和连接池 | 项目标准数据库驱动 |
| TanStack Router | ^1.120.0 | 前端文件路由和路由守卫 | 项目已引入 |
| TanStack React Query | ^5.75.0 | 前端数据获取和缓存 | 项目已引入 |
| react-hook-form + zod | ^7.56.0 / ^3.24.0 | 表单校验 | 登录页已在使用 |

### 无需引入新依赖

本阶段所有功能可完全使用现有依赖完成。不需要引入新的 Go 模块或 npm 包。

## Architecture Patterns

### 推荐变更结构

```
internal/controlplane/http/
├── auth.go              # 新：统一登录 handler + 自定义 claims
├── auth_middleware.go   # 新：通用 AuthMiddleware（替代 AdminAuthMiddleware）
├── admin_auth.go        # 改：AdminAuthMiddleware 重构为调用通用中间件
├── bootstrap_auth.go    # 改：复用统一密码校验逻辑
├── entry.go             # 改：废弃明文密码，改用 bcrypt
├── router.go            # 改：注册新端点，挂载新中间件
└── user_api.go          # 新：用户侧 API（Phase 12 填充，本阶段只建骨架）

internal/store/
├── migrations/
│   └── 0007_auth_unification.sql  # role 列 + claude_accounts 表 + 种子管理员
└── repository/
    ├── models.go         # 改：User 增加 Role 字段，新增 ClaudeAccount 模型
    └── queries.go        # 改：查询增加 role 列，新增 GetUserByShortIDForAuth

web/admin/src/
├── lib/auth.ts           # 改：增加 getRole()、parseToken() 工具函数
├── routes/login.tsx      # 改：登录后根据 role 跳转
├── routes/_dashboard.tsx # 改：路由守卫增加角色检查
└── routes/_portal.tsx    # 新：用户面板布局骨架（Phase 12 填充内容）
```

### Pattern 1: 自定义 JWT Claims

**What:** 扩展 JWT RegisteredClaims 加入 user_id 和 role

**Example:**
```go
// 基于 golang-jwt/jwt/v5 的自定义 claims
type AuthClaims struct {
    jwt.RegisteredClaims
    UserID string `json:"user_id"`
    Role   string `json:"role"`   // "admin" | "user"
}

func GenerateToken(secret []byte, userID, role string, expiry time.Duration) (string, error) {
    now := time.Now()
    claims := AuthClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   userID,
            Issuer:    "claudedock",
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
        },
        UserID: userID,
        Role:   role,
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(secret)
}
```

**Confidence:** HIGH - golang-jwt/jwt/v5 对自定义 claims 的支持是其核心功能，项目已有 RegisteredClaims 使用示例。

### Pattern 2: 通用认证中间件 + Context 注入

**What:** 从 JWT 提取 user_id 和 role 注入 Go context，下游 handler 通过 context 获取

**Example:**
```go
type contextKey string

const (
    ctxKeyUserID contextKey = "user_id"
    ctxKeyRole   contextKey = "role"
)

func AuthMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 提取 token（复用现有 Bearer / cookie / query 逻辑）
            tokenStr := extractToken(r)

            claims := &AuthClaims{}
            token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
                return secret, nil
            }, jwt.WithValidMethods([]string{"HS256"}))

            if err != nil || !token.Valid {
                writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
                return
            }

            ctx := context.WithValue(r.Context(), ctxKeyUserID, claims.UserID)
            ctx = context.WithValue(ctx, ctxKeyRole, claims.Role)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// 角色限制中间件
func RequireRole(roles ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            role, _ := r.Context().Value(ctxKeyRole).(string)
            for _, allowed := range roles {
                if role == allowed {
                    next.ServeHTTP(w, r)
                    return
                }
            }
            writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
        })
    }
}
```

**Confidence:** HIGH - Go context 注入是标准中间件模式，现有代码库已使用 middleware 链式调用。

### Pattern 3: 资源归属校验

**What:** 用户 API handler 校验请求的资源是否属于当前用户

**Example:**
```go
func (h *UserHostsHandler) List() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := r.Context().Value(ctxKeyUserID).(string)
        hosts, err := h.store.ListHostsByUserID(r.Context(), userID)
        // ... 只返回该用户的主机
    })
}

func (h *UserHostsHandler) Get() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := r.Context().Value(ctxKeyUserID).(string)
        hostID := r.PathValue("hostID")
        host, err := h.store.GetHost(r.Context(), hostID)
        if host.UserID != userID {
            writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
            return
        }
        // ...
    })
}
```

### Pattern 4: 统一登录端点

**What:** 新建 `/v1/auth/login` 统一登录端点，替代 `/v1/admin/login`

**Example:**
```go
type UnifiedLoginHandler struct {
    store     AuthUserStore  // GetUserByShortIDForAuth
    jwtSecret []byte
    logger    *slog.Logger
}

type loginRequest struct {
    ShortID  string `json:"short_id"`
    Password string `json:"password"`
}

func (h *UnifiedLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    // 1. 查 users 表 by short_id
    // 2. bcrypt 比对 password_hash
    // 3. 检查 status == active
    // 4. 签发带 user_id + role 的 JWT
    // 5. 返回 token + role（前端用 role 决定跳转）
}
```

### Pattern 5: 前端 JWT Role 解析与路由

**What:** 前端解析 JWT payload 提取 role，实现角色感知的路由守卫

**Example:**
```typescript
// lib/auth.ts 扩展
export function parseTokenPayload(): { user_id: string; role: string } | null {
  const token = getToken();
  if (!token) return null;
  try {
    const payload = JSON.parse(atob(token.split('.')[1]));
    return { user_id: payload.user_id, role: payload.role };
  } catch {
    return null;
  }
}

export function getRole(): string | null {
  return parseTokenPayload()?.role ?? null;
}
```

```typescript
// routes/login.tsx 改造
onSuccess: (data) => {
  setToken(data.token);
  const role = data.role; // 后端也返回 role，不仅依赖 JWT 解析
  if (role === 'admin') {
    navigate({ to: '/' }); // 管理面板
  } else {
    navigate({ to: '/portal' }); // 用户面板
  }
},
```

### Pattern 6: 数据库迁移 - 种子管理员

**What:** 在应用启动时（而非迁移 SQL 中）检查并创建种子管理员

**Why:** SQL 迁移无法访问环境变量，种子管理员需要从环境变量读取凭证。

**Example:**
```go
// app.go 启动逻辑中
func (a *App) ensureSeedAdmin(ctx context.Context) error {
    // 检查是否已有 admin role 用户
    _, err := a.repo.GetUserByShortID(ctx, a.cfg.AdminUsername) // short_id = admin username
    if err == nil {
        return nil // 已存在
    }

    hash, err := bcrypt.GenerateFromPassword([]byte(a.cfg.AdminPassword), bcrypt.DefaultCost)
    if err != nil {
        return fmt.Errorf("hash admin password: %w", err)
    }

    _, err = a.repo.CreateUserWithRole(ctx, CreateUserWithRoleParams{
        Username:     a.cfg.AdminUsername,
        PasswordHash: string(hash),
        ShortID:      a.cfg.AdminUsername, // 如 "admin"
        Role:         "admin",
    })
    return err
}
```

### Anti-Patterns to Avoid
- **在 SQL 迁移中写死种子数据:** 迁移应只做 schema 变更，种子数据由应用逻辑处理（因为需要环境变量和 bcrypt）
- **在前端存储 role 到独立 localStorage key:** 应始终从 JWT payload 解析，保持单一数据源
- **用 admin_token 和 user_token 两个 localStorage key:** 统一使用同一个 token key，role 从 token 解析
- **在每个 handler 中重复 JWT 解析逻辑:** 集中在中间件中完成，handler 只从 context 读取

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JWT 签发和验证 | 手写 HMAC 签名 | golang-jwt/jwt/v5（已引入） | 处理 claims 验证、过期、签名算法安全 |
| 密码哈希 | MD5/SHA256 | bcrypt（已引入） | 抗暴力破解，内置 salt |
| 前端 JWT 解析 | 手写 base64 解码 | atob + JSON.parse（标准 API） | JWT payload 是标准 base64url，无需库 |

**Key insight:** 本阶段所有工具链已经存在于项目中，不需要引入新依赖。重点是重构和统一现有逻辑。

## Common Pitfalls

### Pitfall 1: 迁移破坏现有数据

**What goes wrong:** ALTER TABLE ADD COLUMN role 时如果现有用户没有正确的默认值，或 NOT NULL 约束导致迁移失败
**Why it happens:** 现有 users 表已有数据
**How to avoid:** 迁移 SQL 使用 `DEFAULT 'user'` 确保现有行自动获得默认角色；先 ADD COLUMN 再 UPDATE 种子管理员
**Warning signs:** 迁移报错 "null value in column role violates not-null constraint"

### Pitfall 2: 旧 JWT 在切换后失效

**What goes wrong:** 已签发的管理员 JWT（只有 RegisteredClaims）在新中间件中解析失败，因为缺少 user_id 和 role claim
**Why it happens:** 新中间件期望 AuthClaims 结构体，旧 token 没有这些字段
**How to avoid:** 切换应在同一次部署中完成。在新中间件中对缺失的自定义 claim 做防御性处理（role 为空时拒绝访问），并确保部署后管理员重新登录
**Warning signs:** 部署后管理员无法访问后台

### Pitfall 3: entry_password 废弃不彻底

**What goes wrong:** 废弃 entry_password 后，entry.go 中的明文比对仍然使用旧字段
**Why it happens:** 遗漏了 entry.go 的改造
**How to avoid:** 明确列出所有使用 entry_password 的代码路径（entry.go Auth()、models.go User 结构体、queries.go 中的 SELECT 和 INSERT），全部改为使用 password_hash + bcrypt
**Warning signs:** entry 认证仍然接受旧的明文密码

### Pitfall 4: 前端 Cookie 名称冲突

**What goes wrong:** 现有 AdminAuthMiddleware 设置 cookie name 为 `admin_token`，新统一认证如果使用不同的 cookie name，会导致 VNC 代理等依赖 cookie 的功能中断
**Why it happens:** VNC 代理通过 query param 认证时会设置 cookie
**How to avoid:** 统一 cookie 名称为 `admin_token`（或统一改名），确保 VNC 代理路径继续工作
**Warning signs:** VNC 代理页面加载后子资源请求 401

### Pitfall 5: 登录端点路径迁移不彻底

**What goes wrong:** 前端改为调用 `/v1/auth/login`，但 `/v1/admin/login` 没有保留或重定向，导致可能存在的外部集成断裂
**Why it happens:** 只改了新端点没处理旧端点
**How to avoid:** 保留 `/v1/admin/login` 作为兼容端点（内部代理到统一登录），或在 router 中注册两个路径指向同一 handler
**Warning signs:** 使用旧 URL 登录的脚本或工具失败

### Pitfall 6: bcrypt cost 不一致

**What goes wrong:** 种子管理员使用 DefaultCost（10），admin UI 创建用户使用 MinCost（4），导致安全不一致
**Why it happens:** 现有 admin_users.go 创建用户时的 bcrypt cost 可能不同
**How to avoid:** 定义项目级常量 `const BcryptCost = bcrypt.DefaultCost`，所有 bcrypt 调用统一使用
**Warning signs:** 不同来源创建的用户密码哈希强度不一致

## Code Examples

### 数据库迁移 0007_auth_unification.sql

```sql
-- 1. users 表增加 role 列
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'user';

-- 2. claude_accounts 表
CREATE TABLE claude_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    host_id UUID REFERENCES hosts (id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_claude_accounts_user_id ON claude_accounts (user_id);
CREATE INDEX idx_claude_accounts_host_id ON claude_accounts (host_id);
CREATE UNIQUE INDEX idx_claude_accounts_email ON claude_accounts (email);
```

### 统一登录响应格式

```json
{
  "token": "eyJhbGciOi...",
  "role": "admin",
  "expires_in": 86400
}
```

返回 role 字段是为了让前端无需解析 JWT 就能立即跳转。

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| 管理员硬编码 hmac.Equal 比对 | 统一 bcrypt + users 表查询 | Phase 11 | 管理员也走数据库认证 |
| entry_password 明文存储 | password_hash bcrypt 哈希 | Phase 11 | 消除明文密码存储 |
| JWT 只含 RegisteredClaims | JWT 含 user_id + role 自定义 claims | Phase 11 | 支持角色区分和资源隔离 |
| 管理员和用户分离的认证路径 | 统一 `/v1/auth/login` 端点 | Phase 11 | 简化认证架构 |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (标准库) + httptest |
| Config file | 无需配置文件 |
| Quick run command | `go test ./internal/controlplane/http/ -run TestAuth -v -count=1` |
| Full suite command | `go test ./internal/... -v -count=1` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | short_id + 密码登录获取带 role 的 JWT | unit | `go test ./internal/controlplane/http/ -run TestUnifiedLogin -v -count=1` | Wave 0 |
| AUTH-02 | 登录后根据角色返回不同 role | unit | `go test ./internal/controlplane/http/ -run TestUnifiedLogin -v -count=1` | Wave 0 |
| AUTH-03 | 用户只能访问自己的资源 | unit | `go test ./internal/controlplane/http/ -run TestResourceOwnership -v -count=1` | Wave 0 |
| CLAUDE-01 | claude_accounts 表创建成功 | migration | 迁移运行无报错即可验证（集成测试级别）| manual-only |

### Sampling Rate
- **Per task commit:** `go test ./internal/controlplane/http/ -v -count=1`
- **Per wave merge:** `go test ./internal/... -v -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/controlplane/http/auth_test.go` -- 统一登录 handler 测试（AUTH-01, AUTH-02）
- [ ] `internal/controlplane/http/auth_middleware_test.go` -- 通用中间件 + 资源归属测试（AUTH-03）
- 测试模式可复用现有 `bootstrap_auth_test.go` 的 stub pattern（stubUserLookup 等）

## Open Questions

1. **统一登录端点路径**
   - 可选 `/v1/auth/login` 或 `/v1/login`
   - 推荐 `/v1/auth/login`，语义清晰
   - 旧 `/v1/admin/login` 建议保留一个版本周期做兼容

2. **entry flow 是否改为返回 JWT**
   - 当前 entry flow 返回 SSH 连接信息，不返回 JWT
   - 推荐：entry flow 的密码校验改用 bcrypt，但不改返回格式（不返回 JWT），因为 entry flow 的消费者是 shell 脚本
   - 理由：entry flow 和 Web 登录是不同场景，entry 不需要 JWT

3. **claude_accounts 表 email 唯一性**
   - 推荐加 UNIQUE 约束（每个 Claude 账号的 email 应全局唯一）
   - 如果业务允许同一 email 绑定多个账号则去掉

## Sources

### Primary (HIGH confidence)
- 项目源码直接审查：admin_auth.go, bootstrap_auth.go, entry.go, router.go, models.go, queries.go, migrations/
- golang-jwt/jwt/v5 自定义 claims：项目已使用 RegisteredClaims，扩展为自定义结构体是官方文档的标准用法
- bcrypt 使用：项目已在 bootstrap_auth.go 中使用，模式可直接复用

### Secondary (MEDIUM confidence)
- TanStack Router 路由守卫：基于项目现有 `_dashboard.tsx` beforeLoad 模式推断扩展方式
- JWT payload base64 解码：标准 RFC 7519 格式，atob 解码是通用做法

## Project Constraints (from CLAUDE.md)

- 所有面向用户的回复、计划、状态更新和总结默认使用中文
- Go 控制面 + pgx 手写查询，无 ORM
- PostgreSQL 迁移使用递增编号 SQL 文件（下一个编号为 0007）
- UUID 主键 + 外键关联是标准模式
- 前端 TanStack Router 文件路由 + React Query 数据获取
- 不使用 Docker 默认 bridge 网络（与本阶段无关但需知悉）
- GSD 工作流约束：修改文件前应通过 GSD 命令

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 所有依赖已在项目中，无需引入新库
- Architecture: HIGH - 基于现有代码直接重构，模式清晰
- Pitfalls: HIGH - 基于源码审查发现的真实集成点
- Data model: HIGH - SQL 迁移模式与现有 6 个迁移文件一致

**Research date:** 2026-03-28
**Valid until:** 2026-04-28 (稳定领域，30 天有效)
