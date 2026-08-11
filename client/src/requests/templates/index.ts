import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type {
  TemplateFile,
  TemplateStorageUsage,
  UploadTemplateInput,
} from "@/requests/templates/types";

let mockFiles: TemplateFile[] = [
  {
    id: "mock-template-1",
    original_filename: "會員資料填寫表.docx",
    content_type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    size_bytes: 24576,
    checksum_sha256: "",
    description: "會員資料填寫表",
    uploaded_by_user_id: "mock-user",
    audiences: [
      { id: "mock-audience-1", scope: "organization", target_group_id: null, target_role_id: null, target_user_id: null },
    ],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  },
];

export async function listTemplates(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockFiles];
  const response = await request<ListResponse<TemplateFile, "templates">>(
    buildListPath("/api/templates", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "templates");
}

export function templateDownloadURL(id: string) {
  return `/api/templates/${encodeURIComponent(id)}/download`;
}

export async function uploadTemplate(input: UploadTemplateInput) {
  if (import.meta.env.DEV) {
    const created: TemplateFile = {
      id: `mock-template-${mockFiles.length + 1}`,
      original_filename: input.file.name,
      content_type: input.file.type || "application/octet-stream",
      size_bytes: input.file.size,
      checksum_sha256: "",
      description: input.description ?? null,
      uploaded_by_user_id: "mock-user",
      audiences: input.audiences.map((audience, index) => ({
        id: `mock-audience-${index}`,
        scope: audience.scope,
        target_group_id: audience.target_group_id ?? null,
        target_role_id: audience.target_role_id ?? null,
        target_user_id: audience.target_user_id ?? null,
      })),
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    mockFiles = [created, ...mockFiles];
    return created;
  }
  const formData = new FormData();
  formData.append("file", input.file);
  formData.append("audiences", JSON.stringify(input.audiences));
  if (input.description) formData.append("description", input.description);
  return request<TemplateFile>("/api/templates", { method: "POST", body: formData });
}

export function deleteTemplate(id: string) {
  if (import.meta.env.DEV) {
    mockFiles = mockFiles.filter((file) => file.id !== id);
    return Promise.resolve();
  }
  return request<void>(`/api/templates/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function getTemplateStorageUsage() {
  if (import.meta.env.DEV) {
    const used = mockFiles.reduce((total, file) => total + file.size_bytes, 0);
    const usage: TemplateStorageUsage = {
      used_bytes: used,
      max_bytes: 500 * 1024 * 1024,
      max_file_bytes: 20 * 1024 * 1024,
      file_count: mockFiles.length,
    };
    return Promise.resolve(usage);
  }
  return request<TemplateStorageUsage>("/api/templates/storage");
}
