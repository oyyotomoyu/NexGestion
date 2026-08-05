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
  if (query.page != null) parameters.set("page", String(query.page));
  if (query.page_size != null) parameters.set("page_size", String(query.page_size));
  if (query.keyword) parameters.set("keyword", query.keyword);
  if (query.sort) parameters.set("sort", query.sort);
  if (query.order) parameters.set("order", query.order);

  const queryString = parameters.toString();
  return request<LogResponse>(`/api/logs${queryString ? `?${queryString}` : ""}`);
}

function toRFC3339(value: Date | string) {
  return value instanceof Date ? value.toISOString() : value;
}
