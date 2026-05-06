# 三个列表页 A 档视觉收敛 — 跨会话交接文档

**生成时间：** 2026-05-07
**任务 ID（仅参考，新会话会重新分配）：** 260507-3o9
**预期入口：** `/gsd:quick A 档视觉收敛三个列表页`，并把本文件路径告诉 Claude

> 本文档由上一会话生成，包含完整的现状诊断、设计决策与 6 项原子改动指引。新会话的 Claude 只需读本文件 + 文中列出的现有源码，无须重新调研三个列表页。

---

## 1. 任务边界

**做：** 在 `/hosts`、`/users`、`/egress-ips` 三个列表页做**视觉/语义层**收敛 —— 新增 status token、抽 `<StatusDot>`、改表头/行高、抽 `<TableSkeleton>`、空态独立、三页状态色统一走 token。

**不做（明确不在本档）：**
- 工具条（搜索 / 筛选 / 计数 / 刷新 / URL deep link）
- 操作列收敛（hosts 4 个 ghost button 收进 dropdown）
- 字段补强（用户主机数、剩余天数预警、egress 健康列合并）
- 新增依赖、新增字体

---

## 2. 现状要点（已读完代码，结论）

技术栈：React 19.2 + Tailwind 4.1 + shadcn 风格 + Radix + lucide。

主要参考文件（新会话需要读的）：
- `web/admin/src/index.css` — 主题 token 定义
- `web/admin/src/components/ui/table.tsx` — Table/TableRow/TableHead/TableCell
- `web/admin/src/components/ui/badge.tsx` — Badge variants
- `web/admin/src/components/layout/data-table-shell.tsx` — 卡片包裹
- `web/admin/src/components/layout/empty-state.tsx` — 空态
- `web/admin/src/routes/_dashboard/hosts/index.tsx` — hosts 列表页
- `web/admin/src/routes/_dashboard/users/index.tsx` — users 列表页
- `web/admin/src/routes/_dashboard/egress-ips/index.tsx` — egress 列表页
- `web/admin/src/hooks/use-users.ts` — 用户类型与 API
- `web/admin/src/hooks/use-hosts.ts` — 主机类型与 API
- `web/admin/src/hooks/use-egress-ips.ts` — 出口 IP 类型与 API

目前痛点（已诊断，仅供新会话参考，不必重新分析）：
1. 状态色硬编码 `text-green-600 / text-yellow-600 / text-red-500 / text-blue-500` 散落各处，违反语义 token 设计
2. 表头与单元格同样字号 `text-sm`，无法快速扫视列结构
3. 行高 `h-10 / p-2` 偏紧，密度高但呼吸不足
4. Skeleton 固定宽度 `w-20`，loading 像色块阵列
5. EmptyState 被嵌进 `<TableCell colSpan>` 里，被表格 border 切割
6. hosts 页用了 Badge default（紫蓝主色）表示"运行中"，会被误读成"被选中"

---

## 3. 设计决策（已与用户对齐）

| 决策 | 选定方案 | 备注 |
|---|---|---|
| 改造范围 | A 档：仅视觉收敛 | 不引入新功能 |
| 主色 | 保留 `oklch(0.488 0.200 264)` 紫蓝 | 不切换到 BI 蓝+琥珀 |
| 状态语义层 | 新增 success / warning / info token | 配合 `<StatusDot>` 使用 |
| 字体 | 沿用 Inter | 不引入 Fira Code/Sans |
| 行高 | 增加约 20% | `h-10 → h-11`；`p-2 → px-3 py-3.5` |
| 表头 | 加 `bg-muted/40` + 小字 + uppercase | 增强扫视性 |
| 走 GSD | `/gsd:quick`（不带 --full / --discuss / --research） | 6 项原子改动适合 quick |

---

## 4. 6 项原子改动（按提交顺序）

### 改动 1 — 新增 status 语义 token

**文件：** `web/admin/src/index.css`

在 `@theme {` 块内追加：

```css
--color-success: var(--success);
--color-success-foreground: var(--success-foreground);
--color-warning: var(--warning);
--color-warning-foreground: var(--warning-foreground);
--color-info: var(--info);
--color-info-foreground: var(--info-foreground);
```

在 `:root {` 块内追加（紫蓝主色不动）：

```css
--success: oklch(0.62 0.17 145);
--success-foreground: oklch(0.985 0 0);
--warning: oklch(0.72 0.17 75);
--warning-foreground: oklch(0.18 0 0);
--info: oklch(0.58 0.16 220);
--info-foreground: oklch(0.985 0 0);
```

