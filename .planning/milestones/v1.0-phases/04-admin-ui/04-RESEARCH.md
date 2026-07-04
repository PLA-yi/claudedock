# Phase 4: 后台管理界面 - Research

**Researched:** 2026-03-27
**Domain:** 后台管理界面（React SPA + Go JWT 认证 + 管理 API）
**Confidence:** HIGH

## Summary

Phase 4 需要从零搭建一个 React SPA 后台管理界面，同时在 Go 控制面补齐管理 API 和 JWT 认证层。前端使用 React 19 + Vite 8 + Tailwind CSS v4 + shadcn/ui 组件体系；后端使用 `golang-jwt/jwt/v5` 实现 HMAC-SHA256 JWT 认证中间件，并基于现有 `net/http` + repository 模式新增用户 CRUD、出口 IP CRUD、绑定管理和密码轮换 API。

现有后端已经具备完整的数据模型（User、Host、EgressIP、HostBinding、Task、Event）、主机生命周期操作队列（create/start/stop/rebuild）和 repository 查询层。前端只需调用后端 REST API，不涉及 SSR 或 BFF 层。开发阶段通过 Vite dev server 代理 `/v1/*` 请求到 Go 后端，部署时 Go 后端直接 serve 前端静态文件。

**Primary recommendation:** 使用 shadcn/ui + Tailwind CSS v4 作为 UI 基础，TanStack Query 管理服务端状态，TanStack Router 做前端路由，golang-jwt/jwt/v5 做后端 JWT 认证。

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** v1 采用单管理员账号 + JWT 认证。管理员凭证通过环境变量或首次配置设定，登录后签发 JWT，前端所有 API 请求携带 Bearer Token。
- **D-02:** v1 不做多角色 RBAC，后台所有功能对已认证管理员全部开放。
- **D-03:** JWT 应设置合理过期时间，过期后前端引导重新登录。
- **D-04:** 采用侧边栏导航 + 顶栏布局。侧边栏分为：仪表板概览、用户管理、出口 IP 管理、主机管理、任务列表。
- **D-05:** 仪表板概览展示关键计数（活跃用户数、运行中主机数、可用出口 IP 数）和最近任务状态摘要。
- **D-06:** 前端使用 React 19.2 + Vite 8.x 搭建，与后端 Go API 分离部署（开发阶段 Vite dev server 代理到后端）。
- **D-07:** 创建用户时管理员手动设定初始密码，后端使用 bcrypt 哈希存储（沿用 Phase 3 决策）。
- **D-08:** 删除用户时弹窗确认 + 要求输入用户名二次确认，防止误操作。删除会级联清理关联主机与绑定。
- **D-09:** 密码轮换：管理员点击按钮系统自动生成随机强密码，界面展示一次供复制，不支持找回。
- **D-10:** 禁用用户后，该用户无法通过 bootstrap 流程认证，但不影响已运行主机的即时状态。
- **D-11:** 出口 IP 的创建和编辑使用右侧抽屉表单，不离开列表页面。
- **D-12:** 绑定操作在用户详情页完成：管理员在用户/主机视图中选择可用出口 IP 进行绑定。
- **D-13:** 运行中主机不允许直接换绑出口 IP，必须先停机再操作。
- **D-14:** 解绑出口 IP 时如果主机正在运行，界面应给出警告并阻止操作。
- **D-15:** 启动/停止/重建操作均通过现有异步任务链路（Phase 1 D-13），后台界面提交后显示任务状态。
- **D-16:** 重建操作时提供模式选择：默认"保留主目录并重置系统层"，可选"工厂重置"。
- **D-17:** 主机当前状态（pending/running/stopped/failed）在列表和详情页实时可见。

