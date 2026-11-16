import {
  clearAccessToken,
  request,
  setAccessToken,
} from "@/requests/core/client";
import type { User } from "@/requests/users/types";

export interface LoginInput {
  email: string;
  password: string;
}

export interface TokenResponse {
  access_token: string;
  token_type: "Bearer";
  expires_in: number;
}

export async function login(input: LoginInput) {
  const tokens = await request<TokenResponse>(
    "/api/auth/login",
    { method: "POST", body: JSON.stringify(input) },
    { auth: false, retryOnUnauthorized: false },
  );
  setAccessToken(tokens.access_token);
  return tokens;
}

export async function refreshSession() {
  const tokens = await request<TokenResponse>(
    "/api/auth/refresh",
    { method: "POST" },
    { auth: false, retryOnUnauthorized: false },
  );
  setAccessToken(tokens.access_token);
  return tokens;
}

export function getCurrentUser() {
  return request<User>("/api/auth/me");
}

export async function logout() {
  try {
    await request<void>("/api/auth/logout", { method: "POST" });
  } finally {
    clearAccessToken();
  }
}

export interface SSOConnection {
  id: string;
  name: string;
  provider_type: string;
}

export async function listSSOConnections() {
  const response = await request<{ items: SSOConnection[] }>(
    "/api/auth/sso/connections",
    { method: "GET" },
    { auth: false, retryOnUnauthorized: false },
  );
  return response.items;
}

export async function startSSOLogin(connectionId: string, returnTo?: string) {
  const response = await request<{ authorization_url: string }>(
    `/api/auth/sso/${encodeURIComponent(connectionId)}/start`,
    { method: "POST", body: JSON.stringify({ return_to: returnTo ?? "" }) },
    { auth: false, retryOnUnauthorized: false },
  );
  return response.authorization_url;
}
