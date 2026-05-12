import { useState } from "react";
import { Eye, Shield } from "lucide-react";
import { PresetGrid } from "./preset-grid";
import { CustomRulesTable } from "./custom-rules-table";
import { PreviewSheet } from "./preview-sheet";
import { ApplyProgressDialog } from "./apply-progress-dialog";
import { useBypassRules } from "@/hooks/use-bypass-rules";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import type { BypassPreviewResponse } from "@/lib/api/types/bypass";

interface BypassTabProps {
  hostId: string;
}

/**
 * 代理白名单 Tab 顶层容器。
 *
 * 结构：标题（含规则数 Badge）→ 预设区 → 自定义规则表 →
 *   右下角 sticky「查看生效预览」按钮 → PreviewSheet → ApplyProgressDialog。
 *
 * 子流程联动：
 * - 用户点击「查看生效预览」打开 PreviewSheet（自动调 preview mutation）
 * - PreviewSheet 内部「应用此配置」回调把 risky_count 暂存 + 关闭 Sheet + 打开 ApplyDialog
 * - ApplyProgressDialog 自动触发 apply mutation 并依据 task.progress_percent 推进 5 阶段
 * - RollbackConfirmDialog 在本 plan 暂不接入 UI（待 SnapshotHistory 组件落地）
 */
export function BypassTab({ hostId }: BypassTabProps) {
  const rulesQuery = useBypassRules(hostId);
  const ruleCount = rulesQuery.data?.rules.length ?? 0;

  const [previewOpen, setPreviewOpen] = useState(false);
  const [applyOpen, setApplyOpen] = useState(false);
  const [pendingRiskyCount, setPendingRiskyCount] = useState(0);

  function handleApplyFromPreview(preview: BypassPreviewResponse) {
    setPendingRiskyCount(preview.risky_count);
    setPreviewOpen(false);
    setApplyOpen(true);
  }

  return (
    <div className="relative space-y-6" data-testid="bypass-tab">
      <header className="flex items-center gap-2">
        <Shield className="size-5 text-primary" />
        <h2 className="text-base font-semibold">代理白名单</h2>
        {ruleCount > 0 && (
          <Badge variant="secondary" className="font-normal">
            {ruleCount} 条规则
          </Badge>
        )}
      </header>

      <section className="space-y-3">
        <div>
          <h3 className="text-base font-semibold">预设规则集</h3>
          <p className="text-xs text-muted-foreground">
            选中预设以快速启用一组系统维护的白名单规则
          </p>
        </div>
        <PresetGrid hostId={hostId} />
      </section>

      <section>
        <CustomRulesTable hostId={hostId} />
      </section>

      {/* sticky 右下角浮动按钮，便于用户在长列表底部仍可触发预览 */}
      <div className="sticky bottom-4 flex justify-end">
        <Button
          data-testid="bypass-open-preview-button"
          size="lg"
          className="shadow-lg"
          onClick={() => setPreviewOpen(true)}
        >
          <Eye className="mr-2 size-4" />
          查看生效预览
        </Button>
      </div>

      <PreviewSheet
        hostId={hostId}
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        onApply={handleApplyFromPreview}
      />

      <ApplyProgressDialog
        hostId={hostId}
        open={applyOpen}
        onOpenChange={setApplyOpen}
        riskyCount={pendingRiskyCount}
      />
    </div>
  );
}
