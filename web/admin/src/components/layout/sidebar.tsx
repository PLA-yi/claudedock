import { useState } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import {
  LayoutDashboard,
  Users,
  Globe,
  Server,
  ListTodo,
  ScrollText,
  LogOut,
  Cloud,
  Monitor,
  Menu,
  X,
} from "lucide-react";
import { logout } from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { cn } from "@/lib/utils";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

const navItems = [
  { label: "仪表板", to: "/", icon: LayoutDashboard },
  { label: "用户管理", to: "/users", icon: Users },
  { label: "出口 IP", to: "/egress-ips", icon: Globe },
  { label: "主机管理", to: "/hosts", icon: Server },
  { label: "任务列表", to: "/tasks", icon: ListTodo },
  { label: "事件日志", to: "/events", icon: ScrollText },
] as const;

export function Sidebar() {
  const routerState = useRouterState();
  const currentPath = routerState.location.pathname;
  const { currentSession } = useAuthSessions();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false);

  return (
    <>
      {/* 移动端汉堡按钮 */}
      <button
        type="button"
        aria-label="打开菜单"
        className="fixed left-3 top-3 z-50 rounded-lg border bg-background p-2 lg:hidden"
        onClick={() => setMobileOpen(true)}
      >
        <Menu className="h-5 w-5" />
      </button>

      {/* 移动端遮罩 */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={() => setMobileOpen(false)}
          aria-hidden
        />
      )}

      <aside
        className={cn(
          "sidebar-glass fixed inset-y-0 left-0 z-40 flex w-60 flex-col text-sidebar-foreground transition-transform duration-200 lg:relative lg:translate-x-0",
          mobileOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        {/* Logo */}
        <div className="flex h-16 items-center justify-between gap-2.5 px-5">
          <Link to="/" className="flex items-center gap-2.5" onClick={() => setMobileOpen(false)}>
            <div className="grad-brand-bg glow-brand flex h-8 w-8 items-center justify-center rounded-lg">
              <Cloud className="h-4.5 w-4.5 text-white" />
            </div>
            <div className="flex flex-col">
              <span className="grad-text text-sm font-bold leading-tight">ClaudeDock</span>
              <span className="text-[10px] font-medium text-sidebar-muted tracking-wider uppercase">Proxy</span>
            </div>
          </Link>
          <button
            type="button"
            aria-label="关闭菜单"
            className="rounded p-1 text-sidebar-foreground/50 hover:text-white lg:hidden"
            onClick={() => setMobileOpen(false)}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 导航 */}
        <nav className="flex-1 px-3 pt-4 space-y-0.5 overflow-y-auto">
          <p className="px-3 pb-2 text-[10px] font-semibold uppercase tracking-wider text-sidebar-muted">
            管理
          </p>
          {navItems.map((item) => {
            const isActive =
              item.to === "/"
                ? currentPath === "/"
                : currentPath.startsWith(item.to);

            return (
              <Link
                key={item.to}
                to={item.to}
                onClick={() => setMobileOpen(false)}
                className={cn(
                  "flex items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium transition-all duration-150",
                  isActive
                    ? "nav-active"
                    : "text-sidebar-foreground/75 hover:bg-white/50 hover:text-sidebar-foreground",
                )}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </Link>
            );
          })}
        </nav>

        {/* 底部操作区 */}
        <div className="px-3 pb-2 space-y-1">
          {/* Portal 预览入口（仅 admin 可见） */}
          <Link
            to="/portal"
            onClick={() => setMobileOpen(false)}
            className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium text-sidebar-foreground/60 transition-colors hover:bg-white/50 hover:text-sidebar-foreground"
          >
            <Monitor className="h-4 w-4" />
            用户门户预览
          </Link>
          <button
            onClick={() => setLogoutConfirmOpen(true)}
            className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-[13px] font-medium text-sidebar-foreground/60 transition-colors hover:bg-white/50 hover:text-sidebar-foreground"
          >
            <LogOut className="h-4 w-4" />
            退出登录
          </button>
          {currentSession && (
            <div className="px-3 pt-2 text-[10px] text-sidebar-muted">
              {currentSession.username ?? currentSession.shortId}
              {" · 管理员"}
            </div>
          )}
        </div>

        <AlertDialog open={logoutConfirmOpen} onOpenChange={setLogoutConfirmOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认退出登录？</AlertDialogTitle>
              <AlertDialogDescription>
                退出后将返回登录页面。
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>取消</AlertDialogCancel>
              <AlertDialogAction onClick={logout}>确认退出</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </aside>
    </>
  );
}
