import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  attendanceCSVURL,
  applyLeave,
  generateAttendanceReport,
  getAttendanceToday,
  getSelfAttendanceMonthlyReport,
  getLeaveTypes,
  listAttendanceDays,
  listLeaveRequests,
  signInAttendance,
  signOutAttendance,
} from "@/requests/attendance";
import type {
  AttendanceDay,
  AttendanceExport,
  AttendanceMonthlyReport,
  AttendanceSession,
  LeaveDurationType,
  LeaveRequest,
  LeaveType,
} from "@/requests/attendance/types";

import "./style.css";

function currentMonth() {
  return new Date().toLocaleDateString("en-CA").slice(0, 7);
}

function currentDate() {
  return new Date().toLocaleDateString("en-CA");
}

function hasPermission(user: ReturnType<typeof useAuth>["user"], key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some((role) =>
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

function formatHours(minutes: number, t: (key: string, options?: Record<string, unknown>) => string) {
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return t("global.k_Attendance_HoursMinutes", {
    hours,
    minutes: remainingMinutes,
  });
}

function sessionMinutes(session: AttendanceSession) {
  if (session.worked_minutes != null) return session.worked_minutes;
  const end = session.sign_out_at ? Date.parse(session.sign_out_at) : Date.now();
  return Math.max(0, Math.floor((end - Date.parse(session.sign_in_at)) / 60000));
}

export default function Attendance() {
  const { i18n, t } = useTranslation("ui");
  const { user } = useAuth();
  const locale = i18n.resolvedLanguage || navigator.language;
  const [today, setToday] = useState<AttendanceDay | null>(null);
  const [days, setDays] = useState<AttendanceDay[]>([]);
  const [month, setMonth] = useState(currentMonth());
  const [monthly, setMonthly] = useState<AttendanceMonthlyReport | null>(null);
  const [exportInfo, setExportInfo] = useState<AttendanceExport | null>(null);
  const [loading, setLoading] = useState(true);
  const [clockSaving, setClockSaving] = useState(false);
  const [leaveSaving, setLeaveSaving] = useState(false);
  const [reportSaving, setReportSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [leaveTypes, setLeaveTypes] = useState<LeaveType[]>([]);
  const [leaveRequests, setLeaveRequests] = useState<LeaveRequest[]>([]);
  const [leaveType, setLeaveType] = useState("");
  const [leaveDate, setLeaveDate] = useState(currentDate());
  const [leaveDuration, setLeaveDuration] = useState<LeaveDurationType>("full_day");
  const [leaveStart, setLeaveStart] = useState("09:00");
  const [leaveEnd, setLeaveEnd] = useState("10:00");
  const [leaveReason, setLeaveReason] = useState("");
  const canGenerateReports = hasPermission(user, "attendance.manage");
  const canDownloadReports = hasPermission(user, "attendance.reports.read");

  useDocumentTitle(t("global.k_Attendance_PageTitle"));

  const openSession = useMemo(
    () => today?.sessions.find((session) => !session.sign_out_at) ?? null,
    [today],
  );

  useEffect(() => {
    let mounted = true;
    async function load() {
      setLoading(true);
      setError(null);
      try {
        const [todayResult, daysResult, leaveTypeResult, leaveRequestResult] = await Promise.all([
          getAttendanceToday(),
          listAttendanceDays(month),
          getLeaveTypes(),
          listLeaveRequests(),
        ]);
        if (!mounted) return;
        setToday(todayResult);
        setDays(daysResult);
        setLeaveTypes(leaveTypeResult);
        setLeaveType((current) => current || leaveTypeResult[0]?.key || "");
        setLeaveRequests(leaveRequestResult);
        if (month < currentMonth()) {
          try {
            const report = await getSelfAttendanceMonthlyReport(month);
            if (mounted) setMonthly(report);
          } catch {
            if (mounted) setMonthly(null);
          }
        } else {
          setMonthly(null);
        }
      } catch {
        if (mounted) setError(t("global.k_Attendance_Error"));
      } finally {
        if (mounted) setLoading(false);
      }
    }
    void load();
    return () => {
      mounted = false;
    };
  }, [month, t]);

  async function clock() {
    setClockSaving(true);
    setError(null);
    setNotice(null);
    try {
      const nextToday =
        today?.status === "working"
          ? await signOutAttendance()
          : await signInAttendance();
      setToday(nextToday);
      setDays(await listAttendanceDays(month));
    } catch {
      setError(t("global.k_Attendance_ClockError"));
    } finally {
      setClockSaving(false);
    }
  }

  async function generateReport() {
    setReportSaving(true);
    setError(null);
    setNotice(null);
    try {
      const generated = await generateAttendanceReport(month);
      setExportInfo(generated);
      setNotice(t("global.k_Attendance_ReportGenerated"));
    } catch {
      setError(t("global.k_Attendance_ReportError"));
    } finally {
      setReportSaving(false);
    }
  }

  async function submitLeave(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLeaveSaving(true);
    setError(null);
    setNotice(null);
    try {
      await applyLeave({
        leave_type: leaveType,
        leave_date: leaveDate,
        duration_type: leaveDuration,
        start_time: leaveDuration === "hourly" ? leaveStart : undefined,
        end_time: leaveDuration === "hourly" ? leaveEnd : undefined,
        reason: leaveReason.trim(),
      });
      setLeaveRequests(await listLeaveRequests());
      setLeaveReason("");
      setNotice(t("global.k_Attendance_LeaveApplied"));
    } catch {
      setError(t("global.k_Attendance_LeaveError"));
    } finally {
      setLeaveSaving(false);
    }
  }

  return (
    <section className="attendance-page">
      <header className="attendance-page__header">
        <div>
          <NexText variant="title">{t("global.k_Attendance_Title")}</NexText>
          <NexText color="muted">{t("global.k_Attendance_Description")}</NexText>
        </div>
        <NexInput
          id="attendance-month"
          label={t("global.k_Attendance_Month")}
          type="month"
          value={month}
          onChange={(event) => setMonth(event.target.value || currentMonth())}
        />
      </header>

      {error ? <div className="attendance-alert attendance-alert--error" role="alert"><NexText color="danger">{error}</NexText></div> : null}
      {notice ? <div className="attendance-alert attendance-alert--success" role="status"><NexText color="primary">{notice}</NexText></div> : null}
      {loading ? <div className="attendance-loading"><span className="attendance-loading__dot" /><NexText color="muted">{t("global.k_Common_Loading")}</NexText></div> : null}

      {today ? (
        <section className="attendance-status" aria-labelledby="attendance-status-title">
          <div className="attendance-status__main">
            <NexText id="attendance-status-title" variant="subheading">
              {t("global.k_Attendance_Today")}
            </NexText>
            <span className={`attendance-badge attendance-badge--${today.status}`}>
              {today.status === "working"
                ? t("global.k_Attendance_Status_Working")
                : t("global.k_Attendance_Status_NonWorking")}
            </span>
          </div>
          <dl className="attendance-metrics">
            <div>
              <dt>{t("global.k_Attendance_Date")}</dt>
              <dd>{today.attendance_date}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_Timezone")}</dt>
              <dd>{today.timezone}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_WorkedTime")}</dt>
              <dd>{formatHours(today.worked_minutes, t)}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_CurrentSession")}</dt>
              <dd>{openSession ? formatDateTime(openSession.sign_in_at, locale) : "-"}</dd>
            </div>
          </dl>
          <NexButton
            type="button"
            variant={today.status === "working" ? "danger" : "primary"}
            className="attendance-status__clock"
            disabled={clockSaving}
            onClick={() => void clock()}
          >
            {clockSaving
              ? t("global.k_Common_Saving")
              : today.status === "working"
                ? t("global.k_Attendance_SignOut")
                : t("global.k_Attendance_SignIn")}
          </NexButton>
        </section>
      ) : null}

      {today?.sessions.length ? (
        <section className="attendance-section" aria-labelledby="attendance-session-title">
          <NexText id="attendance-session-title" variant="subheading">
            {t("global.k_Attendance_TodaySessions")}
          </NexText>
          <div className="attendance-table-wrap">
            <table className="attendance-table">
              <thead>
                <tr>
                  <th>{t("global.k_Attendance_Session")}</th>
                  <th>{t("global.k_Attendance_SignInAt")}</th>
                  <th>{t("global.k_Attendance_SignOutAt")}</th>
                  <th>{t("global.k_Attendance_Duration")}</th>
                </tr>
              </thead>
              <tbody>
                {today.sessions.map((session) => (
                  <tr key={session.id}>
                    <td data-label={t("global.k_Attendance_Session")}>
                      {session.sequence_number}
                    </td>
                    <td data-label={t("global.k_Attendance_SignInAt")}>
                      {formatDateTime(session.sign_in_at, locale)}
                    </td>
                    <td data-label={t("global.k_Attendance_SignOutAt")}>
                      {formatDateTime(session.sign_out_at, locale)}
                    </td>
                    <td data-label={t("global.k_Attendance_Duration")}>
                      {formatHours(sessionMinutes(session), t)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      ) : null}

      <section className="attendance-section" aria-labelledby="attendance-leave-title">
        <div className="attendance-section__intro">
          <NexText id="attendance-leave-title" variant="subheading">
            {t("global.k_Attendance_ApplyLeave")}
          </NexText>
          <NexText color="muted">{t("global.k_Attendance_LeaveDescription")}</NexText>
        </div>
        <form className="attendance-leave-form" onSubmit={(event) => void submitLeave(event)}>
          <label className="attendance-select-field attendance-leave-form__type">
            <NexText as="span" variant="label">{t("global.k_Attendance_LeaveType")}</NexText>
            <NexSelect
              ariaLabel={t("global.k_Attendance_LeaveType")}
              value={leaveType}
              options={leaveTypes.map((type) => ({ value: type.key, label: type.label }))}
              onChange={setLeaveType}
            />
          </label>
          <NexInput
            className="attendance-leave-form__date"
            id="attendance-leave-date"
            label={t("global.k_Attendance_LeaveDate")}
            type="date"
            required
            value={leaveDate}
            onChange={(event) => setLeaveDate(event.target.value)}
          />
          <label className="attendance-select-field attendance-leave-form__duration">
            <NexText as="span" variant="label">{t("global.k_Attendance_LeaveDurationType")}</NexText>
            <NexSelect
              ariaLabel={t("global.k_Attendance_LeaveDurationType")}
              value={leaveDuration}
              options={[
                { value: "full_day", label: t("global.k_Attendance_LeaveFullDay") },
                { value: "hourly", label: t("global.k_Attendance_LeaveHourly") },
              ]}
              onChange={setLeaveDuration}
            />
          </label>
          {leaveDuration === "hourly" ? (
            <>
              <NexInput className="attendance-leave-form__start" id="attendance-leave-start" label={t("global.k_Attendance_LeaveStart")}
                type="time" required value={leaveStart}
                onChange={(event) => setLeaveStart(event.target.value)} />
              <NexInput className="attendance-leave-form__end" id="attendance-leave-end" label={t("global.k_Attendance_LeaveEnd")}
                type="time" required value={leaveEnd}
                onChange={(event) => setLeaveEnd(event.target.value)} />
            </>
          ) : (
            <div className="attendance-leave-form__hint">
              <span className="attendance-leave-form__hint-icon" aria-hidden="true">8h</span>
              <NexText color="muted">
              {t("global.k_Attendance_LeaveFullDayHint")}
              </NexText>
            </div>
          )}
          <NexInput className="attendance-leave-form__reason" id="attendance-leave-reason" label={t("global.k_Attendance_LeaveReason")}
            value={leaveReason} onChange={(event) => setLeaveReason(event.target.value)} />
          <div className="attendance-leave-form__actions">
            <NexText color="muted" variant="caption">{t("global.k_Attendance_LeaveMinimumHint")}</NexText>
            <NexButton type="submit" disabled={leaveSaving || !leaveType}>
              {leaveSaving ? t("global.k_Common_Saving") : t("global.k_Attendance_LeaveSubmit")}
            </NexButton>
          </div>
        </form>
        <div className="attendance-section__subheader">
          <NexText variant="label">{t("global.k_Attendance_LeaveHistory")}</NexText>
          <span className="attendance-count">{leaveRequests.length}</span>
        </div>
        {leaveRequests.length ? (
          <div className="attendance-table-wrap">
            <table className="attendance-table">
              <thead><tr>
                <th>{t("global.k_Attendance_Date")}</th>
                <th>{t("global.k_Attendance_LeaveType")}</th>
                <th>{t("global.k_Attendance_Duration")}</th>
                <th>{t("global.k_Attendance_Status")}</th>
              </tr></thead>
              <tbody>{leaveRequests.map((request) => (
                <tr key={request.id}>
                  <td data-label={t("global.k_Attendance_Date")}>{request.leave_date}</td>
                  <td data-label={t("global.k_Attendance_LeaveType")}>
                    {leaveTypes.find((type) => type.key === request.leave_type)?.label ?? request.leave_type}
                  </td>
                  <td data-label={t("global.k_Attendance_Duration")}>
                    {request.duration_type === "full_day"
                      ? t("global.k_Attendance_LeaveFullDay")
                      : `${request.start_time}–${request.end_time} (${formatHours(request.requested_minutes, t)})`}
                  </td>
                  <td data-label={t("global.k_Attendance_Status")}>
                    <span className={`attendance-badge attendance-badge--leave-${request.status}`}>
                      {t(`global.k_Attendance_LeaveStatus_${request.status}`)}
                    </span>
                  </td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        ) : <div className="attendance-empty"><NexText color="muted">{t("global.k_Attendance_LeaveNoRequests")}</NexText></div>}
      </section>

      {monthly ? (
        <section className="attendance-section" aria-labelledby="attendance-monthly-title">
          <NexText id="attendance-monthly-title" variant="subheading">
            {t("global.k_Attendance_MonthlySummary")}
          </NexText>
          <dl className="attendance-metrics">
            <div>
              <dt>{t("global.k_Attendance_PresentDays")}</dt>
              <dd>{monthly.present_days}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_AbsentDays")}</dt>
              <dd>{monthly.absent_days}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_IncompleteDays")}</dt>
              <dd>{monthly.incomplete_days}</dd>
            </div>
            <div>
              <dt>{t("global.k_Attendance_WorkedTime")}</dt>
              <dd>{formatHours(monthly.worked_minutes, t)}</dd>
            </div>
          </dl>
        </section>
      ) : null}

      <section className="attendance-section" aria-labelledby="attendance-days-title">
        <div className="attendance-section__header">
          <NexText id="attendance-days-title" variant="subheading">
            {t("global.k_Attendance_DayRecords")}
          </NexText>
          <div className="attendance-actions">
            {canGenerateReports ? (
              <NexButton
                type="button"
                variant="secondary"
                size="compact"
                disabled={reportSaving}
                onClick={() => void generateReport()}
              >
                {reportSaving ? t("global.k_Common_Saving") : t("global.k_Attendance_GenerateCSV")}
              </NexButton>
            ) : null}
            {canDownloadReports ? (
              <a className="attendance-download" href={attendanceCSVURL(month)}>
                {t("global.k_Attendance_DownloadCSV")}
              </a>
            ) : null}
          </div>
        </div>
        {exportInfo ? (
          <NexText color="muted">
            {t("global.k_Attendance_ReportRows", { count: exportInfo.row_count })}
          </NexText>
        ) : null}
        {days.length ? (
          <div className="attendance-table-wrap">
            <table className="attendance-table">
              <thead>
                <tr>
                  <th>{t("global.k_Attendance_Date")}</th>
                  <th>{t("global.k_Attendance_Status")}</th>
                  <th>{t("global.k_Attendance_WorkedTime")}</th>
                  <th>{t("global.k_Attendance_Sessions")}</th>
                  <th>{t("global.k_Attendance_Review")}</th>
                </tr>
              </thead>
              <tbody>
                {days.map((day) => (
                  <tr key={day.id}>
                    <td data-label={t("global.k_Attendance_Date")}>
                      {day.attendance_date}
                    </td>
                    <td data-label={t("global.k_Attendance_Status")}>
                      {day.status === "working"
                        ? t("global.k_Attendance_Status_Working")
                        : t("global.k_Attendance_Status_NonWorking")}
                    </td>
                    <td data-label={t("global.k_Attendance_WorkedTime")}>
                      {formatHours(day.worked_minutes, t)}
                    </td>
                    <td data-label={t("global.k_Attendance_Sessions")}>
                      {day.sessions.length}
                    </td>
                    <td data-label={t("global.k_Attendance_Review")}>
                      {day.requires_review
                        ? t("global.k_Dashboard_Value_True")
                        : t("global.k_Dashboard_Value_False")}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : loading ? null : (
          <NexText color="muted">{t("global.k_Attendance_NoRecords")}</NexText>
        )}
      </section>
    </section>
  );
}