### Claude's Discretion
- 具体的 UI 组件库选择（可用 Tailwind CSS + headless 组件，也可用其他轻量方案）。
- 前端路由组织方式和状态管理策略。
- JWT 签名算法、过期时间和刷新机制的具体实现细节。
- 仪表板概览的数据刷新策略（轮询间隔、手动刷新按钮等）。
- 表格分页、排序、筛选的具体交互细节。

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| USER-01 | 管理员可以创建、禁用和删除用户账号 | Go 后端用户 CRUD API + 前端用户管理页面，bcrypt 密码哈希沿用 Phase 3 |
| USER-02 | 管理员可以设置或轮换用户在启动流程中使用的登录密码 | 密码轮换 API（`crypto/rand` 生成随机密码）+ 前端一次性展示 |
| LIFE-01 | 管理员可以启动某个用户的容器化云主机 | 复用现有 `POST /v1/hosts/{hostID}/start` 异步任务 API |
| LIFE-02 | 管理员可以停止某个用户的容器化云主机 | 复用现有 `POST /v1/hosts/{hostID}/stop` 异步任务 API |
| LIFE-03 | 管理员可以基于受管镜像模板重建某个用户的云主机 | 复用现有 `POST /v1/hosts/{hostID}/rebuild` API + 重建模式选择 |
| ADMN-01 | 管理员可以创建、编辑、启用、禁用和删除出口 IP 资源 | Go 后端出口 IP CRUD API + 前端抽屉表单 |
| ADMN-02 | 管理员可以把出口 IP 资源绑定到用户或用户主机策略上 | 绑定/解绑 API + 前端绑定选择器 + 运行中主机保护逻辑 |
| ADMN-03 | 管理员可以在管理系统中查看用户、容器、出口 IP 绑定、生命周期和到期状态 | 仪表板概览 API + 各资源列表/详情页 |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| React | 19.2.4 | UI 框架 | 当前官方稳定版，shadcn/ui 和 TanStack 生态完整支持 |
| Vite | 8.0.3 | 构建与开发服务器 | 当前稳定主版本，原生 Tailwind v4 插件支持，HMR 速度极快 |
| TypeScript | 5.x | 类型安全 | Vite 模板默认包含，与 TanStack Router 类型推导配合最佳 |
| Tailwind CSS | 4.2.2 | 样式系统 | v4 CSS-first 配置、零配置内容检测、Vite 原生插件 |
| shadcn/ui | latest | UI 组件集合 | 基于 Radix UI 原语的可定制组件，60+ 生产级组件 |
| TanStack Query | 5.95.2 | 服务端状态管理 | 处理 API 缓存、后台刷新、乐观更新的事实标准 |
| TanStack Router | 1.168.7 | 前端路由 | 全类型推导路由，搜索参数类型安全，优于 React Router v7 |
| TanStack Table | 8.21.3 | 数据表格 | Headless 表格引擎，与 shadcn/ui DataTable 无缝集成 |
| golang-jwt/jwt | v5.3.1 | Go 端 JWT 签发与验证 | Go 生态 JWT 事实标准，支持 HMAC/RSA/ECDSA |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| lucide-react | 1.7.0 | 图标库 | shadcn/ui 默认图标方案，按需导入 |
| react-hook-form | 7.72.0 | 表单管理 | 处理创建/编辑表单的校验和提交状态 |
| zod | 4.3.6 | Schema 校验 | 前端表单校验和 API 响应类型验证 |
| @hookform/resolvers | 5.2.2 | 表单校验桥接 | 连接 zod schema 到 react-hook-form |
| sonner | latest | Toast 通知 | shadcn/ui 推荐的通知方案，操作反馈 |
| golang.org/x/crypto | (已引入) | bcrypt 密码哈希 | 沿用 Phase 3，用户密码创建和轮换 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| TanStack Router | React Router v7 | React Router 生态更大但类型支持弱，且维护速度下降 |
| shadcn/ui | Ant Design / MUI | 开箱即用但体积大、定制难度高，与 Tailwind 生态冲突 |
| TanStack Query | SWR | TanStack Query 功能更全面（mutation、乐观更新、devtools） |
| golang-jwt/jwt v5 | go-jwt-middleware v3 (Auth0) | Auth0 中间件更重、针对 Auth0 集成优化，单管理员场景过度设计 |

**Installation:**

前端（在 `web/admin/` 目录下）：
```bash
npm create vite@latest web/admin -- --template react-ts
cd web/admin
npm install tailwindcss @tailwindcss/vite
npm install @tanstack/react-query @tanstack/react-router @tanstack/react-table
npm install react-hook-form zod @hookform/resolvers
npm install lucide-react sonner
npx shadcn@latest init
```

后端（Go 模块）：
```bash
go get github.com/golang-jwt/jwt/v5@v5.3.1
```

