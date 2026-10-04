import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type {
  AddSecurityAlertRecipientInput,
  SecurityAlertRecipient,
  SecurityEvent,
  SecurityRule,
  SecuritySettings,
  UpdateSecurityRuleInput,
  UpdateSecuritySettingsInput,
} from "./types";

const mockEvents: SecurityEvent[] = [
  {
    id: "evt-1",
    occurred_at: new Date().toISOString(),
    event_type: "login_succeeded",
    severity: "info",
    actor_user_id: "user-admin",
    source_ip: "192.168.1.10",
    ip_class: "private",
    module: "users",
    record_ref: null,
    summary: "login succeeded",
    hash: "mock-hash-1",
  },
];

const mockRules: SecurityRule[] = [
  { rule_key: "brute_force_login", enabled: true, config_json: '{"threshold":5,"window_minutes":10}', updated_at: new Date().toISOString() },
  { rule_key: "privilege_escalation", enabled: true, config_json: "{}", updated_at: new Date().toISOString() },
  { rule_key: "mass_export", enabled: true, config_json: '{"threshold":20,"window_minutes":10}', updated_at: new Date().toISOString() },
  { rule_key: "new_device_privileged_login", enabled: true, config_json: "{}", updated_at: new Date().toISOString() },
  { rule_key: "network_change", enabled: false, config_json: "{}", updated_at: new Date().toISOString() },
  { rule_key: "off_hours_privileged_action", enabled: false, config_json: "{}", updated_at: new Date().toISOString() },
];

let mockSettings: SecuritySettings = {
  deployment_posture: "lan_only",
  retention_days: 365,
  updated_at: new Date().toISOString(),
};

let mockRecipients: SecurityAlertRecipient[] = [];

export async function listSecurityEvents(query: ListQuery = {}) {
  if (process.env.NODE_ENV !== "production") {
    return [...mockEvents];
  }
  const response = await request<ListResponse<SecurityEvent, "events">>(
    buildListPath("/api/security/events", { sort: "occurred_at", order: "desc", page_size: 50, ...query }),
  );
  return listItems(response, "events");
}

export function listSecurityRules() {
  if (process.env.NODE_ENV !== "production") {
    return Promise.resolve([...mockRules]);
  }
  return request<{ rules: SecurityRule[] }>("/api/security/rules").then((result) => result.rules);
}

export function updateSecurityRule(ruleKey: string, input: UpdateSecurityRuleInput) {
  if (process.env.NODE_ENV !== "production") {
    const index = mockRules.findIndex((rule) => rule.rule_key === ruleKey);
    if (index < 0) return Promise.reject(new Error(`Mock rule ${ruleKey} was not found`));
    const updated: SecurityRule = {
      ...mockRules[index],
      enabled: input.enabled ?? mockRules[index].enabled,
      config_json: input.config_json ?? mockRules[index].config_json,
      updated_at: new Date().toISOString(),
    };
    mockRules[index] = updated;
    return Promise.resolve({ ...updated });
  }
  return request<SecurityRule>(`/api/security/rules/${encodeURIComponent(ruleKey)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function getSecuritySettings() {
  if (process.env.NODE_ENV !== "production") {
    return Promise.resolve({ ...mockSettings });
  }
  return request<SecuritySettings>("/api/security/settings");
}

export function updateSecuritySettings(input: UpdateSecuritySettingsInput) {
  if (process.env.NODE_ENV !== "production") {
    mockSettings = { ...mockSettings, ...input, updated_at: new Date().toISOString() };
    return Promise.resolve({ ...mockSettings });
  }
  return request<SecuritySettings>("/api/security/settings", {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function listSecurityRecipients() {
  if (process.env.NODE_ENV !== "production") {
    return Promise.resolve([...mockRecipients]);
  }
  return request<{ recipients: SecurityAlertRecipient[] }>("/api/security/recipients").then((result) => result.recipients);
}

export function addSecurityRecipient(input: AddSecurityAlertRecipientInput) {
  if (process.env.NODE_ENV !== "production") {
    const recipient: SecurityAlertRecipient = {
      id: crypto.randomUUID(),
      kind: input.user_id ? "user" : "email",
      user_id: input.user_id ?? null,
      email: input.email ?? null,
      scope_type: input.scope_type,
      scope_value: input.scope_value,
      channel: input.channel,
      created_at: new Date().toISOString(),
    };
    mockRecipients = [...mockRecipients, recipient];
    return Promise.resolve({ ...recipient });
  }
  return request<SecurityAlertRecipient>("/api/security/recipients", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function removeSecurityRecipient(id: string) {
  if (process.env.NODE_ENV !== "production") {
    mockRecipients = mockRecipients.filter((recipient) => recipient.id !== id);
    return Promise.resolve();
  }
  return request<void>(`/api/security/recipients/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function sendSecurityTestAlert() {
  if (process.env.NODE_ENV !== "production") {
    return Promise.resolve({ ...mockEvents[0] });
  }
  return request<SecurityEvent>("/api/security/test-alert", { method: "POST" });
}
