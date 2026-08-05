import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type { CreateRoleInput, Role, UpdateRoleInput } from "./types";
import type { User } from "@/requests/users/types";

let mockRoles: Role[] = [
  {
    id: "role-admin",
    title: "Admin",
    description: "Full system access reserved for user id 0",
    is_system: true,
    grants_all_permissions: true,
    permissions: [],
  },
  {
    id: "role-manager",
    title: "Manager",
    description: "Team management access",
    is_system: true,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-hr",
    title: "HR",
    description: "People operations access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-finance",
    title: "Finance",
    description: "Finance workspace access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-support",
    title: "Support",
    description: "Customer support access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-sales",
    title: "Sales",
    description: "Sales workspace access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-product",
    title: "Product",
    description: "Product planning access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-engineering",
    title: "Engineering",
    description: "Engineering workspace access",
    is_system: false,
    grants_all_permissions: false,
    permissions: [],
  },
  {
    id: "role-customer-success",
    title: "Customer Success",
    description: "Customer success workspace access",
    is_system: false,
    grants_all_permissions: false,
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

export async function listRoles(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockRoles];
  const response = await request<ListResponse<Role, "roles">>(
    buildListPath("/api/roles", { sort: "title", order: "asc", page_size: 100, ...query }),
  );
  return listItems(response, "roles");
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

export async function listRoleUsers(id:string,query:ListQuery={}){if(import.meta.env.DEV)return [];const response=await request<ListResponse<User,"users">>(buildListPath(`/api/roles/${encodeURIComponent(id)}/users`,{sort:"display_name",order:"asc",page_size:100,...query}));return listItems(response,"users");}
export function setRoleUser(roleId:string,userId:string,assign:boolean){if(import.meta.env.DEV)return Promise.resolve();return request<void>(`/api/roles/${encodeURIComponent(roleId)}/users/${encodeURIComponent(userId)}`,{method:assign?"PUT":"DELETE"});}
