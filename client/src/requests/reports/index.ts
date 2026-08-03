import { request } from "@/requests/core/client";
import type { ReportFile } from "@/requests/reports/types";

let mockFiles: ReportFile[] = [
  {
    path: "attendance/attendance-2026-07.csv",
    name: "attendance-2026-07.csv",
    size: 2048,
    modified_at: new Date().toISOString(),
  },
];

export async function listReportFiles() {
  if (import.meta.env.DEV) return [...mockFiles];
  const response = await request<{ files: ReportFile[] }>("/api/reports/files");
  return response.files;
}

export function reportFileDownloadURL(path: string) {
  return `/api/reports/files/${path
    .split("/")
    .map((part) => encodeURIComponent(part))
    .join("/")}`;
}

export function deleteReportFile(path: string) {
  if (import.meta.env.DEV) {
    mockFiles = mockFiles.filter((file) => file.path !== path);
    return Promise.resolve();
  }
  return request<void>(reportFileDownloadURL(path), { method: "DELETE" });
}
