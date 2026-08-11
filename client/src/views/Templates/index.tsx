import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { ApiError } from "@/requests/core/client";
import { listGroups } from "@/requests/groups";
import type { Group } from "@/requests/groups/types";
import { listRoles } from "@/requests/roles";
import type { Role } from "@/requests/users/types";
import {
  deleteTemplate,
  getTemplateStorageUsage,
  listTemplates,
  templateDownloadURL,
  uploadTemplate,
} from "@/requests/templates";
import type {
  TemplateAudience,
  TemplateAudienceInput,
  TemplateAudienceScope,
  TemplateFile,
  TemplateStorageUsage,
} from "@/requests/templates/types";

import "./style.css";

function hasPermission(user: ReturnType<typeof useAuth>["user"], key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true
  );
}

function formatDateTime(value: string, locale: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatBytes(bytes: number) {
  if (bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, exponent);
  return `${exponent === 0 ? value : value.toFixed(1)} ${units[exponent]}`;
}

function nextAudienceRow(scope: TemplateAudienceScope, groups: Group[], roles: Role[]): TemplateAudienceInput {
  return {
    scope,
    target_group_id: scope === "group" ? groups[0]?.id : undefined,
    target_role_id: scope === "role" ? roles[0]?.id : undefined,
    target_user_id: scope === "user" ? "" : undefined,
  };
}

function audienceValid(row: TemplateAudienceInput) {
  switch (row.scope) {
    case "organization":
      return true;
    case "group":
      return Boolean(row.target_group_id);
    case "role":
      return Boolean(row.target_role_id);
    case "user":
      return Boolean(row.target_user_id?.trim());
    default:
      return false;
  }
}

function uploadErrorKey(error: unknown) {
  if (error instanceof ApiError) {
    switch (error.body?.code) {
      case "template_file_too_large":
        return "global.k_Templates_Error_FileTooLarge";
      case "template_storage_limit_exceeded":
        return "global.k_Templates_Error_StorageFull";
      case "template_invalid":
        return "global.k_Templates_Error_Invalid";
      case "template_permission_denied":
        return "global.k_Templates_Error_PermissionDenied";
      default:
        return "global.k_Templates_Error_Upload";
    }
  }
  return "global.k_Templates_Error_Upload";
}

export default function Templates() {
  const { i18n, t } = useTranslation("ui");
  const { user } = useAuth();
  const locale = i18n.resolvedLanguage || navigator.language;
  const canUpload = hasPermission(user, "templates.upload");
  const canManageAll = hasPermission(user, "templates.manage");

  const [templates, setTemplates] = useState<TemplateFile[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [storageUsage, setStorageUsage] = useState<TemplateStorageUsage | null>(null);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [file, setFile] = useState<File | null>(null);
  const [description, setDescription] = useState("");
  const [audienceRows, setAudienceRows] = useState<TemplateAudienceInput[]>([]);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useDocumentTitle(t("global.k_Templates_PageTitle"));

  useEffect(() => {
    let active = true;
    listTemplates()
      .then((result) => {
        if (active) setTemplates(result);
      })
      .catch(() => {
        if (active) setError(t("global.k_Templates_Error_Load"));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [t]);

  useEffect(() => {
    if (!canUpload) return;
    let active = true;
    listGroups().then((result) => {
      if (active) setGroups(result);
    }).catch(() => {});
    listRoles().then((result) => {
      if (active) setRoles(result);
    }).catch(() => {});
    return () => {
      active = false;
    };
  }, [canUpload]);

  useEffect(() => {
    if (!canManageAll) return;
    let active = true;
    getTemplateStorageUsage().then((result) => {
      if (active) setStorageUsage(result);
    }).catch(() => {});
    return () => {
      active = false;
    };
  }, [canManageAll]);

  function addAudienceRow() {
    setAudienceRows((rows) => [...rows, nextAudienceRow("organization", groups, roles)]);
  }

  function updateAudienceScope(index: number, scope: TemplateAudienceScope) {
    setAudienceRows((rows) => rows.map((row, i) => (i === index ? nextAudienceRow(scope, groups, roles) : row)));
  }

  function updateAudienceTarget(index: number, patch: Partial<TemplateAudienceInput>) {
    setAudienceRows((rows) => rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  }

  function removeAudienceRow(index: number) {
    setAudienceRows((rows) => rows.filter((_, i) => i !== index));
  }

  const canSubmit = Boolean(file) && audienceRows.length > 0 && audienceRows.every(audienceValid);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSubmit || !file) return;
    setSaving(true);
    setError(null);
    setNotice(null);
    try {
      const created = await uploadTemplate({
        file,
        description: description.trim() || undefined,
        audiences: audienceRows,
      });
      setTemplates((current) => [created, ...current]);
      setFile(null);
      setDescription("");
      setAudienceRows([]);
      if (fileInputRef.current) fileInputRef.current.value = "";
      setNotice(t("global.k_Templates_UploadSuccess"));
      if (canManageAll) getTemplateStorageUsage().then(setStorageUsage).catch(() => {});
    } catch (err) {
      setError(t(uploadErrorKey(err)));
    } finally {
      setSaving(false);
    }
  }

  async function remove(item: TemplateFile) {
    if (!window.confirm(t("global.k_Templates_DeleteConfirm"))) return;
    setSaving(true);
    setError(null);
    try {
      await deleteTemplate(item.id);
      setTemplates((current) => current.filter((template) => template.id !== item.id));
      if (canManageAll) getTemplateStorageUsage().then(setStorageUsage).catch(() => {});
    } catch {
      setError(t("global.k_Templates_Error_Delete"));
    } finally {
      setSaving(false);
    }
  }

  function audienceLabel(audience: TemplateAudience) {
    switch (audience.scope) {
      case "organization":
        return t("global.k_Templates_Scope_Organization");
      case "group": {
        const group = groups.find((item) => item.id === audience.target_group_id);
        return `${t("global.k_Templates_Scope_Group")}: ${group?.name ?? audience.target_group_id}`;
      }
      case "role": {
        const role = roles.find((item) => item.id === audience.target_role_id);
        return `${t("global.k_Templates_Scope_Role")}: ${role?.title ?? audience.target_role_id}`;
      }
      case "user":
        return audience.target_user_id === user?.id
          ? t("global.k_Templates_Value_You")
          : `${t("global.k_Templates_Scope_User")}: ${audience.target_user_id}`;
      default:
        return audience.scope;
    }
  }

  const visibleTemplates = templates.filter((item) =>
    `${item.original_filename} ${item.description ?? ""}`.toLowerCase().includes(query.trim().toLowerCase()),
  );

  const scopeOptions: { value: TemplateAudienceScope; label: string }[] = [
    { value: "organization", label: t("global.k_Templates_Scope_Organization") },
    { value: "group", label: t("global.k_Templates_Scope_Group") },
    { value: "role", label: t("global.k_Templates_Scope_Role") },
    { value: "user", label: t("global.k_Templates_Scope_User") },
  ];

  return (
    <section className="templates-page">
      <header className="templates-page__header">
        <div>
          <NexText variant="title">{t("global.k_Templates_Title")}</NexText>
          <NexText color="muted">{t("global.k_Templates_Description")}</NexText>
        </div>
      </header>

      {error ? <div className="templates-alert templates-alert--error" role="alert"><NexText color="danger">{error}</NexText></div> : null}
      {notice ? <div className="templates-alert templates-alert--success" role="status"><NexText color="primary">{notice}</NexText></div> : null}

      {canManageAll && storageUsage ? (
        <section className="templates-storage" aria-labelledby="templates-storage-title">
          <NexText id="templates-storage-title" variant="subheading">{t("global.k_Templates_StorageTitle")}</NexText>
          <div className="templates-storage__bar">
            <div
              className="templates-storage__bar-fill"
              style={{ width: `${Math.min(100, (storageUsage.used_bytes / Math.max(1, storageUsage.max_bytes)) * 100)}%` }}
            />
          </div>
          <dl className="templates-metrics">
            <div>
              <dt>{t("global.k_Templates_StorageUsed")}</dt>
              <dd>{formatBytes(storageUsage.used_bytes)} / {formatBytes(storageUsage.max_bytes)}</dd>
            </div>
            <div>
              <dt>{t("global.k_Templates_StorageFiles")}</dt>
              <dd>{storageUsage.file_count}</dd>
            </div>
            <div>
              <dt>{t("global.k_Templates_StorageMaxFile")}</dt>
              <dd>{formatBytes(storageUsage.max_file_bytes)}</dd>
            </div>
          </dl>
        </section>
      ) : null}

      {canUpload ? (
        <section className="templates-upload" aria-labelledby="templates-upload-title">
          <NexText id="templates-upload-title" variant="subheading">{t("global.k_Templates_UploadTitle")}</NexText>
          <form className="template-upload-form" onSubmit={(event) => void submit(event)}>
            <label className="nex-input template-upload-form__file">
              <NexText className="nex-input__label" as="span" variant="label">{t("global.k_Templates_Label_File")}</NexText>
              <input
                ref={fileInputRef}
                type="file"
                required
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
            </label>
            <NexInput
              id="template-description"
              label={t("global.k_Templates_Label_Description")}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />

            <div className="template-upload-form__audiences">
              <div className="template-upload-form__audiences-header">
                <NexText as="span" variant="label">{t("global.k_Templates_Label_Audiences")}</NexText>
                <NexButton type="button" variant="secondary" size="compact" onClick={addAudienceRow}>
                  {t("global.k_Templates_Button_AddAudience")}
                </NexButton>
              </div>
              {audienceRows.length === 0 ? (
                <NexText color="muted" variant="caption">{t("global.k_Templates_NoAudiences")}</NexText>
              ) : null}
              {audienceRows.map((row, index) => (
                <div className="template-audience-row" key={index}>
                  <NexSelect
                    ariaLabel={t("global.k_Templates_Label_Scope")}
                    value={row.scope}
                    options={scopeOptions}
                    onChange={(scope) => updateAudienceScope(index, scope)}
                  />
                  {row.scope === "group" ? (
                    groups.length ? (
                      <NexSelect
                        ariaLabel={t("global.k_Templates_Label_TargetGroup")}
                        value={row.target_group_id ?? ""}
                        options={groups.map((group) => ({ value: group.id, label: group.name }))}
                        onChange={(value) => updateAudienceTarget(index, { target_group_id: value })}
                      />
                    ) : (
                      <NexText color="muted" variant="caption">{t("global.k_Templates_NoAudienceOptions")}</NexText>
                    )
                  ) : null}
                  {row.scope === "role" ? (
                    roles.length ? (
                      <NexSelect
                        ariaLabel={t("global.k_Templates_Label_TargetRole")}
                        value={row.target_role_id ?? ""}
                        options={roles.map((role) => ({ value: role.id, label: role.title }))}
                        onChange={(value) => updateAudienceTarget(index, { target_role_id: value })}
                      />
                    ) : (
                      <NexText color="muted" variant="caption">{t("global.k_Templates_NoAudienceOptions")}</NexText>
                    )
                  ) : null}
                  {row.scope === "user" ? (
                    <NexInput
                      id={`template-audience-user-${index}`}
                      label={t("global.k_Templates_Label_TargetUser")}
                      value={row.target_user_id ?? ""}
                      onChange={(event) => updateAudienceTarget(index, { target_user_id: event.target.value })}
                    />
                  ) : null}
                  <NexButton type="button" variant="danger" size="compact" onClick={() => removeAudienceRow(index)}>
                    {t("global.k_Templates_Button_RemoveAudience")}
                  </NexButton>
                </div>
              ))}
            </div>

            <NexButton type="submit" disabled={!canSubmit || saving}>
              {saving ? t("global.k_Common_Saving") : t("global.k_Templates_Button_Upload")}
            </NexButton>
          </form>
        </section>
      ) : null}

      <section className="templates-list" aria-labelledby="templates-list-title">
        <div className="templates-list__header">
          <NexText id="templates-list-title" variant="subheading">{t("global.k_Templates_ListTitle")}</NexText>
          <NexInput
            id="template-search"
            type="search"
            label={t("global.k_Templates_Label_Search")}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </div>

        {loading ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
        {!loading && visibleTemplates.length === 0 ? (
          <NexText color="muted">{t("global.k_Templates_Empty")}</NexText>
        ) : null}

        {visibleTemplates.length ? (
          <div className="template-table-wrap">
            <table className="template-table">
              <thead>
                <tr>
                  <th>{t("global.k_Templates_Column_Filename")}</th>
                  <th>{t("global.k_Templates_Column_Description")}</th>
                  <th>{t("global.k_Templates_Column_Size")}</th>
                  <th>{t("global.k_Templates_Column_Audiences")}</th>
                  <th>{t("global.k_Templates_Column_UploadedBy")}</th>
                  <th>{t("global.k_Templates_Column_UploadedAt")}</th>
                  <th className="template-table__actions-heading">{t("global.k_Common_Actions")}</th>
                </tr>
              </thead>
              <tbody>
                {visibleTemplates.map((item) => (
                  <tr key={item.id}>
                    <td data-label={t("global.k_Templates_Column_Filename")}>
                      <a className="template-download" href={templateDownloadURL(item.id)}>
                        <NexText as="span" color="primary" weight={600}>{item.original_filename}</NexText>
                      </a>
                    </td>
                    <td data-label={t("global.k_Templates_Column_Description")}>
                      <NexText as="span" color="muted">{item.description || t("global.k_Templates_NoDescription")}</NexText>
                    </td>
                    <td data-label={t("global.k_Templates_Column_Size")}>{formatBytes(item.size_bytes)}</td>
                    <td data-label={t("global.k_Templates_Column_Audiences")}>
                      <div className="template-audience-badges">
                        {item.audiences.map((audience) => (
                          <span className="template-audience-badge" key={audience.id}>
                            <NexText as="span" variant="caption" color="inherit">{audienceLabel(audience)}</NexText>
                          </span>
                        ))}
                      </div>
                    </td>
                    <td data-label={t("global.k_Templates_Column_UploadedBy")}>
                      {item.uploaded_by_user_id === user?.id ? t("global.k_Templates_Value_You") : item.uploaded_by_user_id}
                    </td>
                    <td data-label={t("global.k_Templates_Column_UploadedAt")}>
                      {formatDateTime(item.created_at, locale)}
                    </td>
                    <td className="template-table__actions-cell" data-label={t("global.k_Common_Actions")}>
                      {item.uploaded_by_user_id === user?.id || canManageAll ? (
                        <NexButton
                          type="button"
                          variant="danger"
                          size="compact"
                          disabled={saving}
                          onClick={() => void remove(item)}
                        >
                          {t("global.k_Common_Delete")}
                        </NexButton>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}
      </section>
    </section>
  );
}
