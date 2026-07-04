# Coding Conventions

**Analysis Date:** 2026-05-10

## Communication

- 所有面向用户的回复、计划、状态更新、说明、错误提示与总结，默认必须全部使用中文。
- 除非用户明确要求英文原文或双语，否则不要输出英文或中英混写的自然语言说明。
- 命令、路径、环境变量、协议名、代码标识符、第三方产品名可以保留英文原文，但解释文字必须使用中文。

## Documentation

- 项目规划、实现说明、运行手册、排障记录优先使用中文撰写。
- 需求 ID、文件名、接口字段名等机器可读标识保持原格式，不做翻译。

## Privacy & Security

- 禁止在代码、注释、文档、规划文件或提交信息中写入任何本机绝对路径（如 `/Users/xxx/`、`/home/xxx/`、`C:\Users\xxx\`）。
- 禁止在任何被 git 跟踪的文件中写入真实的 API 密钥、私钥、密码、token、个人邮箱、手机号等敏感信息。
- 涉及路径引用时，一律使用项目根目录的相对路径。
- 涉及示例凭据时，使用明确的占位符（如 `your-secret-here`、`test@example.com`）。
- 每次批量生成或修改 `.planning/`、`.cursor/`、`.claude/` 等工具链文件后，必须检查是否引入了绝对路径或个人信息。

## Naming Patterns

### Files
- Go 源码: 小写 + 下划线分隔（snake_case），如 `admin_hosts.go`, `container_proxy_provider.go`
- Go 测试: 与被测文件同名加 `_test.go`，如 `worker_test.go`
- React 组件: PascalCase，如 `EgressIpDrawer.tsx`, `DataTableShell.tsx`
- React hooks: camelCase 前缀 `use-`，如 `use-auth-sessions.ts`

### Functions / Methods
- Go: PascalCase 导出，camelCase 内部使用
- Go 构造函数: `NewXxx` 或 `NewXxxWithYyy`，如 `NewServer`, `NewLocalManagerWithRunner`
- React 组件: PascalCase，如 `function DashboardLayout()`
- React hooks: camelCase 前缀 `use`，如 `useAuthSessions()`

### Variables
- Go: camelCase 局部变量，PascalCase 导出字段
- TypeScript: camelCase，布尔值前缀 `is`/`has`/`force`，如 `isAuthenticated`, `forceOpen`

### Types / Interfaces
- Go: PascalCase，接口名以功能命名，如 `Provider`, `HealthChecker`, `ContainerResolver`
- TypeScript: PascalCase，Props 类型后缀 `Props`，如 `SSEMessage`, `ConnectionState`

## Code Style

### Go
- 使用标准 `go fmt` 格式化
- 错误处理: 显式检查 `if err != nil`，使用 `fmt.Errorf("...: %w", err)` 包装上下文
- 上下文传递: 所有可能阻塞的函数接收 `context.Context` 作为第一个参数
- 日志: 使用 `log/slog` 结构化日志，key-value 对形式

### TypeScript / React
- 使用 Vite 构建，Tailwind CSS v4 样式
- 组件使用函数式组件 + hooks
- 路由使用 TanStack Router（文件系统路由）
- 状态管理使用 TanStack Query（React Query）

## Import Organization

### Go
顺序:
1. 标准库
2. 第三方库
3. 项目内部包（按路径排序）

示例:
```go
import (
    "context"
    "fmt"
    "log/slog"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/claudedock/claudedock/internal/agentapi"
    "github.com/zaneliu/claudedock/internal/store/repository"
)
```

### TypeScript
顺序:
1. React / 框架
2. 第三方库
3. 项目内部路径别名 `@/...`

## Error Handling

### Go
**Pattern:** 显式错误返回 + 结构化错误码

```go
// 基础设施层
if err != nil {
    return fmt.Errorf("load image.lock runtime spec: %w", err)
}

// 业务层使用 errcodes
if mountCfg.KeepAliveInterval < 15*time.Second {
    fmt.Fprintln(os.Stderr, errcodes.Format(errcodes.SESSION_KEEPALIVE_TOO_AGGRESSIVE, ...))
}
```

### HTTP 错误响应
- 4xx: 客户端错误（认证失败、参数无效、资源不存在）
- 5xx: 服务端错误（数据库连接失败、docker 执行失败）
- 统一格式: `{"error": "message"}` 或业务数据结构

## Database Access Pattern

**Repository + Raw SQL (pgx)**

- 不使用 ORM，直接手写 SQL
- `repository.Repository` 封装 `*pgxpool.Pool`
- 每个表提供 Query/Scan/Create/Update/Delete 方法
- 模型定义在 `internal/store/repository/models.go`
- SQL 查询内联在方法中，无单独 query builder

示例:
```go
func (r *Repository) GetUser(ctx context.Context, userID string) (User, error) {
    var item User
    if err := r.db.QueryRow(ctx, `
        SELECT id::text, username, ... FROM users WHERE id = $1
    `, userID).Scan(&item.ID, ...); err != nil {
        return User{}, fmt.Errorf("get user: %w", err)
    }
    return item, nil
}
```

## API 路由组织

**Location:** `internal/controlplane/http/router.go`

- 使用 Go 1.22+ `net/http` 的 method + path 路由（`mux.Handle("GET /v1/users", ...)`）
- 路由按功能分组：healthz, bootstrap, entry, admin, user
- Admin 路由使用 `adminGuard` JWT + role 中间件
- User 路由使用 `userGuard` JWT + role 中间件
- SSE 端点不使用中间件，由前端自行管理认证

## Testing Conventions

### Go
- 测试文件: `*_test.go`，与被测文件同包
- 单元测试使用标准 `testing` + 少量 `testify` 风格（项目未引入 testify，使用原生断言）
- Mock 通过接口注入 + 测试桩实现
- 包级变量作为测试钩子，如 `var execInContainer = func(...)`, `var TestPanicTrigger = func(...) bool`
- 集成测试标记: `integration_test.go`

### TypeScript
- 项目当前前端测试覆盖较少
- 使用 Vitest（与 Vite 天然契合）

## Module Design

### Go
- 每个 `internal/` 子目录一个包
- 包名与目录名一致（单数小写）
- 接口定义在使用方（consumer）或共享契约包（`agentapi`）
- 避免循环依赖：共享契约放在 `internal/agentapi/`

### React
- 组件按功能域组织：`components/hosts/`, `components/users/`, `components/ui/`
- 共享 hooks 放在 `hooks/`
- API 封装放在 `lib/api.ts`, `lib/portal-api.ts`
- UI 组件基于 Radix UI + Tailwind（`components/ui/` 下的 button, dialog, table 等）

## Function Design

### Go
- 函数签名保持简洁，context 作为第一个参数
- 配置结构体使用 Options 模式或显式 Config struct
- 避免全局状态，必要的单例（如 SSE Hub）提供 `SetLogger` 等初始化方法

### React
- 自定义 hooks 封装数据获取逻辑
- 组件 props 使用解构 + 类型标注
- 表单使用 react-hook-form + zod 校验

---

*Convention analysis: 2026-05-10*