**Version verification:**
- react: 19.2.4（npm registry 确认 2026-03-27）
- vite: 8.0.3（npm registry 确认 2026-03-27）
- tailwindcss: 4.2.2（npm registry 确认 2026-03-27）
- @tanstack/react-query: 5.95.2（npm registry 确认 2026-03-27）
- @tanstack/react-router: 1.168.7（npm registry 确认 2026-03-27）
- @tanstack/react-table: 8.21.3（npm registry 确认 2026-03-27）
- golang-jwt/jwt: v5.3.1（pkg.go.dev 确认）

## Architecture Patterns

### Recommended Project Structure

```
web/admin/                       # 前端项目根目录
├── src/
│   ├── main.tsx                 # 应用入口
│   ├── App.tsx                  # 根组件 + QueryClientProvider + RouterProvider
│   ├── routeTree.gen.ts         # TanStack Router 自动生成的路由树
│   ├── routes/
│   │   ├── __root.tsx           # 根路由（认证守卫 + 布局切换）
│   │   ├── login.tsx            # 登录页
│   │   ├── _dashboard.tsx       # 侧边栏布局（认证后区域）
│   │   ├── _dashboard/
│   │   │   ├── index.tsx        # 仪表板概览
│   │   │   ├── users/
│   │   │   │   ├── index.tsx    # 用户列表
│   │   │   │   └── $userId.tsx  # 用户详情/编辑
│   │   │   ├── egress-ips/
│   │   │   │   └── index.tsx    # 出口 IP 列表 + 抽屉
│   │   │   ├── hosts/
│   │   │   │   ├── index.tsx    # 主机列表
│   │   │   │   └── $hostId.tsx  # 主机详情
│   │   │   └── tasks/
│   │   │       └── index.tsx    # 任务列表
│   ├── components/
│   │   ├── ui/                  # shadcn/ui 组件（自动生成）
│   │   ├── layout/
│   │   │   ├── sidebar.tsx      # 侧边栏导航
│   │   │   ├── topbar.tsx       # 顶栏
│   │   │   └── dashboard-layout.tsx
│   │   ├── users/               # 用户管理相关组件
│   │   ├── egress-ips/          # 出口 IP 相关组件
│   │   └── hosts/               # 主机管理相关组件
│   ├── lib/
│   │   ├── api.ts               # fetch 封装 + JWT token 注入
│   │   ├── auth.ts              # 认证状态管理
│   │   └── utils.ts             # shadcn/ui cn() 等工具
│   ├── hooks/
│   │   ├── use-auth.ts          # 认证状态 hook
│   │   └── queries/             # TanStack Query hooks
│   │       ├── use-users.ts
│   │       ├── use-hosts.ts
│   │       ├── use-egress-ips.ts
│   │       └── use-tasks.ts
│   └── index.css                # Tailwind 入口
├── vite.config.ts
├── tsconfig.json
├── components.json              # shadcn/ui 配置
└── package.json
```

后端新增 API 结构：
```
internal/controlplane/http/
├── admin_auth.go                # JWT 签发（POST /v1/admin/login）
├── admin_middleware.go          # JWT 验证中间件
├── admin_users.go               # 用户 CRUD API
├── admin_egress_ips.go          # 出口 IP CRUD API
├── admin_bindings.go            # 绑定管理 API
├── admin_dashboard.go           # 仪表板统计 API
├── static.go                   # 生产环境静态文件 serve
└── router.go                   # 路由注册（扩展现有）
```

### Pattern 1: JWT 认证中间件（Go net/http）

**What:** 基于 `golang-jwt/jwt/v5` 的 HMAC-SHA256 中间件，保护 `/v1/admin/*` 路由。
**When to use:** 所有管理 API 路由。

```go
func AdminAuthMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tokenStr := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
            if tokenStr == "" {
                writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing token"})
                return
            }
            token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
                if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                    return nil, fmt.Errorf("unexpected signing method")
                }
                return secret, nil
            }, jwt.WithValidMethods([]string{"HS256"}))
            if err != nil || !token.Valid {
                writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

### Pattern 2: TanStack Query + API 封装（前端）

**What:** 统一的 fetch 封装 + 自动 JWT 注入 + Query/Mutation hooks。
**When to use:** 所有前端数据获取和变更操作。

```typescript
// lib/api.ts
const API_BASE = "/v1/admin";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const token = localStorage.getItem("admin_token");
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init?.headers,
    },
  });
  if (res.status === 401) {
    localStorage.removeItem("admin_token");
    window.location.href = "/login";
    throw new Error("Unauthorized");
  }
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

