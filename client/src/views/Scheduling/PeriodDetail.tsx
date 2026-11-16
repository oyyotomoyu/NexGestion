import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  assignUser,
  cancelAssignment,
  cancelShift,
  createShift,
  getPeriod,
  getPeriodConflicts,
  getPeriodEvents,
  getShiftCandidates,
  transitionPeriod,
  updatePeriod,
  type AssignmentMode,
  type PeriodAction,
  type ScheduleCandidate,
  type SchedulePeriod,
  type ScheduleShift,
  type SchedulingConflict,
  type SchedulingEvent,
} from "@/requests/scheduling";

import {
  addDays,
  ConflictList,
  conflictsFromError,
  errorLabelKey,
  useFormatMinutes,
  formatShiftTime,
  OverridePanel,
  StatusBadge,
  usePermissions,
  useSchedulingLabels,
} from "./shared";
import "./style.css";

type Action = (reason?: string) => Promise<unknown>;
interface PendingOverride {
  conflicts: SchedulingConflict[];
  retry: (reason: string) => void;
}

const modes: AssignmentMode[] = ["named_users", "open_shift", "requirement_only"];
const actionsByStatus: Record<string, PeriodAction[]> = {
  draft: ["publish", "cancel"],
  published: ["lock", "cancel"],
  locked: ["archive"],
};

