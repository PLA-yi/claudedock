import { useState } from "react";
import { FileText, X, RefreshCw } from "lucide-react";
import { useHostLogs } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface HostLogsDialogProps {
  hostId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function HostLogsDialog({ hostId, open, onOpenChange }: HostLogsDialogProps) {
  const { data, isLoading, refetch, isRefetching } = useHostLogs(hostId);
  const [autoRefresh, setAutoRefresh] = useState(true);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[80vh] flex flex-col p-0">
        <DialogHeader className="px-6 pt-6 pb-2">
          <DialogTitle className="flex items-center gap-2 text-base">
            <FileText className="h-5 w-5" />
            容器日志
            <span className="text-xs font-normal text-muted-foreground">
              {data?.container_name}
            </span>
          </DialogTitle>
        </DialogHeader>

        <div className="flex items-center gap-2 px-6 pb-2">
          <Button
            variant="outline"
            size="sm"
            className="h-8 gap-1"
            onClick={() => refetch()}
            disabled={isRefetching}
          >
            <RefreshCw className={`h-3.5 w-3.5 ${isRefetching ? "animate-spin" : ""}`} />
            刷新
          </Button>
          <Button
            variant={autoRefresh ? "default" : "outline"}
            size="sm"
            className="h-8"
            onClick={() => setAutoRefresh(!autoRefresh)}
          >
            {autoRefresh ? "自动刷新中" : "自动刷新关闭"}
          </Button>
          {data?.error && (
            <span className="text-xs text-destructive ml-auto">
              错误: {data.error}
            </span>
          )}
        </div>

        <div className="flex-1 overflow-auto px-6 pb-6">
          {isLoading ? (
            <div className="h-40 animate-pulse rounded bg-muted" />
          ) : (
            <pre className="text-xs font-mono whitespace-pre-wrap break-all rounded-lg border border-border/60 bg-muted/40 p-4 leading-relaxed">
              {data?.logs || "暂无日志"}
            </pre>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
