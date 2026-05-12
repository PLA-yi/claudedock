import { useState } from "react";
import { toast } from "sonner";
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
import { Input } from "@/components/ui/input";
import { useRollbackBypass } from "@/hooks/use-bypass-snapshots";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";

interface RollbackConfirmDialogProps {
  hostId: string;
  /** host.slug 字段值，用户必须严格输入此字符串才能确认回滚（T-46-13 mitigate） */
  hostSlug: string;
  /** 目标快照 ID（要回滚到的历史 snapshot） */
  targetSnapshotId: string;
  /** 目标快照版本号（仅用于 UI 文案，对应 v{N}） */
  targetVersion: number;
  /** 当前生效版本号（对应 v{current}） */
  currentVersion: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** rollback 成功后回调，外层可用于触发 ApplyProgressDialog 跟踪 task */
  onSuccess?: (taskId: string) => void;
}

/**
 * 回滚二次确认：参考 egress IP 删除确认 UX，
 * 强制要求输入 host slug 严格匹配后才能确认（与 删除高危 IP 同款体感）。
 *
 * 输入框未匹配前主按钮 disabled + 默认色；匹配后变 destructive 色。
 * 关闭 Dialog 时自动清空输入框。
 */
export function RollbackConfirmDialog({
  hostId,
  hostSlug,
  targetSnapshotId,
  targetVersion,
  currentVersion,
  open,
  onOpenChange,
  onSuccess,
}: RollbackConfirmDialogProps) {
  const [input, setInput] = useState("");
  const rollbackMutation = useRollbackBypass(hostId);
  const matched = input.trim() === hostSlug;

  return (
    <AlertDialog
      open={open}
      onOpenChange={(o) => {
        if (!o) setInput("");
        onOpenChange(o);
      }}
    >
      <AlertDialogContent data-testid="rollback-confirm-dialog">
        <AlertDialogHeader>
          <AlertDialogTitle>回滚到 v{targetVersion}？</AlertDialogTitle>
          <AlertDialogDescription>
            当前配置 v{currentVersion} 将被替换为 v{targetVersion}。需要在输入框输入 host slug{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs">
              {hostSlug}
            </code>{" "}
            以确认。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Input
          data-testid="rollback-slug-input"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="输入 host slug"
          className="font-mono"
        />
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction
            data-testid="rollback-confirm-button"
            disabled={!matched || rollbackMutation.isPending}
            className={
              matched
                ? "bg-destructive text-destructive-foreground hover:bg-destructive/90"
                : ""
            }
            onClick={(e) => {
              e.preventDefault();
              rollbackMutation.mutate(targetSnapshotId, {
                onSuccess: (resp) => {
                  toast.success(resp.message || "回滚请求已下发");
                  onSuccess?.(resp.task_id);
                  setInput("");
                  onOpenChange(false);
                },
                onError: (err) => {
                  const { message } = parseBypassError(err);
                  toast.error(message);
                },
              });
            }}
          >
            执行回滚
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