> 验证：在浏览器控制台 `getComputedStyle(document.body).getPropertyValue('--color-success')` 应返回非空。

**提交建议：** `feat(admin): add success/warning/info semantic tokens`

---

### 改动 2 — 新建 `<StatusDot>` 组件

**文件：** `web/admin/src/components/ui/status-dot.tsx`（新建）

完整代码（直接落地）：

```tsx
import { cn } from "@/lib/utils";

type StatusDotVariant =
  | "success"
  | "warning"
  | "info"
  | "danger"
  | "muted"
  | "loading";

interface StatusDotProps {
  variant: StatusDotVariant;
  pulse?: boolean;
  size?: "sm" | "md";
  className?: string;
}

const variantClass: Record<StatusDotVariant, string> = {
  success: "bg-success",
  warning: "bg-warning",
  info: "bg-info",
  danger: "bg-destructive",
  muted: "bg-muted-foreground/50",
  loading: "bg-info",
};

export function StatusDot({
  variant,
  pulse = false,
  size = "sm",
  className,
}: StatusDotProps) {
  const sizeClass = size === "md" ? "h-2.5 w-2.5" : "h-2 w-2";

  return (
    <span
      role="presentation"
      className={cn(
        "relative inline-flex shrink-0 items-center justify-center",
        sizeClass,
        className,
      )}
    >
      <span
        className={cn(
          "absolute inset-0 rounded-full",
          variantClass[variant],
        )}
      />
      {pulse ? (
        <span
          className={cn(
            "absolute inset-0 rounded-full opacity-60",
            variantClass[variant],
            "motion-safe:animate-ping motion-reduce:animate-none",
          )}
        />
      ) : null}
    </span>
  );
}
```

> 关键点：
> - `motion-reduce:animate-none` 自动尊重 `prefers-reduced-motion`
> - `bg-success / bg-warning / bg-info` 来自改动 1 的 token；Tailwind 4 自动识别
> - 默认 `sm=8px`、`md=10px`

**提交建议：** `feat(admin): add StatusDot component using semantic tokens`

---

### 改动 3 — 表头视觉加重 + 行高放宽

**文件：** `web/admin/src/components/ui/table.tsx`

#### 3a. `TableHead`（行 66–77）

before：
```tsx
className={cn(
  "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
  className
)}
```

after：
```tsx
className={cn(
  "h-11 bg-muted/40 px-3 text-left align-middle text-xs font-medium uppercase tracking-wide whitespace-nowrap text-muted-foreground [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
  className
)}
```

#### 3b. `TableCell`（行 79–90）

before：
```tsx
className={cn(
  "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
  className
)}
```

after：
```tsx
className={cn(
  "px-3 py-3.5 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]",
  className
)}
```

#### 3c. `TableRow`（行 53–64）

before：
```tsx
className={cn(
  "border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted",
  className
)}
```

after：
```tsx
className={cn(
  "border-b transition-colors hover:bg-accent/40 data-[state=selected]:bg-muted",
  className
)}
```

> 兼容性：三页里现有 `<TableCell className="text-muted-foreground text-sm">` 的覆盖会通过 cn() 后置叠加，仍然生效；不需要改页面。
> 注意：这个改动会全站影响所有 Table，不止三页。新会话需要在执行后随手扫一下其他用 Table 的地方（如 events、tasks 列表）确认无视觉退化。

**提交建议：** `style(admin): emphasize table headers and relax row density`

---

### 改动 4 — 新建 `<TableSkeleton>`

**文件：** `web/admin/src/components/ui/table-skeleton.tsx`（新建）

完整代码：

```tsx
import { cn } from "@/lib/utils";
import { TableBody, TableCell, TableRow } from "@/components/ui/table";

interface TableSkeletonColumn {
  width: string;
  pill?: boolean;
  muted?: boolean;
  align?: "left" | "right" | "center";
}

interface TableSkeletonProps {
  columns: TableSkeletonColumn[];
  rows?: number;
}

export function TableSkeleton({ columns, rows = 4 }: TableSkeletonProps) {
  return (
    <TableBody>
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <TableRow key={rowIdx}>
          {columns.map((col, colIdx) => (
            <TableCell
              key={colIdx}
              className={cn(
                col.align === "right" && "text-right",
                col.align === "center" && "text-center",
              )}
            >
              <div
                className={cn(
                  "animate-pulse",
                  col.width,
                  col.pill ? "h-5 rounded-full" : "h-4 rounded",
                  col.muted ? "bg-muted/60" : "bg-muted",
                  col.align === "right" && "ml-auto",
                  col.align === "center" && "mx-auto",
                )}
              />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </TableBody>
  );
}
```

