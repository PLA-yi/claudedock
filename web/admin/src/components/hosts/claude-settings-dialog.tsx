import { useEffect, useState } from "react";
import { toast } from "sonner";
import {
  useClaudeSettings,
  useUpdateClaudeSettings,
} from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ClaudeSettingsDialogProps {
  hostId: string;
  hostStatus: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ClaudeSettingsDialog({
  hostId,
  hostStatus,
  open,
  onOpenChange,
}: ClaudeSettingsDialogProps) {
  const { data, isLoading } = useClaudeSettings(hostId, open && hostStatus === "running");
  const updateMutation = useUpdateClaudeSettings();
  const [text, setText] = useState("");

  useEffect(() => {
    if (data?.settings) {
      setText(JSON.stringify(data.settings, null, 2));
    }
  }, [data]);

  function handleSave() {
    try {
      const parsed = JSON.parse(text);
      updateMutation.mutate(
        { hostId, settings: parsed },
        {
          onSuccess: () => {
            toast.success("Claude 配置已保存");
            onOpenChange(false);
          },
          onError: () => toast.error("保存 Claude 配置失败"),
        },
      );
    } catch {
      toast.error("JSON 格式不合法，请检查后重试");
    }
  }

  const isRunning = hostStatus === "running";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>编辑 Claude 配置</DialogTitle>
          <DialogDescription>
            读取和修改容器内的 <code>/workspace/.claude/settings.json</code>。
          </DialogDescription>
        </DialogHeader>

        {!isRunning ? (
          <p className="text-sm text-muted-foreground">
            容器未运行，无法读取配置。
          </p>
        ) : isLoading ? (
          <div className="h-64 animate-pulse rounded bg-muted" />
        ) : (
          <Textarea
            className="min-h-[20rem] font-mono text-sm"
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            onClick={handleSave}
            disabled={!isRunning || isLoading || updateMutation.isPending}
          >
            {updateMutation.isPending ? "保存中…" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
