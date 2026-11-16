import { request } from "@/requests/core/client";
import { buildListPath, type ListQuery, type ListResponse } from "@/requests/core/list";

export type PeriodStatus = "draft" | "published" | "locked" | "archived" | "cancelled";
export type PeriodAction = "publish" | "lock" | "archive" | "cancel";
export type AssignmentMode = "named_users" | "open_shift" | "requirement_only";
export type AssignmentStatus = "planned" | "confirmed" | "declined" | "cancelled" | "completed";

export interface SchedulingConfig {
  group_mode: "required" | "advisory";
  allow_open_shifts: boolean;
  max_weekly_minutes: number;
  min_rest_minutes: number;
}

export interface ScheduleAssignment {
  id: string;
  shift_id: string;
  user_id: string;
  display_name?: string;
  assignment_status: AssignmentStatus;
  source: string;
  override_reason: string | null;
  created_at: string;
  updated_at: string;
}

export interface ScheduleShift {
  id: string;
  schedule_period_id: string;
  title: string | null;
  local_start_at: string;
  local_end_at: string;
  start_at_utc: string;
  end_at_utc: string;
  planned_minutes: number;
  required_headcount: number;
  assignment_mode: AssignmentMode;
  notes: string | null;
  status: "draft" | "published" | "cancelled" | "completed";
  assigned_count: number;
  open_slots: number;
  assignments: ScheduleAssignment[];
  created_at: string;
  updated_at: string;
}

export interface SchedulePeriod {
  id: string;
  group_id: string;
  group_name: string;
  group_type: string;
  name: string;
  timezone: string;
  start_date: string;
  end_date: string;
  status: PeriodStatus;
  created_by_user_id: string;
  published_at: string | null;
  created_at: string;
  updated_at: string;
  shift_count: number;
  shifts?: ScheduleShift[];
}

export interface SchedulingConflict {
  code: string;
  severity: "error" | "warning";
  message: string;
  shift_id?: string;
  related_shift_id?: string;
  user_id?: string;
  week?: string;
}

export interface ScheduleCandidate {
  user_id: string;
  display_name: string;
  email: string;
  title: string | null;
  already_assigned: boolean;
  conflicts: SchedulingConflict[];
}

export interface MyScheduledShift extends ScheduleShift {
  period_name: string;
  period_status: PeriodStatus;
  group_id: string;
  group_name: string;
  timezone: string;
  assignment_id: string;
  assignment_status: AssignmentStatus;
}

export interface SchedulingEvent {
  id: string;
  shift_id: string | null;
  assignment_id: string | null;
  actor_user_id: string;
  event_type: string;
  reason: string | null;
  detail: string | null;
  occurred_at: string;
}

export interface SchedulingGroup {
  id: string;
  name: string;
  type: string;
  status: string;
}

export interface PeriodInput {
  group_id: string;
  name: string;
  timezone: string;
  start_date: string;
  end_date: string;
}

export interface ShiftInput {
  title?: string;
  local_start_at: string;
  local_end_at: string;
  required_headcount: number;
  assignment_mode: AssignmentMode;
  notes?: string;
}

type Warned<Key extends string, T> = { [K in Key]: T } & { warnings: SchedulingConflict[] };

const root = "/api/scheduling";
const id = (value: string) => encodeURIComponent(value);
const send = <T>(path: string, method: string, body?: unknown) =>
  request<T>(path, { method, body: body === undefined ? undefined : JSON.stringify(body) });

export const getSchedulingConfig = () => request<SchedulingConfig>(`${root}/config`);
export const updateSchedulingConfig = (input: Partial<SchedulingConfig>) => send<SchedulingConfig>(`${root}/config`, "PATCH", input);

export const listSchedulingGroups = (query: ListQuery = {}) =>
  request<ListResponse<SchedulingGroup, "groups">>(buildListPath(`${root}/groups`, { sort: "name", order: "asc", page_size: 100, ...query }));

export const listPeriods = (query: ListQuery) => request<ListResponse<SchedulePeriod, "periods">>(buildListPath(`${root}/periods`, query));
export const getPeriod = (periodId: string) => request<SchedulePeriod>(`${root}/periods/${id(periodId)}`);
export const createPeriod = (input: PeriodInput) => send<SchedulePeriod>(`${root}/periods`, "POST", input);
export const updatePeriod = (periodId: string, input: Partial<Omit<PeriodInput, "group_id">>) => send<SchedulePeriod>(`${root}/periods/${id(periodId)}`, "PATCH", input);
export const transitionPeriod = (periodId: string, action: PeriodAction, reason?: string) =>
  send<Warned<"period", SchedulePeriod>>(`${root}/periods/${id(periodId)}/${action}`, "POST", reason ? { reason } : undefined);
export const getPeriodConflicts = (periodId: string) => request<{ conflicts: SchedulingConflict[] }>(`${root}/periods/${id(periodId)}/conflicts`);
export const getPeriodEvents = (periodId: string) => request<{ events: SchedulingEvent[] }>(`${root}/periods/${id(periodId)}/events`);

export const createShift = (periodId: string, input: ShiftInput) => send<ScheduleShift>(`${root}/periods/${id(periodId)}/shifts`, "POST", input);
export const updateShift = (shiftId: string, input: Partial<ShiftInput> & { override_reason?: string }) =>
  send<Warned<"shift", ScheduleShift>>(`${root}/shifts/${id(shiftId)}`, "PATCH", input);
export const cancelShift = (shiftId: string) => send<ScheduleShift>(`${root}/shifts/${id(shiftId)}/cancel`, "POST");
export const getShiftCandidates = (shiftId: string) => request<{ candidates: ScheduleCandidate[] }>(`${root}/shifts/${id(shiftId)}/candidates`);
export const assignUser = (shiftId: string, userId: string, overrideReason?: string) =>
  send<Warned<"assignment", ScheduleAssignment>>(`${root}/shifts/${id(shiftId)}/assignments`, "POST", { user_id: userId, override_reason: overrideReason || undefined });
export const cancelAssignment = (assignmentId: string) => send<void>(`${root}/assignments/${id(assignmentId)}`, "DELETE");

export const listMyShifts = (from: string, to: string) =>
  request<{ shifts: MyScheduledShift[] }>(buildListPath(`${root}/me/shifts`, { from, to }));
export const respondToAssignment = (assignmentId: string, status: "confirmed" | "declined") =>
  send<ScheduleAssignment>(`${root}/me/assignments/${id(assignmentId)}`, "PATCH", { assignment_status: status });
