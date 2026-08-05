import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type {
  ApplyLeaveInput,
  AttendanceDay,
  AttendanceExport,
  AttendanceMonthlyReport,
  LeaveRequest,
  LeaveApprovalRequest,
  DecideLeaveInput,
  LeaveType,
} from "@/requests/attendance/types";

let mockToday: AttendanceDay | null = null;
const mockLeaveRequests: LeaveRequest[] = [];
const mockLeaveApprovals: LeaveApprovalRequest[] = [];
const mockLeaveTypes: LeaveType[] = [
  ["sick_leave", "Sick leave"],
  ["personal_leave", "Personal leave"],
  ["annual_leave", "Annual leave (paid leave)"],
  ["official_leave", "Official leave"],
  ["maternity_leave", "Maternity leave"],
  ["paternity_leave", "Paternity leave"],
  ["parental_leave", "Parental leave"],
  ["bereavement_leave", "Bereavement leave"],
  ["menstrual_leave", "Menstrual leave"],
  ["marriage_leave", "Marriage leave"],
  ["unpaid_leave", "Unpaid leave"],
  ["other", "Other"],
].map(([key, label]) => ({ key, label }));

function nowISO() {
  const now = new Date();
  now.setSeconds(0, 0);
  return now.toISOString();
}

function currentDate() {
  return new Date().toLocaleDateString("en-CA");
}

function createMockDay(): AttendanceDay {
  const now = nowISO();
  return {
    id: `att-${currentDate()}`,
    user_id: "0",
    attendance_date: currentDate(),
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Taipei",
    status: "non_working",
    worked_hours: 0,
    worked_minutes: 0,
    requires_review: false,
    sessions: [],
    created_at: now,
    updated_at: now,
  };
}

function minutesBetween(start: string, end: string) {
  return Math.max(0, Math.floor((Date.parse(end) - Date.parse(start)) / 60000));
}

function refreshMockTotals(day: AttendanceDay): AttendanceDay {
  const now = nowISO();
  const workedMinutes = day.sessions.reduce((total, session) => {
    const end = session.sign_out_at ?? (day.status === "working" ? now : session.sign_in_at);
    return total + minutesBetween(session.sign_in_at, end);
  }, 0);
  return {
    ...day,
    worked_minutes: workedMinutes,
    worked_hours: Math.round((workedMinutes / 60) * 100) / 100,
    updated_at: now,
  };
}

function getMockToday() {
  if (!mockToday || mockToday.attendance_date !== currentDate()) {
    mockToday = createMockDay();
  }
  mockToday = refreshMockTotals(mockToday);
  return Promise.resolve({ ...mockToday, sessions: [...mockToday.sessions] });
}

export async function getAttendanceToday() {
  if (import.meta.env.DEV) return getMockToday();
  return request<AttendanceDay>("/api/attendance/today");
}

export async function signInAttendance() {
  if (import.meta.env.DEV) {
    const day = await getMockToday();
    if (day.status === "working") return day;
    mockToday = {
      ...day,
      status: "working",
      sessions: [
        ...day.sessions,
        {
          id: crypto.randomUUID(),
          sequence_number: day.sessions.length + 1,
          continued_from_session_id: null,
          sign_in_at: nowISO(),
          sign_out_at: null,
          worked_minutes: null,
        },
      ],
    };
    return getMockToday();
  }
  return request<AttendanceDay>("/api/attendance/today/sign-in", { method: "POST" });
}

export async function signOutAttendance() {
  if (import.meta.env.DEV) {
    const day = await getMockToday();
    if (day.status === "non_working") return day;
    const endedAt = nowISO();
    mockToday = {
      ...day,
      status: "non_working",
      sessions: day.sessions.map((session) =>
        session.sign_out_at
          ? session
          : {
              ...session,
              sign_out_at: endedAt,
              worked_minutes: minutesBetween(session.sign_in_at, endedAt),
            },
      ),
    };
    return getMockToday();
  }
  return request<AttendanceDay>("/api/attendance/today/sign-out", { method: "POST" });
}

