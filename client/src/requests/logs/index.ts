import { request } from "@/requests/core/client";
import type { LogQuery, LogResponse } from "./types";

export type { LogQuery, LogRecord, LogResponse, LogStatus } from "./types";

export function listLogs(query: LogQuery = {}) {
  const parameters = new URLSearchParams();
  if (query.start) parameters.set("start", toRFC3339(query.start));
  if (query.end) parameters.set("end", toRFC3339(query.end));
  if (query.status) {
    parameters.set(
      "status",
      Array.isArray(query.status) ? query.status.join(",") : query.status,
    );
  }
  if (query.limit != null) parameters.set("limit", String(query.limit));
  if (query.cursor) parameters.set("cursor", query.cursor);

  const queryString = parameters.toString();
  return request<LogResponse>(`/api/logs${queryString ? `?${queryString}` : ""}`);
}

function toRFC3339(value: Date | string) {
  return value instanceof Date ? value.toISOString() : value;
}
