export interface SecurityEvent {
  id: string;
  occurred_at: string;
  event_type: string;
  severity: "info" | "warning" | "critical";
  actor_user_id: string | null;
  source_ip: string;
  ip_class: "private" | "public";
  module: string;
  record_ref: string | null;
  summary: string;
  hash: string;
}

export interface SecurityRule {
  rule_key: string;
  enabled: boolean;
  config_json: string;
  updated_at: string;
}

export interface UpdateSecurityRuleInput {
  enabled?: boolean;
  config_json?: string;
}

export interface SecuritySettings {
  deployment_posture: "lan_only" | "internet_exposed";
  retention_days: number;
  updated_at: string;
}

export interface UpdateSecuritySettingsInput {
  deployment_posture?: "lan_only" | "internet_exposed";
  retention_days?: number;
}

export interface SecurityAlertRecipient {
  id: string;
  kind: "user" | "email";
  user_id: string | null;
  email: string | null;
  scope_type: "severity" | "rule";
  scope_value: string;
  channel: "in_app" | "email" | "both";
  created_at: string;
}

export interface AddSecurityAlertRecipientInput {
  user_id?: string;
  email?: string;
  scope_type: "severity" | "rule";
  scope_value: string;
  channel: "in_app" | "email" | "both";
}