export default function SchedulePeriodDetail() {
  const { periodId = "" } = useParams();
  const label = useSchedulingLabels();
  const has = usePermissions();
  const [period, setPeriod] = useState<SchedulePeriod | null>(null);
  const [conflicts, setConflicts] = useState<SchedulingConflict[]>([]);
  const [events, setEvents] = useState<SchedulingEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState<PendingOverride | null>(null);
  const [nameDraft, setNameDraft] = useState("");
  useDocumentTitle(period?.name || label("Periods_Title"));

  const editable = period?.status === "draft" || period?.status === "published";
  const canManage = has("scheduling.manage") && editable;
  const canPublish = has("scheduling.publish");
  const canOverride = has("scheduling.override");

  const reload = useCallback(async () => {
    const [detail, conflictResponse, eventResponse] = await Promise.all([
      getPeriod(periodId),
      getPeriodConflicts(periodId),
      getPeriodEvents(periodId),
    ]);
    setPeriod(detail);
    setNameDraft(detail.name);
    setConflicts(conflictResponse.conflicts);
    setEvents(eventResponse.events);
  }, [periodId]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    reload()
      .catch((cause) => { if (active) setError(label(errorLabelKey(cause))); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [reload]);

  async function run(action: Action, successKey: string, reason?: string) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action(reason);
      setPending(null);
      setMessage(label(successKey));
      await reload();
    } catch (cause) {
      const blocking = conflictsFromError(cause);
      if (blocking) {
        setPending({ conflicts: blocking, retry: (overrideReason) => void run(action, successKey, overrideReason) });
      } else {
        setError(label(errorLabelKey(cause)));
      }
    } finally {
      setBusy(false);
    }
  }

  const shifts = period?.shifts ?? [];
  const describeShift = useCallback((shiftId: string) => {
    const shift = shifts.find((item) => item.id === shiftId);
    return shift ? `${shift.local_start_at.slice(0, 10)} ${formatShiftTime(shift)}` : label("OtherSchedule");
  }, [shifts, label]);
  const names = useMemo(() => {
    const map = new Map<string, string>();
    shifts.forEach((shift) => shift.assignments.forEach((assignment) => {
      if (assignment.display_name) map.set(assignment.user_id, assignment.display_name);
    }));
    return map;
  }, [shifts]);
  const describeUser = useCallback((userId: string) => names.get(userId) || userId, [names]);

  const shiftsByDate = useMemo(() => {
    const groups = new Map<string, ScheduleShift[]>();
    shifts.forEach((shift) => {
      const date = shift.local_start_at.slice(0, 10);
      groups.set(date, [...(groups.get(date) ?? []), shift]);
    });
    return [...groups.entries()];
  }, [shifts]);

  if (loading) {
    return <section className="scheduling-page"><NexText role="status">{label("Loading")}</NexText></section>;
  }
  if (!period) {
    return (
      <section className="scheduling-page">
        <Link className="scheduling-link" to="/scheduling/periods"><NexText as="span" color="inherit">{label("BackToSchedules")}</NexText></Link>
        <div role="alert"><NexText color="danger">{error || label("Error_NotFound")}</NexText></div>
      </section>
    );
  }

  const blockingCount = conflicts.filter((conflict) => conflict.severity === "error").length;

  return (
    <section className="scheduling-page">
      <Link className="scheduling-link" to="/scheduling/periods"><NexText as="span" color="inherit">{label("BackToSchedules")}</NexText></Link>
      <header className="scheduling-header">
        <div className="scheduling-header__text">
          <NexText variant="title">{period.name}</NexText>
          <NexText color="muted">
            {period.group_name} · {period.start_date} – {period.end_date} · {period.timezone}
          </NexText>
        </div>
        <StatusBadge prefix="Status" status={period.status} />
      </header>

      {canPublish && (actionsByStatus[period.status]?.length ?? 0) > 0 && (
        <div className="scheduling-actions">
          {actionsByStatus[period.status].map((action) => (
            <NexButton
              key={action}
              variant={action === "cancel" ? "danger" : action === "publish" ? "primary" : "secondary"}
              disabled={busy}
              onClick={() => {
                if (action === "cancel" && !window.confirm(label("Confirm_CancelPeriod"))) return;
                void run((reason) => transitionPeriod(period.id, action, reason), `Done_${action}`);
              }}
            >
              {label(`Action_${action}`)}
            </NexButton>
          ))}
        </div>
      )}

      {error && <div role="alert"><NexText color="danger">{error}</NexText></div>}
      {message && <div role="status"><NexText color="success">{message}</NexText></div>}
      {pending && (
        <OverridePanel
          conflicts={pending.conflicts}
          canOverride={canOverride}
          busy={busy}
          onOverride={pending.retry}
          onDismiss={() => setPending(null)}
          describeShift={describeShift}
          describeUser={describeUser}
        />
      )}

      {canManage && (
        <form
          className="scheduling-panel scheduling-form"
          onSubmit={(event) => {
            event.preventDefault();
            void run(() => updatePeriod(period.id, { name: nameDraft.trim() }), "Done_Saved");
          }}
        >
          <NexInput label={label("Name")} required value={nameDraft} onChange={(event) => setNameDraft(event.target.value)} />
          <NexButton variant="secondary" disabled={busy || !nameDraft.trim() || nameDraft.trim() === period.name}>{label("Save")}</NexButton>
        </form>
      )}

      <section className="scheduling-panel">
        <div className="scheduling-section-header">
          <NexText variant="subheading">{label("Conflicts_Title")}</NexText>
          {blockingCount > 0 && (
            <NexText as="span" variant="caption" color="danger">{label("Conflicts_BlockingCount", { count: blockingCount })}</NexText>
          )}
        </div>
        <ConflictList conflicts={conflicts} describeShift={describeShift} describeUser={describeUser} />
      </section>

      {canManage && <ShiftForm period={period} busy={busy} onSubmit={(input) => run(() => createShift(period.id, input), "Done_ShiftCreated")} />}

      <section className="scheduling-panel">
        <NexText variant="subheading">{label("Shifts")}</NexText>
        {shiftsByDate.length === 0 && <NexText color="muted">{label("Shifts_Empty")}</NexText>}
        {shiftsByDate.map(([date, dayShifts]) => (
          <div key={date} className="scheduling-day">
            <NexText as="h3" variant="label">{date}</NexText>
            {dayShifts.map((shift) => (
              <ShiftCard
                key={shift.id}
                shift={shift}
                canManage={canManage}
                busy={busy}
                onAssign={(userId) => run((reason) => assignUser(shift.id, userId, reason), "Done_Assigned")}
                onRemove={(assignmentId) => run(() => cancelAssignment(assignmentId), "Done_Unassigned")}
                onCancel={() => {
                  if (!window.confirm(label("Confirm_CancelShift"))) return;
                  void run(() => cancelShift(shift.id), "Done_ShiftCancelled");
                }}
              />
            ))}
          </div>
        ))}
      </section>

      <details className="scheduling-panel scheduling-history">
        <summary><NexText as="span" variant="subheading">{label("History_Title", { count: events.length })}</NexText></summary>
        <ul className="scheduling-events">
          {[...events].reverse().map((event) => (
            <li key={event.id}>
              <NexText as="span" variant="caption" color="muted">{new Date(event.occurred_at).toLocaleString()}</NexText>
              <NexText as="span">{label(`Event_${event.event_type}`)}</NexText>
              {event.shift_id && <NexText as="span" variant="caption" color="muted">{describeShift(event.shift_id)}</NexText>}
              {event.reason && <NexText as="span" variant="caption">{label("History_Reason", { reason: event.reason })}</NexText>}
            </li>
          ))}
        </ul>
      </details>
    </section>
  );
}

