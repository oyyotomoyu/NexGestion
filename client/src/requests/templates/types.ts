export type TemplateAudienceScope = "organization" | "group" | "role" | "user";

export interface TemplateAudience {
  id: string;
  scope: TemplateAudienceScope;
  target_group_id: string | null;
  target_role_id: string | null;
  target_user_id: string | null;
}

export interface TemplateAudienceInput {
  scope: TemplateAudienceScope;
  target_group_id?: string;
  target_role_id?: string;
  target_user_id?: string;
}

export interface TemplateFile {
  id: string;
  original_filename: string;
  content_type: string;
  size_bytes: number;
  checksum_sha256: string;
  description: string | null;
  uploaded_by_user_id: string;
  audiences: TemplateAudience[];
  created_at: string;
  updated_at: string;
}

export interface TemplateStorageUsage {
  used_bytes: number;
  max_bytes: number;
  max_file_bytes: number;
  file_count: number;
}

export interface UploadTemplateInput {
  file: File;
  description?: string;
  audiences: TemplateAudienceInput[];
}