// hooks/queries/use-users.ts
export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => apiFetch<{ users: User[] }>("/users"),
  });
}

export function useCreateUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateUserPayload) =>
      apiFetch("/users", { method: "POST", body: JSON.stringify(data) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["users"] }),
  });
}
```

### Pattern 3: TanStack Router 认证守卫

**What:** 在路由层拦截未认证访问，重定向到登录页。
**When to use:** `_dashboard` 布局路由。

```typescript
// routes/_dashboard.tsx
export const Route = createFileRoute("/_dashboard")({
  beforeLoad: ({ location }) => {
    if (!isAuthenticated()) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: DashboardLayout,
});
```

### Pattern 4: Vite Dev Proxy

**What:** 开发环境将 API 请求代理到 Go 后端。
**When to use:** 开发阶段避免 CORS。

```typescript
// vite.config.ts
export default defineConfig({
  server: {
    proxy: {
      "/v1": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
```

### Pattern 5: Go 后端 Serve 静态文件（生产环境）

**What:** 生产环境用 Go `http.FileServer` 或 `embed` 直接 serve 前端构建产物。
**When to use:** 单进程部署。

```go
//go:embed web/admin/dist
var adminUI embed.FS

func serveAdminUI(mux *http.ServeMux) {
    fsys, _ := fs.Sub(adminUI, "web/admin/dist")
    fileServer := http.FileServer(http.FS(fsys))
    mux.Handle("/admin/", http.StripPrefix("/admin", fileServer))
}
```

### Anti-Patterns to Avoid

- **在前端硬编码 API URL:** 使用 Vite 代理 + 相对路径，环境切换零改动。
- **用 Redux/Zustand 管理服务端状态:** TanStack Query 专门处理服务端缓存，不要用客户端状态管理库重新发明。
- **在每个组件里单独处理 loading/error 状态:** 利用 TanStack Query 的 `suspense` 模式和 React Error Boundary 统一处理。
- **JWT token 存 cookie 再由前端 JS 读取:** v1 单管理员场景用 `localStorage` + Bearer 头即可，httpOnly cookie 增加 CSRF 复杂度。
- **管理 API 不加认证中间件就暴露:** 所有 `/v1/admin/*` 必须经过 JWT 中间件。

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| UI 组件（按钮、对话框、表格等） | 自定义 CSS 组件 | shadcn/ui (Radix UI) | 可访问性、键盘导航、焦点管理极难手写正确 |
| 数据表格排序/分页/筛选 | 自定义表格组件 | TanStack Table + shadcn/ui DataTable | 虚拟化、排序算法、列拖拽等边界情况太多 |
| API 缓存和后台刷新 | 自定义 fetch + useState | TanStack Query | 请求去重、缓存失效、乐观更新自己写容易出错 |
| 表单校验 | 手写 if/else 校验 | react-hook-form + zod | 嵌套对象、数组字段、异步校验复杂度高 |
| JWT 签发与验证 | 手写 base64 + HMAC | golang-jwt/jwt/v5 | 时钟偏移、算法混淆攻击等安全边界需要库处理 |
| 密码哈希 | 手写 SHA256 | bcrypt (golang.org/x/crypto) | bcrypt 自动处理 salt 和工作因子，抗彩虹表攻击 |
| 随机密码生成 | math/rand | crypto/rand | math/rand 可预测，密码场景必须用密码学安全随机源 |
| 路由参数类型安全 | 手动 parse URL params | TanStack Router | 编译期类型推导捕获路由 typo，运行时不出错 |

**Key insight:** 后台管理界面的大部分复杂度在于交互细节（表格状态、表单校验、认证流转），而非业务逻辑。使用成熟组件库和 headless 工具可以把精力集中在业务流程上。

## Common Pitfalls

### Pitfall 1: JWT Token 过期时前端静默失败
**What goes wrong:** Token 过期后 API 返回 401，但前端没有统一拦截逻辑，导致页面表现为"数据空白"或组件报错。
**Why it happens:** 没有在 API 封装层统一处理 401 响应。
**How to avoid:** 在 `apiFetch` 封装中拦截 401，清除本地 token 并重定向到登录页。TanStack Query 的 `onError` 回调也可做全局处理。
**Warning signs:** 登录后长时间不操作，再点击按钮无反应。

### Pitfall 2: 删除/禁用用户后关联数据悬挂
**What goes wrong:** 删除用户后其主机记录、绑定记录没有清理干净，导致列表显示孤立数据。
**Why it happens:** 数据库有 `ON DELETE CASCADE`（hosts 表），但 host_egress_bindings 也级联删除需要确认。应用层可能还需要额外清理（如停止运行中容器）。
**How to avoid:** 删除用户前先检查是否有运行中主机，若有则先停止。依赖数据库 CASCADE 处理记录清理，但生命周期操作需要应用层协调。
**Warning signs:** 主机列表中出现 user_id 指向不存在用户的记录。

### Pitfall 3: 运行中主机的出口 IP 解绑/换绑
**What goes wrong:** 管理员在主机运行时换绑出口 IP，导致网络隧道断裂、流量泄漏。
**Why it happens:** Phase 2 D-03 明确禁止运行时隐式换 IP，但 API 层可能没有检查主机状态。
**How to avoid:** 绑定/解绑 API 必须检查关联主机状态，运行中（status=running）时返回 409 Conflict 错误。前端在调用前也做状态检查并显示警告。
**Warning signs:** 绑定操作成功但主机网络异常。

### Pitfall 4: 前端状态与后端不同步
**What goes wrong:** 管理员执行了启动/停止操作后，列表页的状态标签没有更新，以为操作失败又重复操作。
**Why it happens:** 生命周期操作是异步任务，返回的是 task_id 而非最终状态。前端没有轮询或 invalidate 缓存。
**How to avoid:** 提交生命周期操作后，使用 TanStack Query 的 `invalidateQueries` 刷新主机列表。仪表板和列表页使用定时 `refetchInterval` 保持数据新鲜度。
**Warning signs:** 操作后状态停留在旧值，需要手动刷新页面。

### Pitfall 5: Tailwind CSS v4 配置方式变化
**What goes wrong:** 按照 v3 文档创建 `tailwind.config.js` 和 `postcss.config.js`，导致配置不生效或重复加载。
**Why it happens:** v4 完全改用 CSS-first 配置和 Vite 原生插件，不再需要 JS 配置文件和 PostCSS。
**How to avoid:** 使用 `@tailwindcss/vite` 插件，CSS 入口只需 `@import "tailwindcss"`，主题定制用 CSS `@theme` 指令。
**Warning signs:** 样式不生效、HMR 不触发样式更新、控制台 PostCSS 警告。

### Pitfall 6: shadcn/ui 初始化路径别名配置遗漏
**What goes wrong:** `npx shadcn@latest init` 后组件 import 使用 `@/components/ui/button` 但 TS 和 Vite 都无法解析。
**Why it happens:** 需要同时在 `tsconfig.json`（`paths`）和 `vite.config.ts`（`resolve.alias`）配置 `@` 别名。
**How to avoid:** 初始化前确保 `tsconfig.json` 的 `compilerOptions.baseUrl` 和 `paths` 已配置，`vite.config.ts` 用 `path.resolve` 配置对应 alias。
**Warning signs:** TS 编辑器报 "Cannot find module '@/...'" 错误。

## Code Examples

### 管理员登录 API（Go 后端）

```go
// internal/controlplane/http/admin_auth.go
func (h *adminAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
        return
    }

    if req.Username != h.adminUsername || !hmac.Equal([]byte(req.Password), []byte(h.adminPassword)) {
        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
        return
    }

    claims := jwt.RegisteredClaims{
        Subject:   "admin",
        IssuedAt:  jwt.NewNumericDate(time.Now()),
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
        Issuer:    "claudedock",
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    tokenStr, err := token.SignedString(h.jwtSecret)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token generation failed"})
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "token":      tokenStr,
        "expires_in": 86400,
    })
}
```

### 用户 CRUD API（Go 后端）

```go
// POST /v1/admin/users — 创建用户
func (h *adminUsersHandler) create(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    // ... decode + validate ...

    hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "password hash failed"})
        return
    }

    user, err := h.repo.CreateUser(r.Context(), req.Username, string(hash))
    // ... error handling + writeJSON response ...
}

// POST /v1/admin/users/{userID}/rotate-password — 密码轮换
func (h *adminUsersHandler) rotatePassword(w http.ResponseWriter, r *http.Request) {
    userID := r.PathValue("userID")
    newPassword := generateSecurePassword(20) // crypto/rand
    hash, _ := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
    _ = h.repo.UpdateUserPassword(r.Context(), userID, string(hash))
    writeJSON(w, http.StatusOK, map[string]any{"new_password": newPassword})
}
```

### 仪表板统计 API（Go 后端）

```go
// GET /v1/admin/dashboard/stats
func (h *dashboardHandler) stats(w http.ResponseWriter, r *http.Request) {
    stats, err := h.repo.GetDashboardStats(r.Context())
    writeJSON(w, http.StatusOK, map[string]any{
        "active_users":    stats.ActiveUsers,
        "running_hosts":   stats.RunningHosts,
        "available_ips":   stats.AvailableIPs,
        "recent_tasks":    stats.RecentTasks,
    })
}
```

### 前端登录页

```typescript
// routes/login.tsx
function LoginPage() {
  const navigate = useNavigate();
  const form = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const loginMutation = useMutation({
    mutationFn: (data: LoginFormData) =>
      fetch("/v1/admin/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      }).then((r) => r.json()),
    onSuccess: (data) => {
      localStorage.setItem("admin_token", data.token);
      navigate({ to: "/" });
    },
  });

  return (
    <form onSubmit={form.handleSubmit((d) => loginMutation.mutate(d))}>
      <Input {...form.register("username")} placeholder="管理员用户名" />
      <Input {...form.register("password")} type="password" placeholder="密码" />
      <Button type="submit" disabled={loginMutation.isPending}>
        {loginMutation.isPending ? "登录中..." : "登录"}
      </Button>
    </form>
  );
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Tailwind CSS JS config | Tailwind CSS v4 CSS-first @theme | 2025-01 | 无需 JS 配置文件，Vite 原生插件 |
| React Router v6 | TanStack Router（类型安全路由） | 2024-2025 | 编译期路由参数类型推导 |
| CRA (Create React App) | Vite 8 | 2023 onwards | CRA 已停止维护，Vite 是 React 官方推荐 |
| 手动 fetch + useState | TanStack Query v5 | 2023-2024 | 声明式数据获取，自动缓存管理 |
| PostCSS + Tailwind 插件 | @tailwindcss/vite 原生插件 | 2025-01 (v4) | 构建速度提升 5-100x |

**Deprecated/outdated:**
- Create React App: 已停止维护，React 官方不再推荐。
- Tailwind CSS v3 配置方式: `tailwind.config.js` + `postcss.config.js` 在 v4 已不需要。
- dgrijalva/jwt-go: 已弃用，被 golang-jwt/jwt 接替。

## Open Questions

1. **前端项目目录命名**
   - What we know: CONTEXT.md 未锁定前端项目目录名。
   - What's unclear: 放在 `web/admin/` 还是 `frontend/` 或 `admin-ui/`。
   - Recommendation: 使用 `web/admin/`，与 Go 项目 `cmd/`、`internal/` 并列，语义清晰。

2. **JWT 密钥管理**
   - What we know: D-01 说管理员凭证通过环境变量设定。
   - What's unclear: JWT 签名密钥是否也从环境变量读取，还是自动生成。
   - Recommendation: JWT 密钥从环境变量 `ADMIN_JWT_SECRET` 读取，启动时若未设置则报错退出（不自动生成，防止重启后所有 token 失效）。

3. **生产部署模式**
   - What we know: D-06 提到分离部署，开发阶段 Vite proxy。
   - What's unclear: 生产是用 Go embed 嵌入还是 Nginx 反代。
   - Recommendation: v1 用 Go embed 嵌入前端构建产物，单二进制部署。后续可切换到 Nginx 反代。

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Node.js | 前端构建工具链 | ✓ | v22.12.0 | — |
| npm | 前端包管理 | ✓ | 10.9.0 | — |
| Go | 后端 API 开发 | ✓ | 1.25.7 | — |
| PostgreSQL | 数据持久化 | ✓（Phase 1 已部署） | — | — |

**Missing dependencies with no fallback:** None — all dependencies are available.

**Missing dependencies with fallback:** None.

**Note:** Node.js 版本 22.12.0 略低于 CLAUDE.md 推荐的 Node.js 24 LTS，但 Vite 8 和 React 19 在 Node.js 22 上完全兼容，不影响开发和构建。生产部署只需 Go 二进制，不需要 Node.js 运行时。

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Vitest (前端) + Go testing (后端) |
| Config file | `web/admin/vitest.config.ts` (Wave 0 创建) |
| Quick run command | `cd web/admin && npx vitest run --reporter=verbose` |
| Full suite command | `cd web/admin && npx vitest run && cd ../.. && go test ./internal/controlplane/http/...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| USER-01 | 创建、禁用、删除用户 API | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminUsers -v` | ❌ Wave 0 |
| USER-02 | 密码设置与轮换 API | unit (Go) | `go test ./internal/controlplane/http/ -run TestRotatePassword -v` | ❌ Wave 0 |
| LIFE-01 | 管理员启动主机 | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminHostStart -v` | ❌ Wave 0 |
| LIFE-02 | 管理员停止主机 | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminHostStop -v` | ❌ Wave 0 |
| LIFE-03 | 管理员重建主机 | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminHostRebuild -v` | ❌ Wave 0 |
| ADMN-01 | 出口 IP CRUD API | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminEgressIP -v` | ❌ Wave 0 |
| ADMN-02 | 出口 IP 绑定/解绑 API | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminBindings -v` | ❌ Wave 0 |
| ADMN-03 | 仪表板统计查询 | unit (Go) | `go test ./internal/controlplane/http/ -run TestDashboardStats -v` | ❌ Wave 0 |
| D-01 | JWT 认证中间件 | unit (Go) | `go test ./internal/controlplane/http/ -run TestAdminAuth -v` | ❌ Wave 0 |
| D-13/14 | 运行中主机绑定保护 | unit (Go) | `go test ./internal/controlplane/http/ -run TestBindingProtection -v` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/controlplane/http/... -v`
- **Per wave merge:** `go test ./internal/controlplane/http/... -v && cd web/admin && npx vitest run`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/controlplane/http/admin_auth_test.go` — JWT 登录和中间件测试
- [ ] `internal/controlplane/http/admin_users_test.go` — 用户 CRUD 测试
- [ ] `internal/controlplane/http/admin_egress_ips_test.go` — 出口 IP CRUD 测试
- [ ] `internal/controlplane/http/admin_bindings_test.go` — 绑定管理测试
- [ ] `web/admin/vitest.config.ts` — 前端测试框架配置
- [ ] 新增 repository 查询方法的测试

## Project Constraints (from CLAUDE.md)

- **沟通语言:** 所有面向用户的回复默认使用中文。
- **部署方式:** v1 仅支持单台 Linux 宿主机。
- **技术栈:** 前端 React 19.2 + Vite 8.x；后端 Go `net/http` + `pgx`。
- **网络安全:** 出口 IP 绑定语义和运行时禁止换 IP 约束必须在管理 API 层强制执行。
- **产品范围:** v1 不做计费、多宿主机调度。

## Sources

### Primary (HIGH confidence)
- npm registry — react 19.2.4, vite 8.0.3, tailwindcss 4.2.2, @tanstack/react-query 5.95.2, @tanstack/react-router 1.168.7, @tanstack/react-table 8.21.3（npm view 命令确认 2026-03-27）
- pkg.go.dev golang-jwt/jwt/v5 v5.3.1 — JWT 库版本和 API 文档
- tailwindcss.com/docs/installation/using-vite — Tailwind CSS v4 Vite 安装文档
- ui.shadcn.com/docs/installation/vite — shadcn/ui Vite 安装指南
- 项目现有代码 — router.go, hosts.go, queries.go, models.go, migrations

### Secondary (MEDIUM confidence)
- WebSearch: TanStack Router vs React Router v7 比较（多源交叉验证）
- WebSearch: golang-jwt HMAC 使用示例（与官方 GitHub examples 交叉验证）

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 所有版本通过 npm registry 和 pkg.go.dev 实际确认
- Architecture: HIGH — 基于项目现有后端模式的自然延伸，前端方案为 shadcn/ui 官方推荐模式
- Pitfalls: HIGH — 基于 Tailwind v4 迁移文档、JWT 安全最佳实践和项目 Phase 2 约束推导

**Research date:** 2026-03-27
**Valid until:** 2026-04-27（前端生态变化快，但核心栈稳定）
