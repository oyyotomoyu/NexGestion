import type { Role } from "@/requests/users/types";

export type { Role };

export interface CreateRoleInput {
  title: string;
  description?: string;
}

export interface UpdateRoleInput {
  title?: string;
  description?: string;
}
