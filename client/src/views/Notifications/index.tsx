import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  hideNotification,
  listAdminNotifications,
  listNotifications,
  notificationCSVURL,
} from "@/requests/notifications";
import type { Notification, NotificationAudience } from "@/requests/notifications/types";
import type { User } from "@/requests/users/types";

import "./style.css";

type ViewMode = "inbox" | "all";

function currentMonth() {
  return new Date().toLocaleDateString("en-CA").slice(0, 7);
}

function hasPermission(user: User | null, key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some(
      (role) =>
        role.grants_all_permissions ||
        role.permissions.some((permission) => permission.permission_key === key),
    ) === true
  );
}

function formatDateTime(value: string | null | undefined, locale: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function audienceLabel(
  audience: NotificationAudience,
  t: (key: string, options?: Record<string, unknown>) => string,
) {
  if (audience.scope === "organization") return t("global.k_Notifications_AudienceOrganization");
  const target =
    audience.target_group_id || audience.target_role_id || audience.target_user_id || "-";
  return t(`global.k_Notifications_Audience_${audience.scope}`, { target });
}

function sortNotifications(items: Notification[]) {
  return [...items].sort((left, right) => {
    const severity = right.type.severity - left.type.severity;
    if (severity !== 0) return severity;
    return Date.parse(right.updated_at) - Date.parse(left.updated_at);
  });
}

export default function Notifications() {
  const { i18n, t } = useTranslation("ui");
  const { user } = useAuth();
  const locale = i18n.resolvedLanguage || navigator.language;
  const canSeeAll = hasPermission(user, "notifications.manage");
  const [mode, setMode] = useState<ViewMode>("inbox");
  const [items, setItems] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [month, setMonth] = useState(currentMonth());

  useDocumentTitle(t("global.k_Notifications_PageTitle"));

  const visibleMode = canSeeAll ? mode : "inbox";

  useEffect(() => {
    if (!canSeeAll && mode === "all") setMode("inbox");
  }, [canSeeAll, mode]);

  useEffect(() => {
    let mounted = true;
    async function load() {
      setLoading(true);
      setError(null);
      try {
        const result =
          visibleMode === "all" ? await listAdminNotifications() : await listNotifications();
        if (mounted) setItems(sortNotifications(result));
      } catch {
        if (mounted) setError(t("global.k_Notifications_Error"));
      } finally {
        if (mounted) setLoading(false);
      }
    }
    void load();
    return () => {
      mounted = false;
    };
  }, [visibleMode, t]);

  const counts = useMemo(
    () => ({
      active: items.filter((item) => item.status === "active" || item.status === "edited").length,
      total: items.length,
    }),
    [items],
  );

  async function hide(id: string) {
    setSavingId(id);
    setError(null);
    try {
      await hideNotification(id);
      const result =
        visibleMode === "all" ? await listAdminNotifications() : await listNotifications();
      setItems(sortNotifications(result));
    } catch {
      setError(t("global.k_Notifications_HideError"));
    } finally {
      setSavingId(null);
    }
  }

  return (
    <section className="notifications-page">
      <header className="notifications-page__header">
        <div>
          <NexText variant="title">{t("global.k_Notifications_Title")}</NexText>
          <NexText color="muted">{t("global.k_Notifications_Description")}</NexText>
        </div>
        {canSeeAll && (
          <div className="notifications-export">
            <NexInput
              id="notification-export-month"
              label={t("global.k_Notifications_ExportMonth")}
              type="month"
              value={month}
              onChange={(event) => setMonth(event.target.value)}
            />
            <a className="notifications-export__link" href={notificationCSVURL(month)}>
              {t("global.k_Notifications_DownloadCSV")}
            </a>
          </div>
        )}
      </header>

      <div className="notifications-toolbar" role="tablist" aria-label={t("global.k_Notifications_View")}>
        <button
          className={visibleMode === "inbox" ? "active" : ""}
          type="button"
          role="tab"
          aria-selected={visibleMode === "inbox"}
          onClick={() => setMode("inbox")}
        >
          <NexText as="span" variant="button" color="inherit">
            {t("global.k_Notifications_Inbox")}
          </NexText>
        </button>
        {canSeeAll && (
          <button
            className={visibleMode === "all" ? "active" : ""}
            type="button"
            role="tab"
            aria-selected={visibleMode === "all"}
            onClick={() => setMode("all")}
          >
            <NexText as="span" variant="button" color="inherit">
              {t("global.k_Notifications_SeeAll")}
            </NexText>
          </button>
        )}
      </div>

      <section className="notifications-summary" aria-label={t("global.k_Notifications_Summary")}>
        <div>
          <NexText variant="heading">{counts.active}</NexText>
          <NexText color="muted">{t("global.k_Notifications_ActiveCount")}</NexText>
        </div>
        <div>
          <NexText variant="heading">{counts.total}</NexText>
          <NexText color="muted">{t("global.k_Notifications_TotalCount")}</NexText>
        </div>
      </section>

      {error && (
        <div className="notifications-alert notifications-alert--error">
          <NexText color="danger">{error}</NexText>
        </div>
      )}

      {loading ? (
        <div className="notifications-loading">
          <span className="notifications-loading__dot" />
          <NexText color="muted">{t("global.k_Common_Loading")}</NexText>
        </div>
      ) : items.length === 0 ? (
        <div className="notifications-empty">
          <NexText color="muted">{t("global.k_Notifications_Empty")}</NexText>
        </div>
      ) : (
        <div className="notifications-list">
          {items.map((item) => {
            const canHide = item.sender_user_id === user?.id && item.status !== "hidden";
            return (
              <article className="notifications-item" key={item.id}>
                <header className="notifications-item__header">
                  <div>
                    <div className="notifications-item__meta">
                      <span className={`notifications-badge notifications-badge--${item.type.code}`}>
                        {item.type.name}
                      </span>
                      <span className={`notifications-status notifications-status--${item.status}`}>
                        {t(`global.k_Notifications_Status_${item.status}`)}
                      </span>
                    </div>
                    <NexText variant="heading">{item.title}</NexText>
                  </div>
                  {canHide && (
                    <NexButton
                      type="button"
                      variant="secondary"
                      size="compact"
                      disabled={savingId === item.id}
                      onClick={() => void hide(item.id)}
                    >
                      {savingId === item.id
                        ? t("global.k_Common_Saving")
                        : t("global.k_Notifications_Hide")}
                    </NexButton>
                  )}
                </header>

                <NexText>{item.message}</NexText>

                <dl className="notifications-fields">
                  <div>
                    <dt>{t("global.k_Notifications_Sender")}</dt>
                    <dd>{item.sender_user_id}</dd>
                  </div>
                  <div>
                    <dt>{t("global.k_Notifications_ShowFrom")}</dt>
                    <dd>{formatDateTime(item.show_from, locale)}</dd>
                  </div>
                  <div>
                    <dt>{t("global.k_Notifications_ShowUntil")}</dt>
                    <dd>{formatDateTime(item.show_until, locale)}</dd>
                  </div>
                  <div>
                    <dt>{t("global.k_Notifications_Duration")}</dt>
                    <dd>{t(`global.k_Notifications_Duration_${item.duration_code}`)}</dd>
                  </div>
                  {visibleMode === "all" && (
                    <div>
                      <dt>{t("global.k_Notifications_Audience")}</dt>
                      <dd>{item.audiences.map((audience) => audienceLabel(audience, t)).join(", ")}</dd>
                    </div>
                  )}
                  {item.edited_at && (
                    <div>
                      <dt>{t("global.k_Notifications_EditedAt")}</dt>
                      <dd>{formatDateTime(item.edited_at, locale)}</dd>
                    </div>
                  )}
                </dl>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
