import { useEffect, useMemo, useState, type ChangeEvent, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { useBranding } from "@/branding/BrandingProvider";
import { NexButton } from "@/components/NexButton";
import { defaultTheme, type NexColors } from "@/components/NexColor";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  createConfigLeaveType,
  deleteConfigLeaveType,
  listConfigAudit,
  listConfigLeaveTypes,
  listPermissionCatalog,
  reorderConfigLeaveTypes,
  resetBrandingColors,
  resetBrandingImage,
  restoreConfigLeaveTypeDefaults,
  saveBrandingColors,
  updateConfigLeaveType,
  uploadBrandingImage,
  type BrandingImageName,
  type ConfigAuditEvent,
  type ConfigLeaveType,
  type PermissionDefinition,
} from "@/requests/systemConfig";

import "./style.css";

type ColorToken = keyof NexColors;

const colorTokens = Object.keys(defaultTheme.colors) as ColorToken[];
const hexPattern = /^#(?:[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/;

const imageSlots: { name: BrandingImageName; size: string; accept: string; labelKey: string }[] = [
  { name: "login-logo.svg", size: "240 × 56", accept: ".svg,image/svg+xml", labelKey: "global.k_Settings_SystemConfig_Image_LoginLogo" },
  { name: "language-icon.svg", size: "24 × 24", accept: ".svg,image/svg+xml", labelKey: "global.k_Settings_SystemConfig_Image_LanguageIcon" },
  { name: "favicon.ico", size: "32 × 32", accept: ".ico,image/x-icon,image/vnd.microsoft.icon", labelKey: "global.k_Settings_SystemConfig_Image_Favicon" },
];

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function SystemConfig() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const branding = useBranding();
  useDocumentTitle(t("global.k_Settings_SystemConfig_Title"));
  const hasPermission = (key: string) =>
    user?.is_protected === true || user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true;
  const canManageCatalogs = hasPermission("system_config.catalogs.manage");
  const canManageBranding = hasPermission("system_config.branding.manage");
  const genericError = t("global.k_Settings_SystemConfig_Error");

  const [leaveTypes, setLeaveTypes] = useState<ConfigLeaveType[]>([]);
  const [permissions, setPermissions] = useState<PermissionDefinition[]>([]);
  const [audit, setAudit] = useState<ConfigAuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [catalogError, setCatalogError] = useState("");
  const [brandingError, setBrandingError] = useState("");
  const [loadError, setLoadError] = useState(false);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [editingLabel, setEditingLabel] = useState("");
  const [newKey, setNewKey] = useState("");
  const [newLabel, setNewLabel] = useState("");

  const savedColors = useMemo(
    () => ({ ...defaultTheme.colors, ...branding.branding.colors }) as NexColors,
    [branding.branding.colors],
  );
  const [draftColors, setDraftColors] = useState<NexColors>(savedColors);
  const [brandingSaved, setBrandingSaved] = useState(false);
  const colorsDirty = colorTokens.some((token) => draftColors[token].toLowerCase() !== savedColors[token].toLowerCase());
  const draftValid = colorTokens.every((token) => hexPattern.test(draftColors[token]));

  useEffect(() => {
    setDraftColors(savedColors);
  }, [savedColors]);

  // Live preview of unsaved colors across the whole app; always ended when
  // leaving the page so an abandoned draft never sticks.
  useEffect(() => {
    branding.previewColors(colorsDirty && draftValid ? draftColors : null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draftColors, colorsDirty, draftValid]);
  useEffect(() => () => branding.previewColors(null), [branding]);

  async function reloadAudit() {
    try {
      setAudit(await listConfigAudit(20));
    } catch {
      // The history panel is informational; editing keeps working.
    }
  }

  useEffect(() => {
    let active = true;
    Promise.all([listConfigLeaveTypes(), listPermissionCatalog(), listConfigAudit(20)])
      .then(([types, catalog, events]) => {
        if (!active) return;
        setLeaveTypes(types);
        setPermissions(catalog);
        setAudit(events);
      })
      .catch(() => active && setLoadError(true))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  async function runCatalog(action: () => Promise<ConfigLeaveType[]>) {
    setBusy(true);
    setCatalogError("");
    try {
      setLeaveTypes(await action());
      setEditingKey(null);
      void reloadAudit();
      return true;
    } catch (error) {
      setCatalogError(errorMessage(error, genericError));
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function addLeaveType(event: FormEvent) {
    event.preventDefault();
    if (await runCatalog(() => createConfigLeaveType({ key: newKey.trim(), label: newLabel.trim() }))) {
      setNewKey("");
      setNewLabel("");
    }
  }

  function move(index: number, offset: number) {
    const keys = leaveTypes.map((item) => item.key);
    const target = index + offset;
    [keys[index], keys[target]] = [keys[target], keys[index]];
    void runCatalog(() => reorderConfigLeaveTypes(keys));
  }

  function removeLeaveType(item: ConfigLeaveType) {
    if (!window.confirm(t("global.k_Settings_SystemConfig_LeaveTypes_ConfirmDelete", { label: item.label }))) return;
    void runCatalog(() => deleteConfigLeaveType(item.key));
  }

  function restoreDefaults() {
    if (!window.confirm(t("global.k_Settings_SystemConfig_LeaveTypes_ConfirmRestore"))) return;
    void runCatalog(() => restoreConfigLeaveTypeDefaults());
  }

  async function runBranding(action: () => Promise<unknown>) {
    setBusy(true);
    setBrandingError("");
    setBrandingSaved(false);
    try {
      await action();
      await branding.refresh();
      setBrandingSaved(true);
      void reloadAudit();
    } catch (error) {
      setBrandingError(errorMessage(error, genericError));
    } finally {
      setBusy(false);
    }
  }

  function saveColors() {
    // Only tokens that differ from the built-in theme are stored, so a later
    // ODM default change still reaches untouched tokens.
    const overrides: Record<string, string> = {};
    colorTokens.forEach((token) => {
      if (draftColors[token].toLowerCase() !== defaultTheme.colors[token].toLowerCase()) {
        overrides[token] = draftColors[token];
      }
    });
    void runBranding(() => saveBrandingColors(overrides));
  }

  function resetColors() {
    if (!window.confirm(t("global.k_Settings_SystemConfig_Colors_ConfirmReset"))) return;
    void runBranding(() => resetBrandingColors());
  }

  function uploadImage(name: BrandingImageName, event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    void runBranding(() => uploadBrandingImage(name, file));
  }

  function setToken(token: ColorToken, value: string) {
    setBrandingSaved(false);
    setDraftColors((current) => ({ ...current, [token]: value.trim() }));
  }

  const permissionsByModule = useMemo(() => {
    const groups = new Map<string, PermissionDefinition[]>();
    permissions.forEach((permission) => {
      groups.set(permission.module, [...(groups.get(permission.module) ?? []), permission]);
    });
    return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [permissions]);

  return (
    <section className="system-config">
      <header className="system-config__header">
        <NexText variant="heading">{t("global.k_Settings_SystemConfig_Title")}</NexText>
        <NexText color="muted">{t("global.k_Settings_SystemConfig_Description")}</NexText>
      </header>

      {loadError ? (
        <NexText className="system-config__error" role="alert">{genericError}</NexText>
      ) : null}

      <div className="system-config__card">
        <div className="system-config__card-header">
          <div>
            <NexText variant="subheading">{t("global.k_Settings_SystemConfig_LeaveTypes_Title")}</NexText>
            <NexText color="muted">{t("global.k_Settings_SystemConfig_LeaveTypes_Description")}</NexText>
          </div>
          {canManageCatalogs ? (
            <NexButton variant="secondary" size="compact" disabled={busy || loading} onClick={restoreDefaults}>
              {t("global.k_Settings_SystemConfig_LeaveTypes_RestoreDefaults")}
            </NexButton>
          ) : null}
        </div>
        {catalogError ? <NexText className="system-config__error" role="alert">{catalogError}</NexText> : null}
        <div className="system-config__table-wrap">
          <table className="system-config__table">
            <thead>
              <tr>
                <th>{t("global.k_Settings_SystemConfig_LeaveTypes_Key")}</th>
                <th>{t("global.k_Settings_SystemConfig_LeaveTypes_Label")}</th>
                <th>{t("global.k_Settings_SystemConfig_LeaveTypes_Status")}</th>
                <th>{t("global.k_Settings_SystemConfig_LeaveTypes_InUse")}</th>
                {canManageCatalogs ? <th className="system-config__actions">{t("global.k_Settings_SystemConfig_Actions")}</th> : null}
              </tr>
            </thead>
            <tbody>
              {leaveTypes.map((item, index) => (
                <tr key={item.key} className={item.retired ? "system-config__row--retired" : undefined}>
                  <td><code>{item.key}</code></td>
                  <td>
                    {editingKey === item.key ? (
                      <input
                        className="system-config__inline-input"
                        aria-label={t("global.k_Settings_SystemConfig_LeaveTypes_Label")}
                        value={editingLabel}
                        maxLength={100}
                        autoFocus
                        onChange={(event) => setEditingLabel(event.target.value)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter") void runCatalog(() => updateConfigLeaveType(item.key, { label: editingLabel }));
                          if (event.key === "Escape") setEditingKey(null);
                        }}
                      />
                    ) : (
                      <NexText as="span">{item.label}</NexText>
                    )}
                  </td>
                  <td>
                    <NexText as="span" variant="caption" color={item.retired ? "muted" : "success"}>
                      {item.retired
                        ? t("global.k_Settings_SystemConfig_LeaveTypes_Retired")
                        : t("global.k_Settings_SystemConfig_LeaveTypes_Active")}
                    </NexText>
                  </td>
                  <td><NexText as="span">{item.in_use}</NexText></td>
                  {canManageCatalogs ? (
                    <td className="system-config__actions">
                      <div className="system-config__action-row">
                        <NexButton variant="secondary" size="compact" disabled={busy || index === 0}
                          aria-label={t("global.k_Settings_SystemConfig_MoveUp")} onClick={() => move(index, -1)}>↑</NexButton>
                        <NexButton variant="secondary" size="compact" disabled={busy || index === leaveTypes.length - 1}
                          aria-label={t("global.k_Settings_SystemConfig_MoveDown")} onClick={() => move(index, 1)}>↓</NexButton>
                        {editingKey === item.key ? (
                          <>
                            <NexButton size="compact" disabled={busy || !editingLabel.trim()}
                              onClick={() => void runCatalog(() => updateConfigLeaveType(item.key, { label: editingLabel }))}>
                              {t("global.k_Settings_SystemConfig_Save")}
                            </NexButton>
                            <NexButton variant="secondary" size="compact" onClick={() => setEditingKey(null)}>
                              {t("global.k_Settings_SystemConfig_Cancel")}
                            </NexButton>
                          </>
                        ) : (
                          <NexButton variant="secondary" size="compact" disabled={busy}
                            onClick={() => { setEditingKey(item.key); setEditingLabel(item.label); }}>
                            {t("global.k_Settings_SystemConfig_Rename")}
                          </NexButton>
                        )}
                        <NexButton variant="secondary" size="compact" disabled={busy}
                          onClick={() => void runCatalog(() => updateConfigLeaveType(item.key, { retired: !item.retired }))}>
                          {item.retired
                            ? t("global.k_Settings_SystemConfig_LeaveTypes_Reactivate")
                            : t("global.k_Settings_SystemConfig_LeaveTypes_Retire")}
                        </NexButton>
                        {item.in_use === 0 ? (
                          <NexButton variant="danger" size="compact" disabled={busy} onClick={() => removeLeaveType(item)}>
                            {t("global.k_Settings_SystemConfig_Delete")}
                          </NexButton>
                        ) : null}
                      </div>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {canManageCatalogs ? (
          <form className="system-config__add-form" onSubmit={addLeaveType}>
            <NexInput
              id="system-config-new-key"
              label={t("global.k_Settings_SystemConfig_LeaveTypes_Key")}
              placeholder="volunteer_leave"
              pattern="[a-z][a-z0-9_]{0,49}"
              title={t("global.k_Settings_SystemConfig_LeaveTypes_KeyHint")}
              value={newKey}
              required
              onChange={(event) => setNewKey(event.target.value.toLowerCase())}
            />
            <NexInput
              id="system-config-new-label"
              label={t("global.k_Settings_SystemConfig_LeaveTypes_Label")}
              value={newLabel}
              maxLength={100}
              required
              onChange={(event) => setNewLabel(event.target.value)}
            />
            <NexButton type="submit" disabled={busy || !newKey.trim() || !newLabel.trim()}>
              {t("global.k_Settings_SystemConfig_LeaveTypes_Add")}
            </NexButton>
          </form>
        ) : null}
        <NexText variant="caption" color="muted">{t("global.k_Settings_SystemConfig_LeaveTypes_Hint")}</NexText>
      </div>

      <div className="system-config__card">
        <div className="system-config__card-header">
          <div>
            <NexText variant="subheading">{t("global.k_Settings_SystemConfig_Branding_Title")}</NexText>
            <NexText color="muted">{t("global.k_Settings_SystemConfig_Branding_Description")}</NexText>
          </div>
        </div>
        {brandingError ? <NexText className="system-config__error" role="alert">{brandingError}</NexText> : null}
        {brandingSaved ? (
          <NexText variant="label" color="success" role="status">{t("global.k_Settings_SystemConfig_Branding_Saved")}</NexText>
        ) : null}

        <NexText variant="label">{t("global.k_Settings_SystemConfig_Colors_Title")}</NexText>
        <div className="system-config__colors">
          {colorTokens.map((token) => {
            const value = draftColors[token];
            const valid = hexPattern.test(value);
            const overridden = branding.branding.colors[token] !== undefined;
            return (
              <label key={token} className="system-config__color">
                <input
                  type="color"
                  className="system-config__swatch"
                  aria-label={token}
                  value={valid ? value.slice(0, 7) : defaultTheme.colors[token].slice(0, 7)}
                  disabled={!canManageBranding || busy}
                  // A color picker has no alpha channel: keep the token's alpha suffix.
                  onChange={(event) => setToken(token, event.target.value + (value.length === 9 ? value.slice(7) : ""))}
                />
                <span className="system-config__color-text">
                  <NexText as="span" variant="caption">{token}{overridden ? " •" : ""}</NexText>
                  <input
                    className={`system-config__hex${valid ? "" : " system-config__hex--invalid"}`}
                    aria-label={`${token} hex`}
                    value={value}
                    maxLength={9}
                    disabled={!canManageBranding || busy}
                    onChange={(event) => setToken(token, event.target.value)}
                  />
                </span>
              </label>
            );
          })}
        </div>
        {colorsDirty ? (
          <NexText variant="caption" color="warning">{t("global.k_Settings_SystemConfig_Colors_Previewing")}</NexText>
        ) : null}
        {canManageBranding ? (
          <div className="system-config__button-row">
            <NexButton disabled={busy || !colorsDirty || !draftValid} onClick={saveColors}>
              {t("global.k_Settings_SystemConfig_Colors_Save")}
            </NexButton>
            <NexButton variant="secondary" disabled={busy || !colorsDirty} onClick={() => setDraftColors(savedColors)}>
              {t("global.k_Settings_SystemConfig_Cancel")}
            </NexButton>
            <NexButton variant="secondary" disabled={busy || Object.keys(branding.branding.colors).length === 0} onClick={resetColors}>
              {t("global.k_Settings_SystemConfig_Colors_Reset")}
            </NexButton>
          </div>
        ) : null}

        <NexText variant="label">{t("global.k_Settings_SystemConfig_Images_Title")}</NexText>
        <div className="system-config__images">
          {imageSlots.map((slot) => {
            const overridden = branding.branding.images[slot.name] !== undefined;
            return (
              <div key={slot.name} className="system-config__image-slot">
                <div className="system-config__image-preview">
                  <img src={branding.image(slot.name)} alt={t(slot.labelKey)} />
                </div>
                <NexText variant="label">{t(slot.labelKey)}</NexText>
                <NexText variant="caption" color="muted">
                  {slot.name} · {slot.size} · {overridden
                    ? t("global.k_Settings_SystemConfig_Images_Custom")
                    : t("global.k_Settings_SystemConfig_Images_Default")}
                </NexText>
                {canManageBranding ? (
                  <div className="system-config__button-row">
                    <label className={`system-config__upload${busy ? " system-config__upload--disabled" : ""}`}>
                      <input type="file" accept={slot.accept} disabled={busy} onChange={(event) => uploadImage(slot.name, event)} />
                      <NexText as="span" variant="button" color="inherit">{t("global.k_Settings_SystemConfig_Images_Upload")}</NexText>
                    </label>
                    {overridden ? (
                      <NexButton variant="secondary" size="compact" disabled={busy}
                        onClick={() => void runBranding(() => resetBrandingImage(slot.name))}>
                        {t("global.k_Settings_SystemConfig_Images_Reset")}
                      </NexButton>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>

      <div className="system-config__card">
        <div>
          <NexText variant="subheading">{t("global.k_Settings_SystemConfig_Permissions_Title")}</NexText>
          <NexText color="muted">{t("global.k_Settings_SystemConfig_Permissions_Description")}</NexText>
        </div>
        <div className="system-config__permission-groups">
          {permissionsByModule.map(([module, items]) => (
            <details key={module} className="system-config__permission-group">
              <summary>
                <NexText as="span" variant="label">{module}</NexText>
                <NexText as="span" variant="caption" color="muted"> · {items.length}</NexText>
              </summary>
              <ul>
                {items.map((permission) => (
                  <li key={permission.key}>
                    <code>{permission.key}</code>
                    {permission.high_risk ? (
                      <NexText as="span" variant="caption" color="danger"> {t("global.k_Settings_SystemConfig_Permissions_HighRisk")}</NexText>
                    ) : null}
                    <NexText variant="caption" color="muted">{permission.description}</NexText>
                  </li>
                ))}
              </ul>
            </details>
          ))}
        </div>
      </div>

      <div className="system-config__card">
        <NexText variant="subheading">{t("global.k_Settings_SystemConfig_History_Title")}</NexText>
        {audit.length === 0 ? (
          <NexText color="muted">{t("global.k_Settings_SystemConfig_History_Empty")}</NexText>
        ) : (
          <ul className="system-config__history">
            {audit.map((event) => (
              <li key={event.id}>
                <NexText as="span" variant="caption" color="muted">{new Date(event.occurred_at).toLocaleString()}</NexText>
                <NexText as="span" variant="label">{event.surface} · {event.action}</NexText>
                {event.detail ? <NexText as="span" variant="caption">{event.detail}</NexText> : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
