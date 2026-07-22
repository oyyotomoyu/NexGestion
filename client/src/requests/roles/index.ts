import { request } from "@/requests/core/client";
import type { CreateRoleInput, Role, UpdateRoleInput } from "./types";
import type { User } from "@/requests/users/types";

let mockRoles: Role[] = [
  {
    id: "00000000-0000-0000-0000-000000000001",
    title: "Admin",
    description: "Full system access reserved for the initial administrator",
    is_system: true,
    grants_all_permissions: true,
    permissions: [],
  },
  {
    id: "mock-store-manager",
    title: "Store Manager",
    description: "Manages store staff and daily operations",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
];

export async function listRoles() {
  if (import.meta.env.DEV) return [...mockRoles];
  const response = await request<{ roles: Role[] }>("/api/roles");
  return response.roles;
}

export function getRole(id: string) {
  if (import.meta.env.DEV) {
    const role = mockRoles.find((item) => item.id === id);
    if (!role) return Promise.reject(new Error(`Mock role ${id} was not found`));
    return Promise.resolve({ ...role });
  }
  return request<Role>(`/api/roles/${encodeURIComponent(id)}`);
}

export function createRole(input: CreateRoleInput) {
  if (import.meta.env.DEV) {
    const role: Role = {
      id: crypto.randomUUID(),
      title: input.title.trim(),
      description: input.description?.trim() || null,
      is_system: false,
      grants_all_permissions: false,
      permissions: [],
    };
    mockRoles = [...mockRoles, role];
    return Promise.resolve({ ...role });
  }
  return request<Role>("/api/roles", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateRole(id: string, input: UpdateRoleInput) {
  if (import.meta.env.DEV) {
    const index = mockRoles.findIndex((item) => item.id === id);
    if (index < 0) return Promise.reject(new Error(`Mock role ${id} was not found`));
    const current = mockRoles[index];
    const updated: Role = {
      ...current,
      title: input.title?.trim() || current.title,
      description:
        input.description === undefined
          ? current.description
          : input.description.trim() || null,
    };
    mockRoles = mockRoles.map((item) => (item.id === id ? updated : item));
    return Promise.resolve({ ...updated });
  }
  return request<Role>(`/api/roles/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteRole(id: string) {
  if (import.meta.env.DEV) {
    mockRoles = mockRoles.filter((item) => item.id !== id);
    return Promise.resolve();
  }
  return request<void>(`/api/roles/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function listRoleUsers(id:string){if(import.meta.env.DEV)return [];const response=await request<{users:User[]}>(`/api/roles/${encodeURIComponent(id)}/users`);return response.users;}
export function setRoleUser(roleId:string,userId:string,assign:boolean){if(import.meta.env.DEV)return Promise.resolve();return request<void>(`/api/roles/${encodeURIComponent(roleId)}/users/${encodeURIComponent(userId)}`,{method:assign?"PUT":"DELETE"});}
