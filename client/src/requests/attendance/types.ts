export type AttendanceStatus = "working" | "non_working";

export interface AttendanceSession {
  id: string;
  sequence_number: number;
  continued_from_session_id: string | null;
  sign_in_at: string;
  sign_out_at: string | null;
  worked_minutes: number | null;
}

export interface AttendanceDay {
  id: string;
  user_id: string;
  attendance_date: string;
  timezone: string;
  status: AttendanceStatus;
  worked_hours: number;
  worked_minutes: number;
  requires_review: boolean;
  sessions: AttendanceSession[];
  created_at: string;
  updated_at: string;
}

export interface AttendanceMonthlyReport {
  id: string;
  user_id: string;
  employee_number: string;
  display_name: string;
  timezone: string;
  report_month: string;
  scheduled_work_days: number;
  present_days: number;
  absent_days: number;
  incomplete_days: number;
  worked_hours: number;
  worked_minutes: number;
  generated_at: string;
}

export interface AttendanceExport {
  report_month: string;
  relative_path: string;
  sha256: string;
  row_count: number;
  generated_at: string;
}

export type LeaveDurationType = "hourly" | "full_day";

export interface LeaveType {
  key: string;
  label: string;
}

export interface LeaveRequest {
  id: string;
  user_id: string;
  leave_type: string;
  leave_date: string;
  duration_type: LeaveDurationType;
  start_time: string | null;
  end_time: string | null;
  requested_minutes: number;
  reason: string;
  status: "pending" | "approved" | "rejected" | "cancelled";
  created_at: string;
  updated_at: string;
}

export interface ApplyLeaveInput {
  leave_type: string;
  leave_date: string;
  duration_type: LeaveDurationType;
  start_time?: string;
  end_time?: string;
  reason: string;
}

export interface LeaveApprovalRequest extends LeaveRequest {
  requester_name: string;
  organization_group_id: string | null;
  administrator_route: boolean;
}

export interface DecideLeaveInput {
  decision: "approved" | "rejected";
  note: string;
}