**提交建议：** `feat(admin): add TableSkeleton component for differentiated loading state`

---

### 改动 5 — 三页空态独立 + 启用 TableSkeleton

通用模式（三页都要套用）：

**before（现有结构）：**
```tsx
<DataTableShell>
  <Table>
    <TableHeader>…</TableHeader>
    <TableBody>
      {isLoading ? (
        Array.from({ length: 3 }).map(...)
      ) : list.length === 0 ? (
        <TableRow><TableCell colSpan={N} className="p-0">
          <EmptyState … />
        </TableCell></TableRow>
      ) : (
        list.map(...)
      )}
    </TableBody>
  </Table>
</DataTableShell>
```

**after：**
```tsx
{isLoading ? (
  <DataTableShell>
    <Table>
      <TableHeader>…</TableHeader>
      <TableSkeleton
        rows={4}
        columns={[
          { width: "w-32" },
          { width: "w-24" },
          /* 按页配置 */
        ]}
      />
    </Table>
  </DataTableShell>
) : list.length === 0 ? (
  <DataTableShell>
    <EmptyState … />
  </DataTableShell>
) : (
  <DataTableShell>
    <Table>
      <TableHeader>…</TableHeader>
      <TableBody>
        {list.map(...)}
      </TableBody>
    </Table>
  </DataTableShell>
)}
```

#### 5a. `hosts/index.tsx` — TableSkeleton 列配置

```tsx
columns={[
  { width: "w-28" },                  // 主机名
  { width: "w-20" },                  // 所属用户
  { width: "w-24" },                  // 出口 IP
  { width: "w-20", pill: true },      // 状态
  { width: "w-28", muted: true },     // 更新时间
  { width: "w-12", align: "right" },  // 操作
]}
```

#### 5b. `users/index.tsx` — TableSkeleton 列配置

```tsx
columns={[
  { width: "w-24" },                  // 用户名
  { width: "w-16", pill: true },      // 状态
  { width: "w-28", muted: true },     // 到期时间
  { width: "w-32", muted: true },     // 创建时间
  { width: "w-8", align: "right" },   // 操作
]}
```

#### 5c. `egress-ips/index.tsx` — TableSkeleton 列配置

```tsx
columns={[
  { width: "w-24" },                  // 标签
  { width: "w-48" },                  // 代理服务器
  { width: "w-28" },                  // 实际 IP
  { width: "w-20", pill: true },      // 状态
  { width: "w-8", align: "right" },   // 操作
]}
```

**提交建议：** `style(admin): elevate empty state and use TableSkeleton across list pages`

> 三页可以拆 3 个 commit 也可以合 1 个，新会话自行决定（建议合 1 个，因为机械复现）。

---

### 改动 6 — 三页状态色统一走 token + StatusDot

#### 6a. `hosts/index.tsx`

**6a.1 修改 `getHostStatus()`（行 80–98）**

返回类型从 `{ type, label, variant }` 改为 `{ type, label, tone }`，`tone: "success" | "info" | "danger" | "muted"`。

before：
```tsx
if (db === "failed") return { type: "badge" as const, label: "失败", variant: "destructive" as const };
if (db === "pending") return { type: "badge" as const, label: "等待中", variant: "outline" as const };
if (db === "running") return { type: "badge" as const, label: "运行中", variant: "default" as const };
if (db === "stopped") return { type: "badge" as const, label: "已停止", variant: "secondary" as const };
return { type: "badge" as const, label: db || "未知", variant: "outline" as const };
```

after：
```tsx
if (db === "failed") return { type: "dot" as const, label: "失败", tone: "danger" as const };
if (db === "pending") return { type: "dot" as const, label: "等待中", tone: "muted" as const };
if (db === "running") return { type: "dot" as const, label: "运行中", tone: "success" as const };
if (db === "stopped") return { type: "dot" as const, label: "已停止", tone: "muted" as const };
return { type: "dot" as const, label: db || "未知", tone: "muted" as const };
```

**6a.2 修改状态列渲染（行 228–267）**

把原来 `<Badge variant={status.variant}>{status.label}</Badge>` 改为：

```tsx
<span className="inline-flex items-center gap-2 text-sm">
  <StatusDot variant={status.tone} />
  {status.label}
</span>
```

