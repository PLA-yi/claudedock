import { useState } from "react";
import { toast } from "sonner";
import { useChangeRootPassword } from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ChangeRootPasswordDialogProps {
  hostId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ChangeRootPasswordDialog({
  hostId,
  open,
  onOpenChange,
}: ChangeRootPasswordDialogProps) {
  const mutation = useChangeRootPassword();
  const [password, setPassword] = useState("");

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!password.trim()) return;

    mutation.mutate(
      { hostId, password: password.trim() },
      {
        onSuccess: () => {
          toast.success("Root 密码已修改");
          handleClose(false);
        },
        onError: () => toast.error("修改 Root 密码失败"),
      },
    );
  }

  function handleClose(v: boolean) {
    if (!v) {
      setPassword("");
    }
    onOpenChange(v);
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>修改 Root 密码</DialogTitle>
          <DialogDescription>
            设置容器内的 root 用户密码。仅在容器运行时生效。
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <Input
            type="password"
            placeholder="输入新的 root 密码"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoFocus
          />

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleClose(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={mutation.isPending || !password.trim()}
            >
              {mutation.isPending ? "处理中…" : "确认修改"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
