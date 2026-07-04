import { useRouterState } from "@tanstack/react-router";
import { ChevronDown, CircleHelp, LogOut, User } from "lucide-react";
import {
  clearAllSessions,
  logout,
  redirectToSessionHome,
} from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const pageTitles: Record<string, string> = {
  "/": "仪表板",
  "/users": "用户管理",
  "/egress-ips": "出口 IP",
  "/hosts": "主机管理",
  "/tasks": "任务列表",
  "/portal": "我的面板",
};

interface TopbarProps {
  onHelpClick?: () => void;
}

export function Topbar({ onHelpClick }: TopbarProps = {}) {
  const routerState = useRouterState();
  const pathname = routerState.location.pathname;
  const title =
    pageTitles[pathname] ??
    (pathname.startsWith("/portal/hosts/") ? "主机详情" : "管理后台");

  const { currentSession, sessions } = useAuthSessions();
  const roleLabel = currentSession?.role === "admin" ? "管理员" : "用户";

  return (
    <header className="topbar-glass flex h-14 items-center justify-between px-6">
      <h2 className="text-sm font-semibold">{title}</h2>
      <div className="flex items-center gap-2">
        {onHelpClick && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={onHelpClick}
            title="使用引导"
          >
            <CircleHelp className="h-4 w-4 text-muted-foreground" />
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              className="flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors hover:bg-accent"
            >
              <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10">
                <User className="h-3.5 w-3.5 text-primary" />
              </div>
              <div className="text-left hidden sm:block">
                <p className="text-xs font-medium leading-none">
                  {currentSession?.username ?? currentSession?.shortId ?? "未登录"}
                </p>
                <p className="text-[10px] text-muted-foreground mt-0.5">
                  {roleLabel}
                </p>
              </div>
              <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-64">
            <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
              已保存会话
            </DropdownMenuLabel>
            {sessions.map((session) => (
              <DropdownMenuItem
                key={session.id}
                onClick={() => redirectToSessionHome(session.id)}
                className="flex items-center justify-between"
              >
                <div className="min-w-0">
                  <div className="truncate text-sm">
                    {session.username ?? session.shortId}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {session.role === "admin" ? "管理员" : "用户"}
                  </div>
                </div>
                {session.id === currentSession?.id ? (
                  <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
                    当前
                  </span>
                ) : null}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={logout}>
              <LogOut className="h-4 w-4" />
              退出当前账号
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onClick={() => {
                clearAllSessions();
                window.location.href = "/login";
              }}
            >
              <LogOut className="h-4 w-4" />
              清空全部会话
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
