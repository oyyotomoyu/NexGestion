import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import {
  addSecurityRecipient,
  getSecuritySettings,
  listSecurityEvents,
  listSecurityRecipients,
  listSecurityRules,
  removeSecurityRecipient,
  sendSecurityTestAlert,
  updateSecurityRule,
  updateSecuritySettings,
} from "@/requests/security";
import type {
  SecurityAlertRecipient,
  SecurityEvent,
  SecurityRule,
  SecuritySettings,
} from "@/requests/security/types";

import "./style.css";

const SEVERITY_COLOR: Record<SecurityEvent["severity"], "muted" | "warning" | "danger"> = {
  info: "muted",
  warning: "warning",
  critical: "danger",
};

export default function Security() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const hasPermission = (key: string) =>
    user?.is_protected === true || user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true;
  const canReadEvents = hasPermission("security.events.read");
  const canManageRules = hasPermission("security.rules.manage");
  const canManageAlerts = hasPermission("security.alerts.manage");

  const [events, setEvents] = useState<SecurityEvent[]>([]);
  const [rules, setRules] = useState<SecurityRule[]>([]);
  const [settings, setSettings] = useState<SecuritySettings | null>(null);
  const [recipients, setRecipients] = useState<SecurityAlertRecipient[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState(false);
  const [testAlertSent, setTestAlertSent] = useState(false);
  const [recipientTarget, setRecipientTarget] = useState("");
  const [recipientScopeValue, setRecipientScopeValue] = useState("critical");
  const [recipientChannel, setRecipientChannel] = useState<"in_app" | "email" | "both">("email");

  useEffect(() => {
    let active = true;
    const loaders: Promise<unknown>[] = [];
    if (canReadEvents) loaders.push(listSecurityEvents().then((result) => active && setEvents(result)));
    if (canManageRules || canReadEvents) loaders.push(listSecurityRules().then((result) => active && setRules(result)));
    if (canManageRules || canReadEvents) loaders.push(getSecuritySettings().then((result) => active && setSettings(result)));
    if (canManageAlerts) loaders.push(listSecurityRecipients().then((result) => active && setRecipients(result)));
    Promise.all(loaders)
      .catch(() => active && setError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function toggleRule(rule: SecurityRule) {
    setSaving(true);
    setError(false);
    try {
      const updated = await updateSecurityRule(rule.rule_key, { enabled: !rule.enabled });
      setRules((current) => current.map((item) => (item.rule_key === rule.rule_key ? updated : item)));
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  async function changePosture(posture: SecuritySettings["deployment_posture"]) {
    if (posture === "internet_exposed" && !window.confirm(t("global.k_Settings_Security_Posture_ConfirmInternetExposed"))) {
      return;
    }
    setSaving(true);
    setError(false);
    try {
      const updated = await updateSecuritySettings({ deployment_posture: posture });
      setSettings(updated);
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  async function addRecipient(event: FormEvent) {
    event.preventDefault();
    if (!recipientTarget.trim()) return;
    setSaving(true);
    setError(false);
    try {
      const isEmail = recipientTarget.includes("@");
      const created = await addSecurityRecipient({
        ...(isEmail ? { email: recipientTarget.trim() } : { user_id: recipientTarget.trim() }),
        scope_type: "severity",
        scope_value: recipientScopeValue,
        channel: recipientChannel,
      });
      setRecipients((current) => [...current, created]);
      setRecipientTarget("");
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  async function removeRecipient(id: string) {
    setSaving(true);
    setError(false);
    try {
      await removeSecurityRecipient(id);
      setRecipients((current) => current.filter((recipient) => recipient.id !== id));
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  async function sendTestAlert() {
    setSaving(true);
    setError(false);
    setTestAlertSent(false);
    try {
      await sendSecurityTestAlert();
      setTestAlertSent(true);
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <NexText color="muted">{t("global.k_Common_Loading")}</NexText>;

  return (
    <section className="security-page">
      <header className="security-page__header">
        <NexText variant="heading">{t("global.k_Settings_Security_Title")}</NexText>
        <NexText color="muted">{t("global.k_Settings_Security_Description")}</NexText>
      </header>

      {error ? <NexText color="danger">{t("global.k_Settings_Security_Error")}</NexText> : null}

      {(canManageRules || canReadEvents) && settings ? (
        <div className="security-card">
          <NexText variant="subheading">{t("global.k_Settings_Security_Posture_Title")}</NexText>
          <NexText color="muted">{t("global.k_Settings_Security_Posture_Description")}</NexText>
          {canManageRules ? (
            <NexSelect
              ariaLabel={t("global.k_Settings_Security_Posture_Title")}
              value={settings.deployment_posture}
              onChange={(value) => void changePosture(value)}
              options={[
                { value: "lan_only", label: t("global.k_Settings_Security_Posture_LanOnly") },
                { value: "internet_exposed", label: t("global.k_Settings_Security_Posture_InternetExposed") },
              ]}
            />
          ) : (
            <NexText as="span" weight={600}>
              {settings.deployment_posture === "lan_only"
                ? t("global.k_Settings_Security_Posture_LanOnly")
                : t("global.k_Settings_Security_Posture_InternetExposed")}
            </NexText>
          )}
          <NexText as="span" color="muted" variant="caption">
            {t("global.k_Settings_Security_RetentionDays", { count: settings.retention_days })}
          </NexText>
        </div>
      ) : null}

      {(canManageRules || canReadEvents) && rules.length ? (
        <div className="security-card">
          <NexText variant="subheading">{t("global.k_Settings_Security_Rules_Title")}</NexText>
          <div className="security-rule-table-wrap">
            <table className="security-rule-table">
              <thead>
                <tr>
                  <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Rules_Rule")}</NexText></th>
                  <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Rules_Config")}</NexText></th>
                  <th scope="col" className="security-rule-table__actions-heading">
                    <NexText as="span" variant="label">{t("global.k_Common_Actions")}</NexText>
                  </th>
                </tr>
              </thead>
              <tbody>
                {rules.map((rule) => (
                  <tr key={rule.rule_key}>
                    <td data-label={t("global.k_Settings_Security_Rules_Rule")}>
                      <NexText as="span" weight={600}>{rule.rule_key.replace(/_/g, " ")}</NexText>
                    </td>
                    <td data-label={t("global.k_Settings_Security_Rules_Config")}>
                      <NexText as="span" color="muted" variant="caption">{rule.config_json}</NexText>
                    </td>
                    <td className="security-rule-table__actions-cell" data-label={t("global.k_Common_Actions")}>
                      {canManageRules ? (
                        <NexButton
                          type="button"
                          variant={rule.enabled ? "danger" : "secondary"}
                          size="compact"
                          disabled={saving}
                          onClick={() => void toggleRule(rule)}
                        >
                          {rule.enabled ? t("global.k_Settings_Security_Rules_Disable") : t("global.k_Settings_Security_Rules_Enable")}
                        </NexButton>
                      ) : (
                        <NexText as="span" color={rule.enabled ? "primary" : "muted"} variant="caption">
                          {rule.enabled ? t("global.k_Settings_Security_Rules_Enabled") : t("global.k_Settings_Security_Rules_Disabled")}
                        </NexText>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}

      {canManageAlerts ? (
        <div className="security-card">
          <NexText variant="subheading">{t("global.k_Settings_Security_Recipients_Title")}</NexText>
          <NexText color="muted">{t("global.k_Settings_Security_Recipients_Description")}</NexText>
          <form className="security-recipient-form" onSubmit={(event) => void addRecipient(event)}>
            <NexInput
              id="security-recipient-target"
              label={t("global.k_Settings_Security_Recipients_Target")}
              value={recipientTarget}
              placeholder="owner@example.com"
              required
              onChange={(event) => setRecipientTarget(event.target.value)}
            />
            <NexInput
              id="security-recipient-scope"
              label={t("global.k_Settings_Security_Recipients_Severity")}
              value={recipientScopeValue}
              onChange={(event) => setRecipientScopeValue(event.target.value)}
            />
            <NexSelect
              ariaLabel={t("global.k_Settings_Security_Recipients_Channel")}
              value={recipientChannel}
              onChange={setRecipientChannel}
              options={[
                { value: "email", label: t("global.k_Settings_Security_Recipients_ChannelEmail") },
                { value: "in_app", label: t("global.k_Settings_Security_Recipients_ChannelInApp") },
                { value: "both", label: t("global.k_Settings_Security_Recipients_ChannelBoth") },
              ]}
            />
            <NexButton type="submit" size="compact" disabled={saving || !recipientTarget.trim()}>
              {t("global.k_Settings_Security_Recipients_Add")}
            </NexButton>
          </form>
          {recipients.length ? (
            <ul className="security-recipient-list">
              {recipients.map((recipient) => (
                <li key={recipient.id}>
                  <NexText as="span">{recipient.email ?? recipient.user_id}</NexText>
                  <NexText as="span" color="muted" variant="caption">
                    {recipient.scope_type}:{recipient.scope_value} · {recipient.channel}
                  </NexText>
                  <NexButton type="button" variant="danger" size="compact" disabled={saving} onClick={() => void removeRecipient(recipient.id)}>
                    {t("global.k_Common_Delete")}
                  </NexButton>
                </li>
              ))}
            </ul>
          ) : (
            <NexText color="muted">{t("global.k_Settings_Security_Recipients_Empty")}</NexText>
          )}
          <div>
            <NexButton type="button" variant="secondary" disabled={saving} onClick={() => void sendTestAlert()}>
              {t("global.k_Settings_Security_TestAlert_Button")}
            </NexButton>
            {testAlertSent ? (
              <NexText as="span" color="primary" variant="caption">
                {t("global.k_Settings_Security_TestAlert_Sent")}
              </NexText>
            ) : null}
          </div>
        </div>
      ) : null}

      {canReadEvents ? (
        <div className="security-card">
          <NexText variant="subheading">{t("global.k_Settings_Security_Events_Title")}</NexText>
          {events.length === 0 ? (
            <NexText color="muted">{t("global.k_Settings_Security_Events_Empty")}</NexText>
          ) : (
            <div className="security-event-table-wrap">
              <table className="security-event-table">
                <thead>
                  <tr>
                    <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Events_Time")}</NexText></th>
                    <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Events_Severity")}</NexText></th>
                    <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Events_Type")}</NexText></th>
                    <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Events_Source")}</NexText></th>
                    <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Security_Events_Summary")}</NexText></th>
                  </tr>
                </thead>
                <tbody>
                  {events.map((event) => (
                    <tr key={event.id}>
                      <td data-label={t("global.k_Settings_Security_Events_Time")}>
                        <NexText as="span" variant="caption">{new Date(event.occurred_at).toLocaleString()}</NexText>
                      </td>
                      <td data-label={t("global.k_Settings_Security_Events_Severity")}>
                        <NexText as="span" color={SEVERITY_COLOR[event.severity]} weight={600}>{event.severity}</NexText>
                      </td>
                      <td data-label={t("global.k_Settings_Security_Events_Type")}>
                        <NexText as="span">{event.event_type.replace(/_/g, " ")}</NexText>
                      </td>
                      <td data-label={t("global.k_Settings_Security_Events_Source")}>
                        <NexText as="span" color="muted" variant="caption">{event.source_ip} ({event.ip_class})</NexText>
                      </td>
                      <td data-label={t("global.k_Settings_Security_Events_Summary")}>
                        <NexText as="span" color="muted">{event.summary}</NexText>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      ) : null}
    </section>
  );
}