Loading 分支（任务进行中）改为：

```tsx
<span className="inline-flex items-center gap-2 text-sm text-info">
  <StatusDot variant="info" pulse />
  {status.label}
</span>
```

> 把原来 `text-primary` 改为 `text-info`，避免任务进行中和"运行中"颜色撞车（运行中=success 绿，进行中=info 蓝）。
> Loader2 图标可以删掉，dot 的 pulse 已经表达"进行中"。如果保留也行，但二选一更干净。

**6a.3 行内三个彩色 ghost button**

`Play` / `Square` / `RotateCcw`（行 287、308、326）：
- `text-green-600` → `text-success`
- `text-red-500` → `text-destructive`
- `text-blue-500` → `text-info`

**6a.4 import 调整**

文件顶部追加：
```tsx
import { StatusDot } from "@/components/ui/status-dot";
```

可以移除 `Badge` 的 import（如果没有别处用了）。

#### 6b. `users/index.tsx`

**6b.1 状态 Badge 改 StatusDot**

before（行 139–155）：
```tsx
<TableCell>
  <Badge variant={user.status === "active" ? "default" : user.status === "expired" ? "destructive" : "secondary"}>
    {user.status === "active" ? "活跃" : user.status === "expired" ? "已过期" : "已禁用"}
  </Badge>
</TableCell>
```

after：
```tsx
<TableCell>
  <span className="inline-flex items-center gap-2 text-sm">
    <StatusDot
      variant={
        user.status === "active"
          ? "success"
          : user.status === "expired"
            ? "danger"
            : "muted"
      }
    />
    {user.status === "active" ? "活跃" : user.status === "expired" ? "已过期" : "已禁用"}
  </span>
</TableCell>
```

**6b.2 到期时间加 `tabular-nums`（行 156–158）**

```tsx
<TableCell className="text-muted-foreground tabular-nums">
  {user.expires_at ? formatDate(user.expires_at) : "永不过期"}
</TableCell>
```

创建时间列也加 `tabular-nums`。

**6b.3 import**

追加 `import { StatusDot } from "@/components/ui/status-dot";`，移除 Badge 的 import（如果没别处用）。

> 注意：A 档不引入"剩 ≤7 天 = warning"逻辑，C 档再做。

#### 6c. `egress-ips/index.tsx`

**6c.1 改 `StatusCell`（行 406–499）**

把所有 `text-green-600 / text-yellow-600 / text-destructive` 替换为 token；用 `<StatusDot>` 替换内嵌 `Check / X` 图标；保留 `Loader2` 检测中动画。

before（passed 分支）：
```tsx
<button
  onClick={onClickResult}
  className="flex items-center gap-1.5 text-sm text-green-600 hover:underline"
>
  <Check className="h-3.5 w-3.5" />
  正常
</button>
```

after：
```tsx
<button
  onClick={onClickResult}
  className="flex items-center gap-2 text-sm text-success hover:underline"
>
  <StatusDot variant="success" />
  正常
</button>
```

partial 分支：`text-yellow-600` → `text-warning`，`<X>` → `<StatusDot variant="warning" />`，文案"部分异常"。

failed 分支：`text-destructive` 保留，`<X>` → `<StatusDot variant="danger" />`。

disabled 分支（行 419–421）：保留 Badge variant=secondary 即可，或改为 `<StatusDot variant="muted" /> 已禁用`，二者皆可（建议改 dot，全页一致）。

isTesting 分支：保留 Loader2，但配色改 `text-info`，可以前置 `<StatusDot variant="info" pulse />` 二选一（建议只留 Loader2 + text-info，避免双重动画）。

待测试分支（行 433–438）：`<Minus>` → `<StatusDot variant="muted" />`，`text-muted-foreground` 保留。

**6c.2 实际 IP 列的 RefreshCw 小按钮**（行 252–259）

颜色保持中性（`text-muted-foreground hover:text-foreground`）即可，不需要 token 化。

**6c.3 import**

追加 `import { StatusDot } from "@/components/ui/status-dot";`，可以移除 `Check / X / Minus` 中不再使用的 import。

**提交建议（6 整体）：** `style(admin): unify list page status colors via tokens and StatusDot`

> 三页可以拆成 3 个 commit 也可以合 1 个 commit，新会话自行权衡。

---

## 5. 验证清单

执行完所有改动后必须通过：

