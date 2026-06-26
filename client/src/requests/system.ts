import { request } from "@/requests/core/client";

export interface HealthResponse {
  status: string;
}

export function getHealth() {
  return request<HealthResponse>("/api/health");
}
