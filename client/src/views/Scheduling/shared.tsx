import { useState } from "react";
import { NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { ApiError } from "@/requests/core/client";
import type { ScheduleShift, SchedulingConflict } from "@/requests/scheduling";
import type { User } from "@/requests/users/types";

export function can(user: User | null, key: string) {
  return user?.is_protected === true || user?.roles.some((role) =>
    role.grants_all_permissions || role.permissions.some((permission) => permission.permission_key === key),
  ) === true;
}

export function useSchedulingLabels() {
  const { t } = useTranslation("ui");
  return (key: string, options?: Record<string, unknown>) => t(`global.k_Scheduling_${key}`, options);
}

export function usePermissions() {
  const { user } = useAuth();
  return (key: string) => can(user, key);
}

export function toDateInput(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

export function addDays(value: string, days: number) {
  const [year, month, day] = value.split("-").map(Number);
  return toDateInput(new Date(year, month - 1, day + days));
}

/** Formats local shift times; overnight shifts show "+1". */
export function formatShiftTime(shift: Pick<ScheduleShift, "local_start_at" | "local_end_at">) {
  const startDate = shift.local_start_at.slice(0, 10);
  const endDate = shift.local_end_at.slice(0, 10);
  const overnight = startDate !== endDate ? " (+1)" : "";
  return `${shift.local_start_at.slice(11, 16)}–${shift.local_end_at.slice(11, 16)}${overnight}`;
}

export function useFormatMinutes() {
  const label = useSchedulingLabels();
  return (minutes: number) => {
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    return rest ? label("Duration_HoursMinutes", { hours, minutes: rest }) : label("Duration_Hours", { hours });
  };
}

export function conflictsFromError(error: unknown): SchedulingConflict[] | null {
  if (error instanceof ApiError && error.status === 409 && error.body?.code === "scheduling_conflicts") {
    return (error.body.conflicts as SchedulingConflict[]) ?? [];
  }
  return null;
}

export function errorLabelKey(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 400) return "Error_Invalid";
    if (error.status === 403) return "Error_Forbidden";
    if (error.status === 404) return "Error_NotFound";
    if (error.status === 409) return "Error_State";
  }
  return "Error_Generic";
}

export function SchedulingTabs() {
  const label = useSchedulingLabels();
  const has = usePermissions();
  return (
    <nav className="scheduling-tabs" aria-label={label("Title")}>
      {has("scheduling.read.self") && (
        <NavLink end to="/scheduling">
          <NexText as="span" variant="label" color="inherit">{label("Tab_MyShifts")}</NexText>
        </NavLink>
      )}
      {has("scheduling.read") && (
        <NavLink to="/scheduling/periods">
          <NexText as="span" variant="label" color="inherit">{label("Tab_Schedules")}</NexText>
        </NavLink>
      )}
      {has("scheduling.config.manage") && (
        <NavLink to="/scheduling/settings">
          <NexText as="span" variant="label" color="inherit">{label("Tab_Settings")}</NexText>
        </NavLink>
      )}
    </nav>
  );
}

export function StatusBadge({ status, prefix }: { status: string; prefix: "Status" | "Assignment" }) {
  const label = useSchedulingLabels();
  return (
    <NexText as="span" variant="caption" className={`scheduling-badge scheduling-badge--${status}`}>
      {label(`${prefix}_${status}`)}
    </NexText>
  );
}

interface ConflictListProps {
  conflicts: SchedulingConflict[];
  describeShift?: (shiftId: string) => string;
  describeUser?: (userId: string) => string;
}

export function ConflictList({ conflicts, describeShift, describeUser }: ConflictListProps) {
  const label = useSchedulingLabels();
  if (!conflicts.length) {
    return <NexText color="muted">{label("Conflicts_None")}</NexText>;
  }
  return (
    <ul className="scheduling-conflicts">
      {conflicts.map((conflict, index) => {
        const context = [
          conflict.user_id && describeUser ? describeUser(conflict.user_id) : "",
          conflict.shift_id && describeShift ? describeShift(conflict.shift_id) : "",
          conflict.week ? label("Conflicts_Week", { week: conflict.week }) : "",
        ].filter(Boolean).join(" · ");
        return (
          <li key={index} className={`scheduling-conflict scheduling-conflict--${conflict.severity}`}>
            <NexText as="span" variant="label" color="inherit">
              {label(conflict.severity === "error" ? "Conflicts_Blocking" : "Conflicts_Warning")}
            </NexText>
            <NexText as="span">{label(`Conflict_${conflict.code}`)}</NexText>
            {context && <NexText as="span" variant="caption" color="muted">{context}</NexText>}
          </li>
        );
      })}
    </ul>
  );
}

interface OverridePanelProps {
  conflicts: SchedulingConflict[];
  canOverride: boolean;
  busy: boolean;
  onOverride: (reason: string) => void;
  onDismiss: () => void;
  describeShift?: (shiftId: string) => string;
  describeUser?: (userId: string) => string;
}

/** Shown when the server rejects an action with blocking conflicts. */
export function OverridePanel({ conflicts, canOverride, busy, onOverride, onDismiss, describeShift, describeUser }: OverridePanelProps) {
  const label = useSchedulingLabels();
  const [reason, setReason] = useState("");
  return (
    <section className="scheduling-panel scheduling-override" role="alert">
      <NexText variant="subheading">{label("Override_Title")}</NexText>
      <ConflictList conflicts={conflicts} describeShift={describeShift} describeUser={describeUser} />
      {canOverride ? (
        <form
          className="scheduling-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (reason.trim()) onOverride(reason.trim());
          }}
        >
          <NexInput label={label("Override_Reason")} required value={reason} onChange={(event) => setReason(event.target.value)} />
          <div className="scheduling-actions">
            <NexButton disabled={busy || !reason.trim()}>{label("Override_Submit")}</NexButton>
            <NexButton type="button" variant="secondary" onClick={onDismiss}>{label("Dismiss")}</NexButton>
          </div>
        </form>
      ) : (
        <div className="scheduling-actions">
          <NexText color="muted">{label("Override_NoPermission")}</NexText>
          <NexButton type="button" variant="secondary" onClick={onDismiss}>{label("Dismiss")}</NexButton>
        </div>
      )}
    </section>
  );
}
