// 与后端 internal/store/repository/models.go 的 Bypass* 结构对齐。
// 字段全部 snake_case，handler 不做大小写转换。

export type BypassRuleType =
  | "ip"
  | "cidr"
  | "domain"
  | "domain_suffix"
  | "domain_keyword";

export interface BypassPreset {
  id: string;
  slug: string; // loopback / lan / cn-dev …
  name: string; // 中文展示名
  description: string | null;
  is_system: boolean;
  is_forced: boolean; // loopback 为 true，强制启用
  rule_count: number;
  sample_rules: BypassRuleSample[]; // 用于 Popover 展示
  created_at: string;
  updated_at: string;
}

export interface BypassRuleSample {
  rule_type: BypassRuleType;
  value: string;
}

export interface BypassRule {
  id: string;
  host_id: string;
  rule_type: BypassRuleType;
  value: string;
  port: string | null; // 单端口 "80" 或范围 "80-443"
  is_risky: boolean;
  note: string | null;
  created_at: string;
  updated_at: string;
}

export interface BypassBinding {
  id: string;
  host_id: string;
  preset_id: string;
  preset?: BypassPreset;
  created_at: string;
}

export interface BypassRuleCreatePayload {
  rule_type: BypassRuleType;
  value: string;
  port?: string;
  note?: string;
  confirm_risky?: boolean;
}

export interface BypassRuleUpdatePayload {
  value?: string;
  port?: string;
  note?: string;
  confirm_risky?: boolean;
}

export interface BypassErrorPayload {
  code: string;
  message: string;
}
