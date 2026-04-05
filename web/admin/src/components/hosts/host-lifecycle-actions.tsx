import { useState } from "react";
import { Play, Square, RefreshCw, Trash2, AlertTriangle, ArrowUpCircle } from "lucide-react";
import { toast } from "sonner";
import { useNavigate } from "@tanstack/react-router";
import { useHostAction, useDeleteHost } from "@/hooks/use-hosts";
import type { HostImageInfo } from "@/hooks/use-hosts";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { RebuildDialog } from "./rebuild-dialog";

interface HostLifecycleActionsProps {
  hostId: string;
  hostStatus: string;
  imageInfo?: HostImageInfo;
}

export function HostLifecycleActions({
  hostId,
  hostStatus,
  imageInfo,
}: HostLifecycleActionsProps) {
  const navigate = useNavigate();
  const actionMutation = useHostAction();
  const deleteMutation = useDeleteHost();
  const [rebuildOpen, setRebuildOpen] = useState(false);
  const [upgradeOpen, setUpgradeOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [forceDeleteOpen, setForceDeleteOpen] = useState(false);

  function handleAction(action: "start" | "stop") {
    actionMutation.mutate(
      { hostId, action },
      {
        onSuccess: () => toast.success("操作已提交，请查看任务状态"),
        onError: () => toast.error("操作提交失败"),
      },
    );
  }

  function handleDelete(force: boolean) {
    deleteMutation.mutate(
      { hostId, force },
      {
        onSuccess: () => {
          toast.success("主机已删除");
          navigate({ to: "/hosts" });
        },
        onError: (err: any) => {
          const msg = err?.message || "删除失败";
          if (msg.includes("运行中")) {
            toast.error("主机正在运行中，请先停止或使用强制删除");
          } else {
            toast.error(msg);
          }
        },
      },
    );
  }

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          运行控制
        </p>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {(hostStatus === "stopped" || hostStatus === "failed") && (
            <Button
              className="h-11 justify-start gap-2"
              onClick={() => handleAction("start")}
              disabled={actionMutation.isPending}
            >
              <Play className="h-4 w-4 shrink-0" />
              <span className="text-left text-sm leading-snug">启动</span>
            </Button>
          )}

          {hostStatus === "running" && (
            <Button
              variant="secondary"
              className="h-11 justify-start gap-2"
              onClick={() => handleAction("stop")}
              disabled={actionMutation.isPending}
            >
              <Square className="h-4 w-4 shrink-0" />
              <span className="text-left text-sm leading-snug">停止</span>
            </Button>
          )}

          <Button
            variant="secondary"
            className="h-11 justify-start gap-2 sm:col-span-2"
            onClick={() => setRebuildOpen(true)}
            disabled={actionMutation.isPending}
          >
            <RefreshCw className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">重建主机</span>
          </Button>

          {imageInfo?.update_available && (
            <Button
              className="h-11 justify-start gap-2 sm:col-span-2 bg-emerald-600 hover:bg-emerald-700 text-white"
              onClick={() => setUpgradeOpen(true)}
              disabled={actionMutation.isPending}
            >
              <ArrowUpCircle className="h-4 w-4 shrink-0" />
              <span className="text-left text-sm leading-snug">
                升级镜像
              </span>
              <Badge variant="secondary" className="ml-auto text-[10px] px-1.5 py-0 bg-white/20 text-white">
                {imageInfo.latest_image_id}
              </Badge>
            </Button>
          )}
        </div>
      </div>

      <div className="space-y-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4">
        <p className="text-xs font-semibold uppercase tracking-wide text-destructive/90">
          危险操作
        </p>
        {hostStatus !== "running" ? (
          <Button
            variant="destructive"
            className="h-11 w-full justify-start gap-2"
            onClick={() => setDeleteOpen(true)}
            disabled={deleteMutation.isPending}
          >
            <Trash2 className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">删除主机</span>
          </Button>
        ) : (
          <Button
            variant="destructive"
            className="h-11 w-full justify-start gap-2"
            onClick={() => setForceDeleteOpen(true)}
            disabled={deleteMutation.isPending}
          >
            <AlertTriangle className="h-4 w-4 shrink-0" />
            <span className="text-left text-sm leading-snug">强制删除</span>
          </Button>
        )}
      </div>

      <RebuildDialog
        hostId={hostId}
        open={rebuildOpen}
        onOpenChange={setRebuildOpen}
      />

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除主机？</AlertDialogTitle>
            <AlertDialogDescription>
              将停止并移除 Docker 容器，删除数据库中的主机记录和出口 IP
              绑定。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => handleDelete(false)}
            >
              确认删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={forceDeleteOpen} onOpenChange={setForceDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              <span className="flex items-center gap-2 text-destructive">
                <AlertTriangle className="h-5 w-5" />
                强制删除运行中的主机？
              </span>
            </AlertDialogTitle>
            <AlertDialogDescription>
              主机当前正在运行，强制删除将立即终止容器并清除所有数据。此操作不可撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => handleDelete(true)}
            >
              强制删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={upgradeOpen} onOpenChange={setUpgradeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              <span className="flex items-center gap-2">
                <ArrowUpCircle className="h-5 w-5 text-emerald-600" />
                升级镜像版本
              </span>
            </AlertDialogTitle>
            <AlertDialogDescription className="space-y-3">
              <span className="block">
                将使用最新镜像重建主机。系统会自动拉取最新镜像后重建容器。
              </span>
              {imageInfo && (
                <span className="block rounded-md border bg-muted/50 p-3 text-xs space-y-1">
                  <span className="block"><strong>当前镜像：</strong><code>{imageInfo.container_image_id}</code></span>
                  <span className="block"><strong>最新镜像：</strong><code>{imageInfo.latest_image_id}</code></span>
                </span>
              )}
              <span className="block">
                升级会保留 home 目录数据，仅重置系统层。等同于"保留主目录"模式的重建。
              </span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-emerald-600 text-white hover:bg-emerald-700"
              onClick={() => {
                actionMutation.mutate(
                  { hostId, action: "rebuild", body: { mode: "preserve" } },
                  {
                    onSuccess: () => {
                      toast.success("升级已启动，系统正在拉取最新镜像并重建");
                      setUpgradeOpen(false);
                    },
                    onError: () => toast.error("升级操作提交失败"),
                  },
                );
              }}
            >
              确认升级
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
