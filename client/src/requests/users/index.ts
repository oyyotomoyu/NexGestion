import { request } from "@/requests/core/client";
import type { CreateUserInput, UpdateUserInput, User } from "./types";

export type { CreateUserInput, UpdateUserInput, User } from "./types";

export async function listUsers() {
  const response = await request<{ users: User[] }>("/api/users");
  return response.users;
}

export function getUser(id: string) {
  return request<User>(`/api/users/${encodeURIComponent(id)}`);
}

export function createUser(input: CreateUserInput) {
  return request<User>("/api/users", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateUser(id: string, input: UpdateUserInput) {
  return request<User>(`/api/users/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function replaceUser(id: string, input: UpdateUserInput) {
  return request<User>(`/api/users/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function deleteUser(id: string) {
  return request<void>(`/api/users/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}