interface ShiftFormProps {
  period: SchedulePeriod;
  busy: boolean;
  onSubmit: (input: Parameters<typeof createShift>[1]) => Promise<void>;
}

function ShiftForm({ period, busy, onSubmit }: ShiftFormProps) {
  const label = useSchedulingLabels();
  const [date, setDate] = useState(period.start_date);
  const [start, setStart] = useState("09:00");
  const [end, setEnd] = useState("17:00");
  const [title, setTitle] = useState("");
  const [headcount, setHeadcount] = useState("1");
  const [mode, setMode] = useState<AssignmentMode>("named_users");
  // An end time at or before the start time means the shift ends the next day.
  const overnight = end <= start;

  async function submit(event: FormEvent) {
    event.preventDefault();
    await onSubmit({
      title: title.trim() || undefined,
      local_start_at: `${date}T${start}`,
      local_end_at: `${overnight ? addDays(date, 1) : date}T${end}`,
      required_headcount: Math.max(1, Number(headcount) || 1),
      assignment_mode: mode,
    });
    setTitle("");
  }

  return (
    <section className="scheduling-panel">
      <NexText variant="subheading">{label("Shifts_Add")}</NexText>
      <form className="scheduling-form scheduling-form--grid" onSubmit={submit}>
        <NexInput label={label("Date")} type="date" required min={period.start_date} max={period.end_date} value={date} onChange={(event) => setDate(event.target.value)} />
        <NexInput label={label("StartTime")} type="time" required value={start} onChange={(event) => setStart(event.target.value)} />
        <NexInput label={label("EndTime")} type="time" required value={end} onChange={(event) => setEnd(event.target.value)} />
        <NexInput label={label("ShiftTitle")} value={title} placeholder={label("ShiftTitlePlaceholder")} onChange={(event) => setTitle(event.target.value)} />
        <NexInput label={label("Headcount")} type="number" min="1" max="1000" required value={headcount} onChange={(event) => setHeadcount(event.target.value)} />
        <div className="scheduling-field">
          <NexText as="span" variant="label">{label("Mode")}</NexText>
          <NexSelect ariaLabel={label("Mode")} value={mode} onChange={setMode} options={modes.map((value) => ({ value, label: label(`Mode_${value}`) }))} />
        </div>
        <div className="scheduling-actions scheduling-form__submit">
          <NexButton disabled={busy || !date}>{label("Shifts_AddSubmit")}</NexButton>
          {overnight && <NexText as="span" variant="caption" color="muted">{label("Shifts_Overnight")}</NexText>}
        </div>
      </form>
    </section>
  );
}

interface ShiftCardProps {
  shift: ScheduleShift;
  canManage: boolean;
  busy: boolean;
  onAssign: (userId: string) => Promise<void>;
  onRemove: (assignmentId: string) => Promise<void>;
  onCancel: () => void;
}

