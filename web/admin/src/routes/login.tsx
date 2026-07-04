import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { removeSession, saveSession, switchSession } from "@/lib/auth";
import { useAuthSessions } from "@/hooks/use-auth-sessions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Cloud, ArrowRight, X, Eye, EyeOff } from "lucide-react";

const loginSchema = z.object({
  username: z.string().min(1, "用户名不能为空"),
  password: z.string().min(1, "密码不能为空"),
});

type LoginForm = z.infer<typeof loginSchema>;

export const Route = createFileRoute("/login")({
  component: LoginPage,
});

function LoginPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);
  const [showPassword, setShowPassword] = useState(false);
  const { sessions } = useAuthSessions();

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<LoginForm>({
    resolver: zodResolver(loginSchema),
  });

  const loginMutation = useMutation({
    mutationFn: async (data: LoginForm) => {
      const res = await fetch("/v1/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({ error: "登录失败" }));
        throw new Error(body.error || "登录失败");
      }

      return res.json() as Promise<{
        username: string;
        short_id: string;
        token: string;
        role: string;
        expires_in: number;
      }>;
    },
    onSuccess: (data) => {
      saveSession(data.short_id, data.token, data.username);
      if (data.role === "admin") {
        navigate({ to: "/" });
      } else {
        navigate({ to: "/portal" });
      }
    },
    onError: (err: Error) => {
      setError(err.message);
    },
  });

  const onSubmit = (data: LoginForm) => {
    setError(null);
    loginMutation.mutate(data);
  };

  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden px-4 py-10">
      {/* 四周极光浮球 */}
      <div className="brand-orb h-96 w-96" style={{ background: "#f0abfc", top: "-6rem", left: "-4rem" }} />
      <div className="brand-orb h-[26rem] w-[26rem]" style={{ background: "#67e8f9", bottom: "-8rem", right: "-6rem", animationDelay: "-4s" }} />
      <div className="brand-orb h-80 w-80" style={{ background: "#a78bfa", top: "8%", right: "12%", animationDelay: "-8s" }} />
      <div className="brand-orb h-72 w-72" style={{ background: "#93c5fd", bottom: "6%", left: "10%", animationDelay: "-6s" }} />

      {/* 居中悬浮单卡 */}
      <div className="glass relative z-10 w-full max-w-lg space-y-6 rounded-[1.75rem] border border-white/70 p-8 sm:p-10 shadow-[0_40px_100px_-30px_rgba(124,92,246,0.55)]">
        {/* 项目信息 */}
        <div className="flex flex-col items-center gap-3.5 text-center">
          <div className="grad-brand-bg glow-brand flex h-14 w-14 items-center justify-center rounded-2xl">
            <Cloud className="h-7 w-7 text-white" />
          </div>
          <span className="grad-text text-2xl font-extrabold tracking-tight">
            ClaudeDock
          </span>
          <h1 className="text-lg font-bold leading-snug text-foreground">
            一台会伪装的云端开发机
          </h1>
          <p className="max-w-md text-sm leading-relaxed text-muted-foreground">
            隔离容器预装 Claude Code，绑定独立住宅出口 IP，本地项目目录同名映射——
            像在自己电脑上写代码，却拥有一个干净、真实的美国开发者身份。
          </p>
          <div className="flex flex-wrap justify-center gap-2 pt-1">
            {["身份伪装", "独立出口 IP", "代码同名映射", "全隧道零泄漏"].map((t) => (
              <span
                key={t}
                className="rounded-full border border-white/70 bg-white/55 px-3 py-1 text-xs font-medium text-secondary-foreground"
              >
                {t}
              </span>
            ))}
          </div>
        </div>

        {/* 分隔 */}
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          <span className="h-px flex-1 bg-border" />
          登录进入控制台
          <span className="h-px flex-1 bg-border" />
        </div>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                placeholder="输入用户名"
                autoComplete="username"
                className="h-11"
                {...register("username")}
              />
              {errors.username && (
                <p className="text-xs text-destructive">
                  {errors.username.message}
                </p>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? "text" : "password"}
                  placeholder="输入密码"
                  autoComplete="current-password"
                  className="h-11 pr-10"
                  {...register("password")}
                />
                <button
                  type="button"
                  aria-label={showPassword ? "隐藏密码" : "显示密码"}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowPassword(!showPassword)}
                >
                  {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {errors.password && (
                <p className="text-xs text-destructive">
                  {errors.password.message}
                </p>
              )}
            </div>

            {error && (
              <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
                {error}
              </div>
            )}

            <Button
              type="submit"
              className="w-full h-11 gap-2"
              disabled={loginMutation.isPending}
            >
              {loginMutation.isPending ? "登录中…" : "登录"}
              {!loginMutation.isPending && <ArrowRight className="h-4 w-4" />}
            </Button>
          </form>

          {sessions.length > 0 && (
            <div className="space-y-3 border-t pt-6">
              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
                已保存会话
              </p>
              <div className="space-y-2">
                {sessions.map((session) => (
                  <div
                    key={session.id}
                    className="flex items-center justify-between rounded-xl border p-3 transition-colors hover:bg-accent/50"
                  >
                    <button
                      type="button"
                      className="min-w-0 text-left"
                      onClick={() => {
                        const next = switchSession(session.id);
                        if (next?.role === "admin") {
                          navigate({ to: "/" });
                        } else {
                          navigate({ to: "/portal" });
                        }
                      }}
                    >
                      <p className="text-sm font-medium">
                        {session.username ?? session.shortId}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {session.role === "admin" ? "管理员" : "用户"}
                      </p>
                    </button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-muted-foreground hover:text-destructive"
                      onClick={() => removeSession(session.id)}
                    >
                      <X className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
  );
}
