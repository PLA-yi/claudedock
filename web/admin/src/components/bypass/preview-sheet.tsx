import { useEffect } from "react";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { usePreviewBypass } from "@/hooks/use-bypass-snapshots";
import { parseBypassError } from "@/lib/i18n/bypass-error-codes";
import type { BypassPreviewResponse } from "@/lib/api/types/bypass";
import { JSONViewer } from "./json-viewer";
import { NftDiffViewer } from "./nft-diff-viewer";

interface PreviewSheetProps {
  hostId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 用户点击「应用此配置」时调用；外层负责打开 ApplyProgressDialog */
  onApply: (preview: BypassPreviewResponse) => void;
}

/**
 * 预览生效配置 Sheet：
 * - 右侧 640px 宽，含两个 Tab：sing-box JSON / nft set diff
 * - 顶部摘要：v{current} → v{next} · {summary}
 * - 风险摘要 > 5 条高风险时主按钮变 warning 色 + 文案变更
 * - 关闭时 reset mutation 状态，下次打开重新拉 preview
 */
export function PreviewSheet({
  hostId,
  open,
  onOpenChange,
  onApply,
}: PreviewSheetProps) {
  const previewMutation = usePreviewBypass(hostId);

  useEffect(() => {
    if (open && !previewMutation.data && !previewMutation.isPending && !previewMutation.isError) {
      previewMutation.mutate(undefined, {
        onError: (err) => {
          const { message } = parseBypassError(err);
          toast.error(message);
        },
      });
    }
    if (!open) {
      previewMutation.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const preview = previewMutation.data;
  const isHighRisk = (preview?.risky_count ?? 0) > 5;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        className="flex w-[640px] flex-col gap-0 p-0 sm:max-w-[640px]"
        data-testid="preview-sheet"
      >
        <SheetHeader className="border-b p-6">
          <SheetTitle className="text-xl font-semibold tracking-tight">
            预览生效配置
          </SheetTitle>
          {preview ? (
            <SheetDescription className="font-mono text-sm">
              v{preview.version_current} → v{preview.version_next} ·{" "}
              {preview.summary}
            </SheetDescription>
          ) : (
            <SheetDescription className="text-sm">
              正在加载 sing-box 配置与 nft 规则差异…
            </SheetDescription>
          )}
        </SheetHeader>

        {previewMutation.isPending ? (
          <div className="flex flex-1 items-center justify-center p-6">
            <Loader2 className="h-6 w-6 animate-spin text-primary" />
            <span className="ml-2 text-sm text-muted-foreground">
              正在生成预览…
            </span>
          </div>
        ) : previewMutation.isError ? (
          <div
            data-testid="preview-error"
            className="flex flex-1 items-center justify-center p-6 text-sm text-destructive"
          >
            预览生成失败，请关闭重试
          </div>
        ) : preview ? (
          <div className="flex flex-1 flex-col gap-4 overflow-hidden p-6">
            <Tabs
              defaultValue="json"
              className="flex flex-1 flex-col overflow-hidden"
            >
              <TabsList>
                <TabsTrigger value="json">sing-box JSON</TabsTrigger>
                <TabsTrigger value="nft">nft set diff</TabsTrigger>
              </TabsList>
              <TabsContent value="json" className="mt-3 flex-1">
                <JSONViewer
                  value={{
                    "whitelist-cidrs.json": preview.whitelist_cidrs_rendered,
                    "whitelist-domains.json":
                      preview.whitelist_domains_rendered,
                  }}
                />
              </TabsContent>
              <TabsContent value="nft" className="mt-3 flex-1">
                <NftDiffViewer diff={preview.nft_diff} />
              </TabsContent>
            </Tabs>

            <div
              data-testid="preview-risk-summary"
              className="rounded-md border bg-muted/30 p-3"
            >
              <p className="mb-1 text-sm font-semibold">风险报告</p>
              <p className="text-xs text-muted-foreground">
                {preview.risky_count === 0
                  ? "无风险项"
                  : `含 ${preview.risky_count} 条高风险规则`}
              </p>
            </div>

            <div className="flex justify-end gap-2 border-t pt-3">
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                取消
              </Button>
              <Button
                data-testid="preview-apply-button"
                className={
                  isHighRisk
                    ? "bg-warning text-warning-foreground hover:bg-warning/90"
                    : ""
                }
                onClick={() => onApply(preview)}
              >
                {isHighRisk
                  ? `应用此配置（含 ${preview.risky_count} 条高风险）`
                  : "应用此配置"}
              </Button>
            </div>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
