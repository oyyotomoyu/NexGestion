import { request } from "@/requests/core/client";
import type { CreateGroupInput, Group, UpdateGroupInput } from "./types";

let mockGroups: Group[] = [];

export async function listGroups() {
  if (import.meta.env.DEV) return [...mockGroups];
  const response = await request<{ groups: Group[] }>("/api/groups");
  return response.groups;
}

export function createGroup(input: CreateGroupInput) {
  if (import.meta.env.DEV) {
    const now = new Date().toISOString();
    const group: Group = {
      id: crypto.randomUUID(), name: input.name.trim(), type: input.type.trim(),
      parent_group_id: input.parent_group_id || null, status: input.status || "active",
      created_at: now, updated_at: now, member_count: 0, permissions: [],
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
