export type UserStatus = "pending" | "active" | "disabled" | "locked";

export interface EmployeeProfile {
  id: string;
  user_id: string;
  employee_number: string;
  legal_name: string | null;
  preferred_name: string | null;
  work_email: string | null;
  work_phone: string | null;
  job_title: string | null;
  employment_status: string;
  hire_date: string | null;
  termination_date: string | null;
  manager_employee_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface Role {
  id: string;
  title: string;
  description: string | null;
  is_system: boolean;
  grants_all_permissions: boolean;
  permissions: Permission[];
}

export interface Permission {
  id: string;
  permission_key: string;
  module: string;
  description: string | null;
  high_risk: boolean;
  high_risk_reason: string | null;
  requires_password: boolean;
}

export interface UserGroup {
  id: string;
  name: string;
  type: string;
  status: string;
  title: string | null;
  joined_at: string | null;
  left_at: string | null;
}

export interface User {
  id: string;
  display_name: string;
  email: string;
  status: UserStatus;
  locale: string | null;
  timezone: string | null;
  must_change_password: boolean;
  failed_login_count: number;
  locked_until: string | null;
  last_login_at: string | null;
  password_changed_at: string | null;
  is_protected: boolean;
  created_at: string;
  updated_at: string;
  deleted_at: string | null;
  employee_profile: EmployeeProfile | null;
  roles: Role[];
  groups: UserGroup[];
}

export interface CreateUserInput {
  display_name: string;
  email: string;
  password: string;
  status?: UserStatus;
  locale?: string;
  timezone?: string;
  must_change_password?: boolean;
}

export interface UpdateUserInput {
  display_name?: string;
  email?: string;
  password?: string;
  current_password?: string;
  status?: UserStatus;
  locale?: string;
  timezone?: string;
  must_change_password?: boolean;
}
