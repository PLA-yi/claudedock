import { RefreshCw, Download } from "lucide-react";
import { toast } from "sonner";
import { useClaudeStatus, useUpdateClaude } from "@/hooks/use-hosts";
import type { ClaudeProcess } from "@/hooks/use-hosts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

interface ClaudeStatusCardProps {
  hostId: string;
  hostStatus: string;
}

function formatElapsed(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

function shortenDir(dir: string): string {
  if (dir === "/workspace") return "~";
  if (dir.startsWith("/workspace/")) return "~/" + dir.slice("/workspace/".length);
  return dir;
}

function ProcessTable({ processes }: { processes: ClaudeProcess[] }) {
  if (processes.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        当前没有运行中的 Claude 进程。
      </p>
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border border-border/60">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-muted/40 text-xs text-muted-foreground">
            <th className="px-3 py-2 text-left font-medium">PID</th>
            <th className="px-3 py-2 text-left font-medium">工作目录</th>
            <th className="px-3 py-2 text-right font-medium">运行时间</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border/40">
          {processes.map((p) => (
            <tr key={p.pid} className="hover:bg-muted/20">
              <td className="px-3 py-2 font-mono text-xs">{p.pid}</td>
              <td className="max-w-[280px] truncate px-3 py-2 font-mono text-xs" title={p.work_dir}>
                {shortenDir(p.work_dir)}
              </td>
              <td className="px-3 py-2 text-right text-xs tabular-nums">
                {formatElapsed(p.elapsed_seconds)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function ClaudeStatusCard({
  hostId,
  hostStatus,
}: ClaudeStatusCardProps) {
  const { data, isLoading, refetch } = useClaudeStatus(
    hostId,
    hostStatus === "running",
  );
  const updateMutation = useUpdateClaude();

  if (hostStatus !== "running") return null;

  function handleUpdate() {
    updateMutation.mutate(hostId, {
      onSuccess: (res) => {
        toast.success(`Claude Code 已更新到 ${res.version}`);
        refetch();
      },
      onError: () => toast.error("更新 Claude Code 失败"),
    });
  }

  return (
    <Card className="rounded-xl border-border/80 shadow-sm">
      <CardHeader className="border-b bg-muted/30 pb-4">
        <CardTitle className="text-base">Claude Code 状态</CardTitle>
        <CardDescription className="text-xs leading-relaxed">
          容器内 Claude 进程信息与版本管理。
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 p-6 pt-5">
        {isLoading ? (
          <div className="space-y-3">
            <div className="h-5 w-40 animate-pulse rounded bg-muted" />
            <div className="h-5 w-32 animate-pulse rounded bg-muted" />
          </div>
        ) : data ? (
          <div className="space-y-4">
            <dl className="grid gap-3 text-sm">
              <div className="flex items-center justify-between">
                <dt className="text-muted-foreground">运行实例数</dt>
                <dd>
                  <Badge
                    variant={
                      data.running_instances > 0 ? "default" : "secondary"
                    }
                  >
                    {data.running_instances}
                  </Badge>
                </dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="text-muted-foreground">安装版本</dt>
                <dd className="font-mono text-xs">{data.version}</dd>
              </div>
            </dl>

            <ProcessTable processes={data.processes ?? []} />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">无法获取状态</p>
        )}

        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => refetch()}
          >
            <RefreshCw className="h-3.5 w-3.5" />
            刷新
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            disabled={updateMutation.isPending}
            onClick={handleUpdate}
          >
            <Download className="h-3.5 w-3.5" />
            {updateMutation.isPending ? "更新中…" : "更新 Claude Code"}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
