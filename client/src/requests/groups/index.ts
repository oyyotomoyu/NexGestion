import { request } from "@/requests/core/client";
import type { CreateGroupInput, Group, GroupMember, SetGroupMemberInput, UpdateGroupInput } from "./types";

let mockGroups: Group[] = [];

export async function listGroups() {
  if (import.meta.env.DEV) return [...mockGroups];
  const response = await request<{ groups: Group[] }>("/api/groups");
  return response.groups;
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
      id: crypto.randomUUID(), name: input.name.trim(), type: input.type.trim(),
      parent_group_id: input.parent_group_id || null, status: input.status || "active",
      created_at: now, updated_at: now, member_count: 0, permissions: [],
      manager_role_id: crypto.randomUUID(), member_role_id: crypto.randomUUID(),
    };
    mockGroups = [...mockGroups, group];
    return Promise.resolve({ ...group });
  }
  return request<Group>("/api/groups", { method: "POST", body: JSON.stringify(input) });
}

export function updateGroup(id: string, input: UpdateGroupInput) {
  return request<Group>(`/api/groups/${encodeURIComponent(id)}`, { method: "PATCH", body: JSON.stringify(input) });
}

export function deleteGroup(id: string) {
  if (import.meta.env.DEV) { mockGroups = mockGroups.filter((group) => group.id !== id); return Promise.resolve(); }
  return request<void>(`/api/groups/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function listGroupMembers(id: string) {
  const response = await request<{ members: GroupMember[] }>(`/api/groups/${encodeURIComponent(id)}/members`);
  return response.members;
}

export function setGroupMember(groupId: string, userId: string, input: SetGroupMemberInput) {
  return request<GroupMember>(`/api/groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`, {
    method: "PUT", body: JSON.stringify(input),
  });
}

export function removeGroupMember(groupId: string, userId: string) {
  return request<void>(`/api/groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`, { method: "DELETE" });
}
