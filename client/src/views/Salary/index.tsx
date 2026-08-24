import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { getMyCurrentCompensationRecord, listMyCompensationRecords } from "@/requests/salary";
import type { CompensationRecord } from "@/requests/salary/types";

import "./style.css";

function hasPermission(user: ReturnType<typeof useAuth>["user"], key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true
  );
}

export default function Salary() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const [current, setCurrent] = useState<CompensationRecord | null>(null);
  const [history, setHistory] = useState<CompensationRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const canManageEmployees =
    hasPermission(user, "salary.settlement.configure") || hasPermission(user, "salary.read");

  useDocumentTitle(t("global.k_Salary_PageTitle"));

  useEffect(() => {
    let mounted = true;
    async function load() {
      setLoading(true);
      setError(false);
      try {
        const [currentResult, historyResult] = await Promise.all([
          getMyCurrentCompensationRecord().catch(() => null),
          listMyCompensationRecords(),
        ]);
        if (!mounted) return;
        setCurrent(currentResult);
        setHistory(historyResult);
      } catch {
        if (mounted) setError(true);
      } finally {
        if (mounted) setLoading(false);
      }
    }
    void load();
    return () => {
      mounted = false;
    };
  }, []);

  return (
    <section className="salary-page">
      <header className="salary-page__header">
        <div>
          <NexText variant="title">{t("global.k_Salary_Title")}</NexText>
          <NexText color="muted">{t("global.k_Salary_Description")}</NexText>
        </div>
        {canManageEmployees ? (
          <Link className="salary-download" to="/salary/employees">
            {t("global.k_Salary_ManageLink")}
          </Link>
        ) : null}
      </header>

      {error ? (
        <div className="salary-alert salary-alert--error" role="alert">
          <NexText color="danger">{t("global.k_Salary_Error")}</NexText>
        </div>
      ) : null}
      {loading ? (
        <div className="salary-loading">
          <span className="salary-loading__dot" />
          <NexText color="muted">{t("global.k_Common_Loading")}</NexText>
        </div>
      ) : null}

      <section className="salary-section" aria-labelledby="salary-current-title">
        <NexText id="salary-current-title" variant="subheading">
          {t("global.k_Salary_Current")}
        </NexText>
        {current ? (
          <dl className="salary-metrics">
            <div>
              <dt>{t("global.k_Salary_Basis")}</dt>
              <dd>{t(`global.k_Salary_Basis_${current.compensation_basis}`)}</dd>
            </div>
            <div>
              <dt>{t("global.k_Salary_Rate")}</dt>
              <dd>{current.rate_amount} {current.currency}</dd>
            </div>
            <div>
              <dt>{t("global.k_Salary_Jurisdiction")}</dt>
              <dd>{current.jurisdiction_id}</dd>
            </div>
            <div>
              <dt>{t("global.k_Salary_EffectiveSince")}</dt>
              <dd>{current.effective_start_date}</dd>
            </div>
          </dl>
        ) : loading ? null : (
          <div className="salary-empty">
            <NexText color="muted">{t("global.k_Salary_NoCurrent")}</NexText>
          </div>
        )}
      </section>

      <section className="salary-section" aria-labelledby="salary-history-title">
        <div className="salary-section__header">
          <NexText id="salary-history-title" variant="subheading">
            {t("global.k_Salary_History")}
          </NexText>
          <span className="salary-count">{history.length}</span>
        </div>
        {history.length ? (
          <div className="salary-table-wrap">
            <table className="salary-table">
              <thead>
                <tr>
                  <th>{t("global.k_Salary_Basis")}</th>
                  <th>{t("global.k_Salary_Rate")}</th>
                  <th>{t("global.k_Salary_Jurisdiction")}</th>
                  <th>{t("global.k_Salary_EffectiveStart")}</th>
                  <th>{t("global.k_Salary_EffectiveEnd")}</th>
                  <th>{t("global.k_Salary_Status")}</th>
                </tr>
              </thead>
              <tbody>
                {history.map((record) => (
                  <tr key={record.id}>
                    <td data-label={t("global.k_Salary_Basis")}>
                      {t(`global.k_Salary_Basis_${record.compensation_basis}`)}
                    </td>
                    <td data-label={t("global.k_Salary_Rate")}>
                      {record.rate_amount} {record.currency}
                    </td>
                    <td data-label={t("global.k_Salary_Jurisdiction")}>{record.jurisdiction_id}</td>
                    <td data-label={t("global.k_Salary_EffectiveStart")}>{record.effective_start_date}</td>
                    <td data-label={t("global.k_Salary_EffectiveEnd")}>{record.effective_end_date ?? "-"}</td>
                    <td data-label={t("global.k_Salary_Status")}>
                      <span
                        className={`salary-badge salary-badge--${record.effective_end_date ? "closed" : "active"}`}
                      >
                        {record.effective_end_date
                          ? t("global.k_Salary_StatusClosed")
                          : t("global.k_Salary_StatusActive")}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : loading ? null : (
          <div className="salary-empty">
            <NexText color="muted">{t("global.k_Salary_NoHistory")}</NexText>
          </div>
        )}
      </section>
    </section>
  );
}
