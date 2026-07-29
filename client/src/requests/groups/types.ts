export type GroupStatus = "active" | "inactive";
export type GroupType = "organization" | "project";
export type OrganizationLevel = 1 | 2 | 3 | 4 | 5;

export interface Group {
  id: string;
  name: string;
  type: GroupType;
  organization_level: OrganizationLevel | null;
  parent_group_id: string | null;
  status: GroupStatus;
  created_at: string;
  updated_at: string;
  member_count: number;
  manager_role_id: string;
  member_role_id: string;
}

export type GroupMemberRole = "manager" | "member";

export interface GroupMember {
  user_id: string;
  display_name: string;
  email: string;
  role: GroupMemberRole;
  title: string | null;
  joined_at: string | null;
  is_primary_organization: boolean;
}

export interface SetGroupMemberInput {
  role?: GroupMemberRole;
  title?: string;
  joined_at?: string;
  is_primary_organization?: boolean;
}

export interface CreateGroupInput {
  name: string;
  type: GroupType;
  organization_level?: OrganizationLevel;
  parent_group_id?: string;
  status?: GroupStatus;
}

export interface UpdateGroupInput {
  name?: string;
  type?: GroupType;
  organization_level?: OrganizationLevel;
  parent_group_id?: string;
  status?: GroupStatus;
}
