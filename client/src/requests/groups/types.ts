import type { Permission } from "@/requests/users/types";

export type GroupStatus = "active" | "inactive";

export interface Group {
  id: string;
  name: string;
  type: string;
  parent_group_id: string | null;
  status: GroupStatus;
  created_at: string;
  updated_at: string;
  member_count: number;
  manager_role_id: string;
  member_role_id: string;
  permissions: Permission[];
}

export type GroupMemberRole = "manager" | "member";

export interface GroupMember {
  user_id: string;
  display_name: string;
  email: string;
  role: GroupMemberRole;
  title: string | null;
  joined_at: string | null;
}

export interface SetGroupMemberInput {
  role?: GroupMemberRole;
  title?: string;
  joined_at?: string;
}

export interface CreateGroupInput {
  name: string;
  type: string;
  parent_group_id?: string;
  status?: GroupStatus;
}

export interface UpdateGroupInput {
  name?: string;
  type?: string;
  parent_group_id?: string;
  status?: GroupStatus;
}
