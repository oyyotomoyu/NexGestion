import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { listUsers } from "@/requests/users";
import type { User } from "@/requests/users/types";
import {
  createCompensationRecord,
  getEmployeeCurrentCompensationRecord,
  listEmployeeCompensationRecords,
} from "@/requests/salary";
import {
  compensationBasisOptions,
  type CompensationBasis,
  type CompensationRecord,
} from "@/requests/salary/types";

import "../Salary/style.css";
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

function currentDate() {
  return new Date().toLocaleDateString("en-CA");
}

export default function SalaryEmployees() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canConfigure = hasPermission(user, "salary.settlement.configure");

  const [employees, setEmployees] = useState<User[]>([]);
  const [query, setQuery] = useState("");
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null);
  const [current, setCurrent] = useState<CompensationRecord | null>(null);
  const [history, setHistory] = useState<CompensationRecord[]>([]);
  const [loadingEmployees, setLoadingEmployees] = useState(true);
  const [loadingRecords, setLoadingRecords] = useState(false);
  const [employeesError, setEmployeesError] = useState(false);
  const [recordsError, setRecordsError] = useState(false);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const [basis, setBasis] = useState<CompensationBasis>("monthly");
  const [rateAmount, setRateAmount] = useState("");
  const [currency, setCurrency] = useState("");
  const [jurisdictionId, setJurisdictionId] = useState("");
  const [startDate, setStartDate] = useState(currentDate());
  const [note, setNote] = useState("");

  useDocumentTitle(t("global.k_Salary_EmployeesTitle"));

  useEffect(() => {
    listUsers()
      .then(setEmployees)
      .catch(() => setEmployeesError(true))
      .finally(() => setLoadingEmployees(false));
  }, []);

  const visibleEmployees = useMemo(
    () =>
      employees.filter((employee) =>
        `${employee.id} ${employee.display_name} ${employee.email}`
          .toLowerCase()
          .includes(query.trim().toLowerCase()),
      ),
    [employees, query],
  );
  const selectedEmployee = employees.find((employee) => employee.id === selectedUserId) ?? null;

  async function loadRecords(userId: string) {
    setLoadingRecords(true);
    setRecordsError(false);
    try {
      const [currentResult, historyResult] = await Promise.all([
        getEmployeeCurrentCompensationRecord(userId).catch(() => null),
        listEmployeeCompensationRecords(userId),
      ]);
      setCurrent(currentResult);
      setHistory(historyResult);
    } catch {
      setRecordsError(true);
    } finally {
      setLoadingRecords(false);
    }
  }

  function selectEmployee(userId: string) {
    setSelectedUserId(userId);
    setNotice(null);
    setFormError(null);
    void loadRecords(userId);
  }

  const rateValid = /^\d+(\.\d{1,6})?$/.test(rateAmount) && rateAmount !== "0";
  const currencyValid = /^[A-Za-z]{3}$/.test(currency.trim());
  const canSubmit = rateValid && currencyValid && jurisdictionId.trim() !== "" && startDate !== "";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedUserId || !canSubmit) return;
    setSaving(true);
    setFormError(null);
    setNotice(null);
    try {
      await createCompensationRecord(selectedUserId, {
        compensation_basis: basis,
        rate_amount: rateAmount,
        currency: currency.trim().toUpperCase(),
        jurisdiction_id: jurisdictionId.trim(),
        effective_start_date: startDate,
        note: note.trim() || undefined,
      });
      await loadRecords(selectedUserId);
      setRateAmount("");
      setNote("");
      setNotice(t("global.k_Salary_AssignSuccess"));
    } catch {
      setFormError(t("global.k_Salary_AssignError"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="salary-page">
      <header className="salary-page__header">
        <div>
          <NexText variant="title">{t("global.k_Salary_EmployeesTitle")}</NexText>
          <NexText color="muted">{t("global.k_Salary_EmployeesDescription")}</NexText>
        </div>
        <Link className="salary-download" to="/salary">
          {t("global.k_Salary_BackToMine")}
        </Link>
      </header>

      <section className="salary-section" aria-labelledby="salary-employees-title">
        <NexText id="salary-employees-title" variant="subheading">
          {t("global.k_Salary_SelectEmployee")}
        </NexText>
        <NexInput
          id="salary-employee-search"
          type="search"
          label={t("global.k_Salary_SearchEmployees")}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        {employeesError ? <NexText color="danger">{t("global.k_Salary_EmployeesError")}</NexText> : null}
        {loadingEmployees ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
        {!loadingEmployees && visibleEmployees.length ? (
          <div className="salary-employee-list">
            {visibleEmployees.map((employee) => (
              <button
                type="button"
                key={employee.id}
                className={`salary-employee-row${employee.id === selectedUserId ? " is-selected" : ""}`}
                onClick={() => selectEmployee(employee.id)}
              >
                <NexText as="span" weight={600}>
                  {employee.display_name}
                </NexText>
                <NexText as="span" color="muted">
                  {employee.email}
                </NexText>
              </button>
            ))}
          </div>
        ) : null}
        {!loadingEmployees && !visibleEmployees.length ? (
          <div className="salary-empty">
            <NexText color="muted">{t("global.k_Salary_NoEmployees")}</NexText>
          </div>
        ) : null}
      </section>

      {selectedEmployee ? (
        <>
          <section className="salary-section" aria-labelledby="salary-selected-current-title">
            <NexText id="salary-selected-current-title" variant="subheading">
              {t("global.k_Salary_CurrentFor", { name: selectedEmployee.display_name })}
            </NexText>
            {recordsError ? <NexText color="danger">{t("global.k_Salary_RecordsError")}</NexText> : null}
            {loadingRecords ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
            {!loadingRecords && current ? (
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
            ) : null}
            {!loadingRecords && !current ? (
              <div className="salary-empty">
                <NexText color="muted">{t("global.k_Salary_NoCurrent")}</NexText>
              </div>
            ) : null}
          </section>

          {canConfigure ? (
            <section className="salary-section" aria-labelledby="salary-assign-title">
              <div className="salary-section__intro">
                <NexText id="salary-assign-title" variant="subheading">
                  {t("global.k_Salary_AssignTitle")}
                </NexText>
                <NexText color="muted">{t("global.k_Salary_AssignDescription")}</NexText>
              </div>
              {notice ? (
                <div className="salary-alert salary-alert--success" role="status">
                  <NexText color="primary">{notice}</NexText>
                </div>
              ) : null}
              {formError ? (
                <div className="salary-alert salary-alert--error" role="alert">
                  <NexText color="danger">{formError}</NexText>
                </div>
              ) : null}
              <form className="salary-assign-form" onSubmit={(event) => void submit(event)}>
                <label className="salary-select-field salary-assign-form__basis">
                  <NexText as="span" variant="label">
                    {t("global.k_Salary_Basis")}
                  </NexText>
                  <NexSelect
                    ariaLabel={t("global.k_Salary_Basis")}
                    value={basis}
                    options={compensationBasisOptions.map((value) => ({
                      value,
                      label: t(`global.k_Salary_Basis_${value}`),
                    }))}
                    onChange={setBasis}
                  />
                </label>
                <NexInput
                  className="salary-assign-form__rate"
                  id="salary-assign-rate"
                  label={t("global.k_Salary_Rate")}
                  inputMode="decimal"
                  required
                  value={rateAmount}
                  onChange={(event) => setRateAmount(event.target.value)}
                />
                <NexInput
                  className="salary-assign-form__currency"
                  id="salary-assign-currency"
                  label={t("global.k_Salary_Currency")}
                  maxLength={3}
                  required
                  value={currency}
                  onChange={(event) => setCurrency(event.target.value.toUpperCase())}
                />
                <NexInput
                  className="salary-assign-form__jurisdiction"
                  id="salary-assign-jurisdiction"
                  label={t("global.k_Salary_Jurisdiction")}
                  required
                  value={jurisdictionId}
                  onChange={(event) => setJurisdictionId(event.target.value)}
                />
                <NexInput
                  className="salary-assign-form__start"
                  id="salary-assign-start"
                  type="date"
                  label={t("global.k_Salary_EffectiveStart")}
                  required
                  value={startDate}
                  onChange={(event) => setStartDate(event.target.value)}
                />
                <NexInput
                  className="salary-assign-form__note"
                  id="salary-assign-note"
                  label={t("global.k_Salary_Note")}
                  value={note}
                  onChange={(event) => setNote(event.target.value)}
                />
                <div className="salary-assign-form__actions">
                  <NexText color="muted" variant="caption">
                    {t("global.k_Salary_AssignHint")}
                  </NexText>
                  <NexButton type="submit" disabled={saving || !canSubmit}>
                    {saving ? t("global.k_Common_Saving") : t("global.k_Salary_Assign")}
                  </NexButton>
                </div>
              </form>
            </section>
          ) : null}

          <section className="salary-section" aria-labelledby="salary-selected-history-title">
            <div className="salary-section__header">
              <NexText id="salary-selected-history-title" variant="subheading">
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
            ) : loadingRecords ? null : (
              <div className="salary-empty">
                <NexText color="muted">{t("global.k_Salary_NoHistory")}</NexText>
              </div>
            )}
          </section>
        </>
      ) : null}
    </section>
  );
}
