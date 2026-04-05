import { useEffect, useState } from "react";
import { toast } from "sonner";
import {
  useClaudeSettings,
  useUpdateClaudeSettings,
} from "@/hooks/use-hosts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";

interface ClaudeSettingsDialogProps {
  hostId: string;
  hostStatus: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type SettingsObj = Record<string, unknown>;
type PermissionsObj = {
  defaultMode?: string;
  allow?: string[];
  deny?: string[];
};

const PERMISSION_MODES = [
  { value: "default", label: "default", desc: "只自动通过只读操作" },
  { value: "acceptEdits", label: "acceptEdits", desc: "自动通过读取和编辑" },
  { value: "plan", label: "plan", desc: "仅规划模式，只读" },
  { value: "auto", label: "auto", desc: "自动通过所有操作（后台安全检查）" },
  { value: "dontAsk", label: "dontAsk", desc: "仅允许预批准工具" },
  { value: "bypassPermissions", label: "bypassPermissions", desc: "跳过所有权限检查（仅限容器）" },
] as const;

function getPermissions(settings: SettingsObj): PermissionsObj {
  const p = settings.permissions;
  if (p && typeof p === "object" && !Array.isArray(p)) return p as PermissionsObj;
  return {};
}

function Toggle({
  checked,
  onChange,
  label,
  description,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
  description?: string;
}) {
  return (
    <button
      type="button"
      className="flex w-full items-center justify-between rounded-lg border border-border/60 p-3 text-left hover:bg-muted/30"
      onClick={() => onChange(!checked)}
    >
      <div className="space-y-0.5">
        <p className="text-sm font-medium">{label}</p>
        {description && (
          <p className="text-xs text-muted-foreground">{description}</p>
        )}
      </div>
      <div
        className={`relative h-5 w-9 rounded-full transition-colors ${checked ? "bg-primary" : "bg-muted-foreground/30"}`}
      >
        <div
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform ${checked ? "translate-x-4" : "translate-x-0.5"}`}
        />
      </div>
    </button>
  );
}

function RuleList({
  label,
  rules,
  onChange,
  placeholder,
}: {
  label: string;
  rules: string[];
  onChange: (rules: string[]) => void;
  placeholder: string;
}) {
  const [draft, setDraft] = useState("");

  function addRule() {
    const trimmed = draft.trim();
    if (trimmed && !rules.includes(trimmed)) {
      onChange([...rules, trimmed]);
      setDraft("");
    }
  }

  function removeRule(index: number) {
    onChange(rules.filter((_, i) => i !== index));
  }

  return (
    <div className="space-y-2">
      <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </Label>
      {rules.length > 0 && (
        <div className="space-y-1">
          {rules.map((rule, i) => (
            <div
              key={i}
              className="flex items-center justify-between rounded border border-border/40 bg-muted/20 px-3 py-1.5"
            >
              <code className="text-xs">{rule}</code>
              <button
                type="button"
                className="text-xs text-muted-foreground hover:text-destructive"
                onClick={() => removeRule(i)}
              >
                ✕
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="flex gap-2">
        <Input
          className="h-8 font-mono text-xs"
          placeholder={placeholder}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addRule();
            }
          }}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 shrink-0"
          onClick={addRule}
          disabled={!draft.trim()}
        >
          添加
        </Button>
      </div>
    </div>
  );
}

type TabId = "general" | "permissions" | "raw";

export function ClaudeSettingsDialog({
  hostId,
  hostStatus,
  open,
  onOpenChange,
}: ClaudeSettingsDialogProps) {
  const { data, isLoading } = useClaudeSettings(hostId, open && hostStatus === "running");
  const updateMutation = useUpdateClaudeSettings();
  const [settings, setSettings] = useState<SettingsObj>({});
  const [rawText, setRawText] = useState("");
  const [activeTab, setActiveTab] = useState<TabId>("general");

  useEffect(() => {
    if (data?.settings) {
      setSettings(data.settings as SettingsObj);
      setRawText(JSON.stringify(data.settings, null, 2));
    }
  }, [data]);

  function updateField(key: string, value: unknown) {
    setSettings((prev) => {
      const next = { ...prev };
      if (value === undefined || value === "" || value === null) {
        delete next[key];
      } else {
        next[key] = value;
      }
      return next;
    });
  }

  function updatePermissions(update: Partial<PermissionsObj>) {
    setSettings((prev) => {
      const perms = { ...getPermissions(prev), ...update };
      Object.keys(perms).forEach((k) => {
        const key = k as keyof PermissionsObj;
        const v = perms[key];
        if (v === undefined || v === "" || (Array.isArray(v) && v.length === 0)) {
          delete perms[key];
        }
      });
      const next = { ...prev };
      if (Object.keys(perms).length === 0) {
        delete next.permissions;
      } else {
        next.permissions = perms;
      }
      return next;
    });
  }

  function handleSave() {
    let toSave = settings;
    if (activeTab === "raw") {
      try {
        toSave = JSON.parse(rawText);
      } catch {
        toast.error("JSON 格式不合法，请检查后重试");
        return;
      }
    }
    updateMutation.mutate(
      { hostId, settings: toSave },
      {
        onSuccess: () => {
          toast.success("Claude 配置已保存");
          onOpenChange(false);
        },
        onError: () => toast.error("保存 Claude 配置失败"),
      },
    );
  }

  function syncRawFromStructured() {
    setRawText(JSON.stringify(settings, null, 2));
  }

  function syncStructuredFromRaw() {
    try {
      setSettings(JSON.parse(rawText));
    } catch {
      /* keep previous */
    }
  }

  const isRunning = hostStatus === "running";
  const perms = getPermissions(settings);

  const tabs: { id: TabId; label: string }[] = [
    { id: "general", label: "常规" },
    { id: "permissions", label: "权限" },
    { id: "raw", label: "JSON" },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>Claude Code 配置</DialogTitle>
          <DialogDescription>
            编辑容器内的 <code className="text-xs">/workspace/.claude/settings.json</code>
          </DialogDescription>
        </DialogHeader>

        {!isRunning ? (
          <p className="text-sm text-muted-foreground">容器未运行，无法读取配置。</p>
        ) : isLoading ? (
          <div className="h-64 animate-pulse rounded bg-muted" />
        ) : (
          <div className="flex-1 overflow-hidden flex flex-col">
            <div className="flex gap-1 border-b border-border/60 mb-4">
              {tabs.map((tab) => (
                <button
                  key={tab.id}
                  type="button"
                  className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px ${
                    activeTab === tab.id
                      ? "border-primary text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground"
                  }`}
                  onClick={() => {
                    if (activeTab === "raw" && tab.id !== "raw") syncStructuredFromRaw();
                    if (activeTab !== "raw" && tab.id === "raw") syncRawFromStructured();
                    setActiveTab(tab.id);
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            <div className="flex-1 overflow-y-auto pr-1 space-y-5">
              {activeTab === "general" && (
                <>
                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      默认模型
                    </Label>
                    <Input
                      className="h-9 font-mono text-sm"
                      placeholder="留空使用默认（claude-sonnet-4-6）"
                      value={(settings.model as string) ?? ""}
                      onChange={(e) => updateField("model", e.target.value || undefined)}
                    />
                  </div>

                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      响应语言
                    </Label>
                    <Input
                      className="h-9 text-sm"
                      placeholder="例如 chinese, japanese, english"
                      value={(settings.language as string) ?? ""}
                      onChange={(e) => updateField("language", e.target.value || undefined)}
                    />
                  </div>

                  <Toggle
                    label="始终启用深度思考"
                    description="alwaysThinkingEnabled — 所有会话默认开启 extended thinking"
                    checked={!!settings.alwaysThinkingEnabled}
                    onChange={(v) => updateField("alwaysThinkingEnabled", v || undefined)}
                  />

                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      思考力度
                    </Label>
                    <Select
                      value={(settings.effortLevel as string) ?? ""}
                      onValueChange={(v) => updateField("effortLevel", v === "__none__" ? undefined : v)}
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue placeholder="默认" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="__none__">默认</SelectItem>
                        <SelectItem value="low">low — 快速回答</SelectItem>
                        <SelectItem value="medium">medium — 平衡</SelectItem>
                        <SelectItem value="high">high — 深入思考</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      自动更新频道
                    </Label>
                    <Select
                      value={(settings.autoUpdatesChannel as string) ?? "latest"}
                      onValueChange={(v) => updateField("autoUpdatesChannel", v === "latest" ? undefined : v)}
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="latest">latest — 最新版本</SelectItem>
                        <SelectItem value="stable">stable — 稳定版（约延迟一周）</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <Separator />

                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      环境变量
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      在「JSON」标签中直接编辑 <code>env</code> 字段设置自定义环境变量。
                    </p>
                  </div>
                </>
              )}

              {activeTab === "permissions" && (
                <>
                  <div className="space-y-2">
                    <Label className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                      默认权限模式
                    </Label>
                    <Select
                      value={perms.defaultMode ?? "default"}
                      onValueChange={(v) =>
                        updatePermissions({ defaultMode: v === "default" ? undefined : v })
                      }
                    >
                      <SelectTrigger className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {PERMISSION_MODES.map((m) => (
                          <SelectItem key={m.value} value={m.value}>
                            <span className="font-mono text-xs">{m.label}</span>
                            <span className="ml-2 text-xs text-muted-foreground">
                              — {m.desc}
                            </span>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>

                  <Separator />

                  <RuleList
                    label="允许规则 (allow)"
                    rules={perms.allow ?? []}
                    onChange={(rules) => updatePermissions({ allow: rules.length > 0 ? rules : undefined })}
                    placeholder='例如 Bash(npm run *)'
                  />

                  <RuleList
                    label="拒绝规则 (deny)"
                    rules={perms.deny ?? []}
                    onChange={(rules) => updatePermissions({ deny: rules.length > 0 ? rules : undefined })}
                    placeholder='例如 Read(./.env)'
                  />
                </>
              )}

              {activeTab === "raw" && (
                <Textarea
                  className="min-h-80 font-mono text-sm"
                  value={rawText}
                  onChange={(e) => setRawText(e.target.value)}
                />
              )}
            </div>
          </div>
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
