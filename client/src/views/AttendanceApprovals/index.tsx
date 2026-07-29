import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { decideLeave, getLeaveTypes, listLeaveApprovals } from "@/requests/attendance";
import type { LeaveApprovalRequest, LeaveType } from "@/requests/attendance/types";

import "../Attendance/style.css";
import "./style.css";

type ApprovalFilter = "pending" | "all";

function duration(request: LeaveApprovalRequest, t: (key: string, options?: Record<string, unknown>) => string) {
  if (request.duration_type === "full_day") return t("global.k_Attendance_LeaveFullDay");
  return `${request.start_time}–${request.end_time} (${Math.floor(request.requested_minutes / 60)}h ${request.requested_minutes % 60}m)`;
}

export default function AttendanceApprovals() {
  const { t } = useTranslation("ui");
  const [requests, setRequests] = useState<LeaveApprovalRequest[]>([]);
  const [leaveTypes, setLeaveTypes] = useState<LeaveType[]>([]);
  const [notes, setNotes] = useState<Record<string, string>>({});
  const [filter, setFilter] = useState<ApprovalFilter>("pending");
  const [loading, setLoading] = useState(true);
  const [savingID, setSavingID] = useState<string | null>(null);
  const [error, setError] = useState(false);
  useDocumentTitle(t("global.k_Attendance_ApprovalsTitle"));

  useEffect(() => {
    let active = true;
    Promise.all([listLeaveApprovals(), getLeaveTypes()])
      .then(([approvalResult, typeResult]) => {
        if (!active) return;
        setRequests(approvalResult);
        setLeaveTypes(typeResult);
      })
      .catch(() => { if (active) setError(true); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const visible = useMemo(
    () => filter === "pending" ? requests.filter((request) => request.status === "pending") : requests,
    [filter, requests],
  );
  const pendingCount = requests.filter((request) => request.status === "pending").length;

  async function decide(request: LeaveApprovalRequest, decision: "approved" | "rejected") {
    setSavingID(request.id);
    setError(false);
    try {
      const updated = await decideLeave(request.id, { decision, note: notes[request.id]?.trim() || "" });
      setRequests((items) => items.map((item) => item.id === updated.id ? updated : item));
      setNotes((current) => ({ ...current, [request.id]: "" }));
    } catch {
      setError(true);
    } finally {
      setSavingID(null);
    }
  }

  return (
    <section className="attendance-page approval-page">
      <header className="attendance-page__header">
        <div>
          <NexText variant="title">{t("global.k_Attendance_ApprovalsTitle")}</NexText>
          <NexText color="muted">{t("global.k_Attendance_ApprovalsDescription")}</NexText>
        </div>
        <Link className="attendance-download" to="/attendance">{t("global.k_Attendance_Back")}</Link>
      </header>
      <div className="approval-toolbar">
        <div className="attendance-section__subheader">
          <NexText variant="label">{t("global.k_Attendance_ApprovalsPending")}</NexText>
          <span className="attendance-count">{pendingCount}</span>
        </div>
        <NexSelect
          ariaLabel={t("global.k_Attendance_ApprovalsFilter")}
          value={filter}
          options={[
            { value: "pending", label: t("global.k_Attendance_ApprovalsPendingOnly") },
            { value: "all", label: t("global.k_Attendance_ApprovalsAll") },
          ]}
          onChange={setFilter}
        />
      </div>
      {error ? <div className="attendance-alert attendance-alert--error" role="alert"><NexText color="danger">{t("global.k_Attendance_ApprovalsError")}</NexText></div> : null}
      {loading ? <div className="attendance-loading"><span className="attendance-loading__dot" /><NexText color="muted">{t("global.k_Common_Loading")}</NexText></div> : null}
      {!loading && !visible.length ? <div className="attendance-empty"><NexText color="muted">{t("global.k_Attendance_ApprovalsEmpty")}</NexText></div> : null}
      <div className="approval-list">
        {visible.map((request) => (
          <article className="approval-card" key={request.id}>
            <div className="approval-card__header">
              <div>
                <NexText variant="subheading">{request.requester_name}</NexText>
                <NexText color="muted">{request.leave_date}</NexText>
              </div>
              <span className={`attendance-badge attendance-badge--leave-${request.status}`}>
                {t(`global.k_Attendance_LeaveStatus_${request.status}`)}
              </span>
            </div>
            <dl className="approval-card__details">
              <div><dt>{t("global.k_Attendance_LeaveType")}</dt><dd>{leaveTypes.find((type) => type.key === request.leave_type)?.label ?? request.leave_type}</dd></div>
              <div><dt>{t("global.k_Attendance_Duration")}</dt><dd>{duration(request, t)}</dd></div>
              <div><dt>{t("global.k_Attendance_LeaveReason")}</dt><dd>{request.reason || "-"}</dd></div>
              <div><dt>{t("global.k_Attendance_ApprovalRoute")}</dt><dd>{request.administrator_route ? t("global.k_Attendance_ApprovalAdministrator") : t("global.k_Attendance_ApprovalManager")}</dd></div>
            </dl>
            {request.status === "pending" ? (
              <div className="approval-card__decision">
                <NexInput
                  id={`approval-note-${request.id}`}
                  label={t("global.k_Attendance_ApprovalNote")}
                  value={notes[request.id] || ""}
                  onChange={(event) => setNotes((current) => ({ ...current, [request.id]: event.target.value }))}
                />
                <div className="approval-card__actions">
                  <NexButton type="button" variant="secondary" disabled={savingID === request.id} onClick={() => void decide(request, "rejected")}>
                    {t("global.k_Attendance_Reject")}
                  </NexButton>
                  <NexButton type="button" disabled={savingID === request.id} onClick={() => void decide(request, "approved")}>
                    {savingID === request.id ? t("global.k_Common_Saving") : t("global.k_Attendance_Approve")}
                  </NexButton>
                </div>
              </div>
            ) : null}
          </article>
        ))}
      </div>
    </section>
  );
}
