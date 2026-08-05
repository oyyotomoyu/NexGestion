import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type { CreateGroupInput, Group, GroupMember, SetGroupMemberInput, UpdateGroupInput } from "./types";

let mockGroups: Group[] = [
  {
    id: "group-operations",
    name: "Operations",
    type: "organization",
    organization_level: 1,
    parent_group_id: null,
    status: "active",
    created_at: "2025-02-03T03:00:00.000Z",
    updated_at: "2026-07-13T08:45:00.000Z",
    member_count: 1,
    manager_role_id: "group-operations-manager",
    member_role_id: "group-operations-member",
  },
  {
    id: "group-people",
    name: "People",
    type: "project",
    organization_level: null,
    parent_group_id: null,
    status: "active",
    created_at: "2026-07-01T06:30:00.000Z",
    updated_at: "2026-07-01T06:30:00.000Z",
    member_count: 1,
    manager_role_id: "group-people-manager",
    member_role_id: "group-people-member",
  },
];

let mockGroupMembers: Record<string, GroupMember[]> = {
  "group-operations": [{
    user_id: "1", display_name: "Mina Lin", email: "mina.lin@nexgestion.test",
    role: "manager", title: "Manager", joined_at: "2025-02-03", is_primary_organization: true,
  }],
  "group-people": [{
    user_id: "2", display_name: "Jordan Wang", email: "jordan.wang@nexgestion.test",
    role: "member", title: "Specialist", joined_at: "2026-07-01", is_primary_organization: false,
  }],
};

export async function listGroups(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockGroups];
  const response = await request<ListResponse<Group, "groups">>(
    buildListPath("/api/groups", { sort: "name", order: "asc", page_size: 100, ...query }),
  );
  return listItems(response, "groups");
}

export function getGroup(id: string) {
  if (import.meta.env.DEV) {
    const group = mockGroups.find((item) => item.id === id);
    return group ? Promise.resolve({ ...group }) : Promise.reject(new Error("Group not found"));
  }
  return request<Group>(`/api/groups/${encodeURIComponent(id)}`);
}

export function createGroup(input: CreateGroupInput) {
  if (import.meta.env.DEV) {
    const now = new Date().toISOString();
    const group: Group = {
      id: crypto.randomUUID(), name: input.name.trim(), type: input.type,
      organization_level: input.type === "organization" ? input.organization_level ?? 1 : null,
      parent_group_id: input.parent_group_id || null, status: input.status || "active",
      created_at: now, updated_at: now, member_count: 0,
      manager_role_id: crypto.randomUUID(), member_role_id: crypto.randomUUID(),
    };
    mockGroups = [...mockGroups, group];
    return Promise.resolve({ ...group });
  }
  return request<Group>("/api/groups", { method: "POST", body: JSON.stringify(input) });
}

export function updateGroup(id: string, input: UpdateGroupInput) {
  if (import.meta.env.DEV) {
    const index = mockGroups.findIndex((group) => group.id === id);
    if (index < 0) return Promise.reject(new Error("Group not found"));
    const current = mockGroups[index];
    const updated: Group = {
      ...current,
      name: input.name?.trim() || current.name,
      type: input.type || current.type,
      organization_level: (input.type ?? current.type) === "organization"
        ? input.organization_level ?? current.organization_level ?? 1
        : null,
      parent_group_id: input.parent_group_id === undefined ? current.parent_group_id : input.parent_group_id || null,
      status: input.status || current.status,
      updated_at: new Date().toISOString(),
    };
    mockGroups = mockGroups.map((group) => group.id === id ? updated : group);
    return Promise.resolve({ ...updated });
  }
  return request<Group>(`/api/groups/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(input) });
}

export function deleteGroup(id: string) {
  if (import.meta.env.DEV) { mockGroups = mockGroups.filter((group) => group.id !== id); return Promise.resolve(); }
  return request<void>(`/api/groups/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listGroupMembers(id: string, query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...(mockGroupMembers[id] || [])];
  const response = await request<ListResponse<GroupMember, "members">>(
    buildListPath(`/api/groups/${encodeURIComponent(id)}/members`, { sort: "display_name", order: "asc", page_size: 100, ...query }),
  );
  return listItems(response, "members");
}

export function setGroupMember(groupId: string, userId: string, input: SetGroupMemberInput) {
  return request<GroupMember>(`/api/groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`, {
    method: "PUT", body: JSON.stringify(input),
  });
}

export function removeGroupMember(groupId: string, userId: string) {
  return request<void>(`/api/groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`, { method: "DELETE" });
}