function ShiftCard({ shift, canManage, busy, onAssign, onRemove, onCancel }: ShiftCardProps) {
  const label = useSchedulingLabels();
  const formatMinutes = useFormatMinutes();
  const [candidates, setCandidates] = useState<ScheduleCandidate[] | null>(null);
  const [selected, setSelected] = useState("");
  const [loadError, setLoadError] = useState("");
  const cancelled = shift.status === "cancelled";
  const active = shift.assignments.filter((assignment) => ["planned", "confirmed"].includes(assignment.assignment_status));
  const inactive = shift.assignments.filter((assignment) => !["planned", "confirmed"].includes(assignment.assignment_status));
  const canAssign = canManage && !cancelled && shift.open_slots > 0;

  async function openPicker() {
    setLoadError("");
    try {
      const response = await getShiftCandidates(shift.id);
      setCandidates(response.candidates.filter((candidate) => !candidate.already_assigned));
    } catch (cause) {
      setLoadError(label(errorLabelKey(cause)));
    }
  }

  function candidateLabel(candidate: ScheduleCandidate) {
    const flags = [...new Set(candidate.conflicts.map((conflict) => label(`ConflictShort_${conflict.code}`)))];
    return flags.length ? `${candidate.display_name} — ${flags.join(", ")}` : candidate.display_name;
  }

  return (
    <article className={`scheduling-shift${cancelled ? " is-cancelled" : ""}`}>
      <div className="scheduling-shift__summary">
        <div className="scheduling-shift__time">
          <NexText as="span" variant="label">{formatShiftTime(shift)}</NexText>
          <NexText as="span" variant="caption" color="muted">{formatMinutes(shift.planned_minutes)}</NexText>
        </div>
        <div className="scheduling-shift__meta">
          <NexText as="span">{shift.title || label("Untitled")}</NexText>
          <NexText as="span" variant="caption" color="muted">
            {label(`Mode_${shift.assignment_mode}`)} · {label("Staffing", { assigned: shift.assigned_count, required: shift.required_headcount })}
          </NexText>
        </div>
        {cancelled ? (
          <StatusBadge prefix="Status" status="cancelled" />
        ) : shift.open_slots > 0 ? (
          <NexText as="span" variant="caption" className="scheduling-badge scheduling-badge--open">{label("OpenSlots", { count: shift.open_slots })}</NexText>
        ) : null}
      </div>

      {(active.length > 0 || inactive.length > 0) && (
        <ul className="scheduling-assignments">
          {[...active, ...inactive].map((assignment) => {
            const isActive = active.includes(assignment);
            return (
              <li key={assignment.id} className={isActive ? "" : "is-inactive"}>
                <NexText as="span">{assignment.display_name || assignment.user_id}</NexText>
                <StatusBadge prefix="Assignment" status={assignment.assignment_status} />
                {assignment.override_reason && (
                  <NexText as="span" variant="caption" color="warning" title={assignment.override_reason}>
                    {label("Overridden", { reason: assignment.override_reason })}
                  </NexText>
                )}
                {canManage && isActive && !cancelled && (
                  <NexButton size="compact" variant="secondary" disabled={busy} onClick={() => void onRemove(assignment.id)}>
                    {label("Unassign")}
                  </NexButton>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {canManage && !cancelled && (
        <div className="scheduling-actions">
          {canAssign && candidates === null && (
            <NexButton size="compact" variant="secondary" disabled={busy} onClick={() => void openPicker()}>{label("Assign")}</NexButton>
          )}
          {canAssign && candidates !== null && (
            <form
              className="scheduling-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (!selected) return;
                void onAssign(selected).then(() => { setSelected(""); setCandidates(null); });
              }}
            >
              {candidates.length === 0 ? (
                <NexText variant="caption" color="muted">{label("Candidates_Empty")}</NexText>
              ) : (
                <div className="scheduling-field">
                  <NexText as="span" variant="label">{label("Candidates_Label")}</NexText>
                  <NexSelect
                    ariaLabel={label("Candidates_Label")}
                    value={selected}
                    onChange={setSelected}
                    options={[{ value: "", label: label("Candidates_Select") }, ...candidates.map((candidate) => ({ value: candidate.user_id, label: candidateLabel(candidate) }))]}
                  />
                </div>
              )}
              <NexButton size="compact" disabled={busy || !selected}>{label("Assign")}</NexButton>
              <NexButton size="compact" type="button" variant="secondary" onClick={() => { setCandidates(null); setSelected(""); }}>{label("Close")}</NexButton>
            </form>
          )}
          <NexButton size="compact" variant="danger" disabled={busy} onClick={onCancel}>{label("Shifts_Cancel")}</NexButton>
        </div>
      )}
      {loadError && <div role="alert"><NexText variant="caption" color="danger">{loadError}</NexText></div>}
    </article>
  );
}
