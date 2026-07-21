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
  permissions: Permission[];
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
