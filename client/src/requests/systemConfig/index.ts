import { request } from "@/requests/core/client";

export interface ConfigLeaveType {
  key: string;
  label: string;
  sort_order: number;
  retired: boolean;
  in_use: number;
  is_default: boolean;
  updated_at: string;
}

export type BrandingImageName = "login-logo.svg" | "language-icon.svg" | "favicon.ico";

export interface BrandingImage {
  name: BrandingImageName;
  url: string;
  updated_at: string;
}

/** Runtime overrides only; anything absent falls back to the bundled odm/ defaults. */
export interface Branding {
  colors: Record<string, string>;
  images: Partial<Record<BrandingImageName, BrandingImage>>;
  updated_at: string | null;
}

export interface PermissionDefinition {
  key: string;
  module: string;
  description: string;
  high_risk?: boolean;
  high_risk_reason?: string;
  requires_password?: boolean;
}

export interface ConfigAuditEvent {
  id: string;
  actor_user_id: string;
  surface: string;
  action: string;
  detail: string | null;
  occurred_at: string;
}

const useMock = process.env.NODE_ENV !== "production" && import.meta.env.VITE_USE_BACKEND !== "true";
const base = "/api/system-config";
const leaveTypesPath = `${base}/catalogs/leave-types`;

let mockLeaveTypes: ConfigLeaveType[] = [
  ["sick_leave", "Sick leave"],
  ["personal_leave", "Personal leave"],
  ["annual_leave", "Annual leave (paid leave)"],
  ["unpaid_leave", "Unpaid leave"],
  ["other", "Other"],
].map(([key, label], index) => ({
  key,
  label,
  sort_order: index + 1,
  retired: false,
  in_use: index === 0 ? 3 : 0,
  is_default: true,
  updated_at: new Date().toISOString(),
}));
let mockBranding: Branding = { colors: {}, images: {}, updated_at: null };
const mockAudit: ConfigAuditEvent[] = [];

function mockRecord(surface: string, action: string, detail: string) {
  mockAudit.unshift({
    id: crypto.randomUUID(),
    actor_user_id: "0",
    surface,
    action,
    detail,
    occurred_at: new Date().toISOString(),
  });
}

function mockLeaveTypesResult() {
  mockLeaveTypes = [...mockLeaveTypes].sort((a, b) => a.sort_order - b.sort_order);
  return Promise.resolve(mockLeaveTypes.map((item) => ({ ...item })));
}

async function items<T>(promise: Promise<{ items: T[] }>) {
  return (await promise).items;
}

export function listConfigLeaveTypes() {
  if (useMock) return mockLeaveTypesResult();
  return items(request<{ items: ConfigLeaveType[] }>(leaveTypesPath));
}

export function createConfigLeaveType(input: { key: string; label: string }) {
  if (useMock) {
    if (mockLeaveTypes.some((item) => item.key === input.key)) {
      return Promise.reject(new Error(`leave type "${input.key}" already exists`));
    }
    mockLeaveTypes.push({
      ...input,
      sort_order: mockLeaveTypes.length + 1,
      retired: false,
      in_use: 0,
      is_default: false,
      updated_at: new Date().toISOString(),
    });
    mockRecord("catalog.leave_types", "create", `key=${input.key}`);
    return mockLeaveTypesResult();
  }
  return items(request<{ items: ConfigLeaveType[] }>(leaveTypesPath, { method: "POST", body: JSON.stringify(input) }));
}

export function updateConfigLeaveType(key: string, input: { label?: string; retired?: boolean }) {
  if (useMock) {
    mockLeaveTypes = mockLeaveTypes.map((item) => (item.key === key ? { ...item, ...input } : item));
    mockRecord("catalog.leave_types", "update", `key=${key}`);
    return mockLeaveTypesResult();
  }
  return items(request<{ items: ConfigLeaveType[] }>(`${leaveTypesPath}/${encodeURIComponent(key)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  }));
}

export function deleteConfigLeaveType(key: string) {
  if (useMock) {
    mockLeaveTypes = mockLeaveTypes.filter((item) => item.key !== key);
    mockRecord("catalog.leave_types", "delete", `key=${key}`);
    return mockLeaveTypesResult();
  }
  return items(request<{ items: ConfigLeaveType[] }>(`${leaveTypesPath}/${encodeURIComponent(key)}`, { method: "DELETE" }));
}

export function reorderConfigLeaveTypes(keys: string[]) {
  if (useMock) {
    mockLeaveTypes = mockLeaveTypes.map((item) => ({ ...item, sort_order: keys.indexOf(item.key) + 1 }));
    mockRecord("catalog.leave_types", "reorder", keys.join(","));
    return mockLeaveTypesResult();
  }
  return items(request<{ items: ConfigLeaveType[] }>(`${leaveTypesPath}/order`, {
    method: "PUT",
    body: JSON.stringify({ keys }),
  }));
}

export function restoreConfigLeaveTypeDefaults() {
  if (useMock) {
    mockRecord("catalog.leave_types", "restore_defaults", "");
    return mockLeaveTypesResult();
  }
  return items(request<{ items: ConfigLeaveType[] }>(`${leaveTypesPath}/restore-defaults`, { method: "POST" }));
}

export function listPermissionCatalog() {
  if (useMock) {
    return Promise.resolve<PermissionDefinition[]>([
      { key: "system_config.read", module: "system_config", description: "View configuration" },
      { key: "system_config.branding.manage", module: "system_config", description: "Change branding" },
    ]);
  }
  return items(request<{ items: PermissionDefinition[] }>(`${base}/permissions`));
}

/** Public: the login page loads branding before anyone signs in. */
export function getBranding() {
  if (useMock) return Promise.resolve(structuredClone(mockBranding));
  return request<Branding>("/api/branding", {}, { auth: false, retryOnUnauthorized: false });
}

export function saveBrandingColors(colors: Record<string, string>) {
  if (useMock) {
    mockBranding = { ...mockBranding, colors: { ...colors }, updated_at: new Date().toISOString() };
    mockRecord("branding", "set_colors", Object.keys(colors).join(","));
    return getBranding();
  }
  return request<Branding>(`${base}/branding/colors`, { method: "PUT", body: JSON.stringify({ colors }) });
}

export function resetBrandingColors() {
  if (useMock) {
    mockBranding = { ...mockBranding, colors: {} };
    mockRecord("branding", "reset_colors", "");
    return getBranding();
  }
  return request<Branding>(`${base}/branding/colors`, { method: "DELETE" });
}

export function uploadBrandingImage(name: BrandingImageName, file: File) {
  if (useMock) {
    mockBranding = {
      ...mockBranding,
      images: { ...mockBranding.images, [name]: { name, url: URL.createObjectURL(file), updated_at: new Date().toISOString() } },
    };
    mockRecord("branding", "set_image", `name=${name}`);
    return getBranding();
  }
  return request<Branding>(`${base}/branding/images/${name}`, {
    method: "PUT",
    body: file,
    headers: { "Content-Type": file.type || "application/octet-stream" },
  });
}

export function resetBrandingImage(name: BrandingImageName) {
  if (useMock) {
    const images = { ...mockBranding.images };
    delete images[name];
    mockBranding = { ...mockBranding, images };
    mockRecord("branding", "reset_image", `name=${name}`);
    return getBranding();
  }
  return request<Branding>(`${base}/branding/images/${name}`, { method: "DELETE" });
}

export function listConfigAudit(limit = 50) {
  if (useMock) return Promise.resolve(mockAudit.slice(0, limit));
  return items(request<{ items: ConfigAuditEvent[] }>(`${base}/audit?limit=${limit}`));
}