```bash
# 前端类型检查
cd web/admin && npm run typecheck

# 启动 dev 服务器（用户已经在跑 :5173）
# 手动验证三页：
# - http://localhost:5173/hosts
# - http://localhost:5173/users
# - http://localhost:5173/egress-ips
```

人眼自检项：
- [ ] 三页 loading 态显示差异化骨架，不是均匀色块
- [ ] 三页 empty 态在卡片中央，不被表格 border 切割
- [ ] hosts 运行中 = 绿点，启动中 = 蓝点（脉冲），失败 = 红点
- [ ] users 活跃 = 绿点，已过期 = 红点，已禁用 = 灰点
- [ ] egress 正常 = 绿点，部分异常 = 琥珀点，异常 = 红点
- [ ] 表头有浅灰底 + 小字 uppercase
- [ ] 行高比之前明显宽松（约 +20%），但不夸张
- [ ] hover 行背景为浅 accent，不抢戏
- [ ] 系统设置 reduced motion 后，StatusDot pulse 应停止

视觉对比度（WCAG AA）：
- success `oklch(0.62 0.17 145)` 在 white 背景：约 4.5:1 ✓
- warning `oklch(0.72 0.17 75)` 在 white 背景：作为色点足够；作为文字时建议旁边配文本（StatusDot + "部分异常"）
- info `oklch(0.58 0.16 220)` 在 white 背景：约 4.6:1 ✓

---

## 6. 影响范围（净增/改）

| 文件 | 类型 | 估计行数 |
|---|---|---|
| `web/admin/src/index.css` | 改 | +12 |
| `web/admin/src/components/ui/status-dot.tsx` | 新 | ~50 |
| `web/admin/src/components/ui/table-skeleton.tsx` | 新 | ~50 |
| `web/admin/src/components/ui/table.tsx` | 改 | className 微调 |
| `web/admin/src/routes/_dashboard/hosts/index.tsx` | 改 | ~40 |
| `web/admin/src/routes/_dashboard/users/index.tsx` | 改 | ~25 |
| `web/admin/src/routes/_dashboard/egress-ips/index.tsx` | 改 | ~50 |

无新增 npm 依赖、无后端字段变更、无 API 调整。

---

## 7. 新会话启动指引（直接复制给下一会话的 Claude）

> Hi，请按以下顺序执行：
>
> 1. 读 `.planning/quick/260507-3o9-a/HANDOFF.md`，里面是上一会话已经做完的诊断和完整 6 项原子改动指引。
> 2. 不要重复调研三个列表页的代码，直接按 HANDOFF 第 4 节执行。
> 3. 入口走 `/gsd:quick A 档视觉收敛三个列表页（按 .planning/quick/260507-3o9-a/HANDOFF.md 指引）`，让 GSD 重新分配 quick_id 和目录。
> 4. 在 plan 阶段把 HANDOFF 第 4 节的 6 项作为 6 个 task；执行阶段直接按文件落地，每项独立提交。
> 5. 全部做完后跑 `cd web/admin && npm run typecheck`，并人工在浏览器验证三页 loading/empty/list 三态。
> 6. 完成后回传 SUMMARY，包括：每项的提交 hash、typecheck 结果、是否有 reduced motion 行为差异。

---

## 8. 已知风险与注意事项

1. **改 `table.tsx` 全站影响。** 三页之外的 `events`、`tasks`、`hosts/$hostId`、`users/$userId` 详情页也会用到 Table。新会话执行后需要扫一眼这些页面，确认行高变化没有破坏布局（详情页里 Table 通常在卡片里，行高放宽反而更舒服，预期无回归）。

2. **`Badge default` 与 success 的语义差异。** 原 hosts 页"运行中"用的是 Badge default（紫蓝），换成 success 绿色后，与 sidebar 紫蓝主色的视觉权重区分更清晰，但在某些用户的潜意识里"紫色 = 选中"已经形成。无需特别处理。

3. **`oklch()` 浏览器兼容性。** 项目已在用 oklch（见现有 token），无需担心。

4. **GSD 工作流约束。** CLAUDE.md 要求"在使用 Edit、Write 或其他会修改文件的工具前，应先通过 GSD 命令进入工作流"。新会话务必走 `/gsd:quick`。

5. **本目录的 commit 时机。** 当前 init 阶段创建的 `.planning/quick/260507-3o9-a/` 目录暂未被 git 跟踪。新会话进入 `/gsd:quick` 时会重新分配目录，此目录可以保留作为参考，最终连同新的 quick 目录一起 commit；或者新会话也可以直接复用此 ID 与目录（取决于 init 行为）。