export async function listAttendanceDays(month: string, query: ListQuery = {}) {
  if (import.meta.env.DEV) {
    const day = await getMockToday();
    return day.attendance_date.startsWith(month) ? [day] : [];
  }
  const response = await request<ListResponse<AttendanceDay, "days">>(
    buildListPath("/api/attendance/days", { month, sort: "attendance_date", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "days");
}

export async function getSelfAttendanceMonthlyReport(month: string) {
  if (import.meta.env.DEV) {
    const days = await listAttendanceDays(month);
    const workedMinutes = days.reduce((total, day) => total + day.worked_minutes, 0);
    return {
      id: `report-${month}`,
      user_id: "0",
      employee_number: "-",
      display_name: "Development user",
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Taipei",
      report_month: month,
      scheduled_work_days: 0,
      present_days: days.filter((day) => day.worked_minutes > 0).length,
      absent_days: 0,
      incomplete_days: days.filter((day) => day.requires_review).length,
      worked_hours: Math.round((workedMinutes / 60) * 100) / 100,
      worked_minutes: workedMinutes,
      generated_at: nowISO(),
    } satisfies AttendanceMonthlyReport;
  }
  return request<AttendanceMonthlyReport>(
    `/api/attendance/monthly/${encodeURIComponent(month)}`,
  );
}

export async function generateAttendanceReport(month: string) {
  if (import.meta.env.DEV) {
    return {
      report_month: month,
      relative_path: `attendance-${month}.csv`,
      sha256: "dev",
      row_count: 1,
      generated_at: nowISO(),
    } satisfies AttendanceExport;
  }
  return request<AttendanceExport>(
    `/api/attendance/reports/${encodeURIComponent(month)}/generate`,
    { method: "POST" },
  );
}

export function attendanceCSVURL(month: string) {
  return `/api/attendance/reports/${encodeURIComponent(month)}/csv`;
}

export async function getLeaveTypes() {
  if (import.meta.env.DEV) return mockLeaveTypes;
  const response = await request<{ leave_types: LeaveType[] }>("/api/attendance/leave-types");
  return response.leave_types;
}

export async function listLeaveRequests(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockLeaveRequests];
  const response = await request<ListResponse<LeaveRequest, "leave_requests">>(
    buildListPath("/api/attendance/leave-requests", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "leave_requests");
}

export async function applyLeave(input: ApplyLeaveInput) {
  if (import.meta.env.DEV) {
    const now = nowISO();
    const request: LeaveRequest = {
      id: crypto.randomUUID(),
      user_id: "0",
      leave_type: input.leave_type,
      leave_date: input.leave_date,
      duration_type: input.duration_type,
      start_time: input.duration_type === "hourly" ? input.start_time ?? null : null,
      end_time: input.duration_type === "hourly" ? input.end_time ?? null : null,
      requested_minutes:
        input.duration_type === "full_day"
          ? 480
          : Math.max(
              0,
              (Date.parse(`2000-01-01T${input.end_time}:00`) -
                Date.parse(`2000-01-01T${input.start_time}:00`)) /
                60000,
            ),
      reason: input.reason,
      status: "pending",
      created_at: now,
      updated_at: now,
    };
    mockLeaveRequests.unshift(request);
    return request;
  }
  return request<LeaveRequest>("/api/attendance/leave-requests", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function listLeaveApprovals(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockLeaveApprovals];
  const response = await request<ListResponse<LeaveApprovalRequest, "leave_requests">>(
    buildListPath("/api/attendance/leave-approvals", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "leave_requests");
}

export async function decideLeave(id: string, input: DecideLeaveInput) {
  if (import.meta.env.DEV) {
    const current = mockLeaveApprovals.find((item) => item.id === id);
    if (!current) throw new Error("Leave request not found");
    const updated = { ...current, status: input.decision, updated_at: nowISO() };
    const index = mockLeaveApprovals.findIndex((item) => item.id === id);
    mockLeaveApprovals[index] = updated;
    return updated;
  }
  return request<LeaveApprovalRequest>(
    `/api/attendance/leave-approvals/${encodeURIComponent(id)}`,
    { method: "PATCH", body: JSON.stringify(input) },
  );
}
