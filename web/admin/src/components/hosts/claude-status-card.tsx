import { RefreshCw, Download } from "lucide-react";
import { toast } from "sonner";
import { useClaudeStatus, useUpdateClaude } from "@/hooks/use-hosts";
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
              <dt className="text-muted-foreground">当前版本</dt>
              <dd className="font-mono text-xs">{data.version}</dd>
            </div>
          </dl>
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
