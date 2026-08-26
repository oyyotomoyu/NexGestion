import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { deleteRole, getRole, updateRole } from "@/requests/roles";
import { listPermissions, setRolePermission } from "@/requests/permissions";
import type { Permission } from "@/requests/users/types";
import type { Role } from "@/requests/roles/types";

export default function RoleDetail() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const { roleId = "" } = useParams();
  const navigate = useNavigate();
  const [role, setRole] = useState<Role | null>(null);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);
  const [permissions,setPermissions]=useState<Permission[]>([]);
  const hasPermission=(key:string)=>user?.is_protected===true||user?.roles.some((item)=>item.grants_all_permissions||item.permissions.some((p)=>p.permission_key===key))===true;
  const canManage = hasPermission("roles.manage") && role?.is_system === false;
  const canAssignPermissions = user?.is_protected === true && role?.is_system === false;

  useEffect(() => {
    let active = true;
    getRole(roleId)
      .then((result) => {
        if (!active) return;
        setRole(result);
        setTitle(result.title);
        setDescription(result.description ?? "");
      })
      .catch(() => {
        if (active) setError(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [roleId]);

  useEffect(()=>{
    if (user?.is_protected !== true) {
      setPermissions([]);
      return;
    }
    listPermissions().then(setPermissions).catch(()=>setError(true));
  },[roleId,user?.is_protected]);
  async function togglePermission(permission:Permission,grant:boolean){
    let currentPassword: string | undefined;
    if (grant && permission.high_risk && permission.requires_password) {
      const warning = permission.high_risk_reason
        ? `${t("global.k_Settings_Roles_HighRiskWarning")}\n\n${permission.high_risk_reason}`
        : t("global.k_Settings_Roles_HighRiskWarning");
      if (!window.confirm(warning)) return;
      currentPassword = window.prompt(t("global.k_Settings_Roles_AdminPasswordPrompt")) ?? undefined;
      if (!currentPassword) return;
    }
    setSaving(true);
    try{await setRolePermission(roleId,permission.id,grant,currentPassword);const refreshed=await getRole(roleId);setRole(refreshed)}catch{setError(true)}finally{setSaving(false)}
  }

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!role || !title.trim()) return;
    setSaving(true);
    setError(false);
    try {
      const updated = await updateRole(role.id, { title, description });
      setRole(updated);
      setTitle(updated.title);
      setDescription(updated.description ?? "");
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    if (!role || !window.confirm(t("global.k_Settings_Roles_DeleteConfirm"))) return;
    setSaving(true);
    setError(false);
    try {
      await deleteRole(role.id);
      navigate("/settings/access-control/roles");
    } catch {
      setError(true);
      setSaving(false);
    }
  }

  if (loading) return <NexText color="muted">{t("global.k_Common_Loading")}</NexText>;
  if (!role) return <NexText color="danger">{t("global.k_Settings_Roles_Error")}</NexText>;

  return (
    <section className="roles-page">
      <Link to="/settings/access-control/roles">
        <NexText as="span" color="primary">{t("global.k_Settings_Roles_Back")}</NexText>
      </Link>
      <header className="roles-page__header">
        <div>
          <div className="role-detail__title-row">
            <NexText variant="heading">{role.title}</NexText>
            <span className={`role-badge role-badge--${role.is_system ? "system" : "custom"}`}>
              <NexText as="span" variant="caption" color="inherit" weight={600}>
                {role.is_system
                  ? t("global.k_Settings_Roles_SystemBadge")
                  : t("global.k_Settings_Roles_CustomBadge")}
              </NexText>
            </span>
          </div>
          <NexText variant="caption" color="muted">{role.id}</NexText>
        </div>
      </header>

      {error ? <NexText color="danger">{t("global.k_Settings_Roles_Error")}</NexText> : null}

      {canManage ? (
        <form className="role-form" onSubmit={(event) => void save(event)}>
          <NexText variant="subheading">{t("global.k_Settings_Roles_EditTitle")}</NexText>
          <NexInput
            id="edit-role-title"
            label={t("global.k_Settings_Roles_Label_Title")}
            value={title}
            required
            onChange={(event) => setTitle(event.target.value)}
          />
          <NexInput
            id="edit-role-description"
            label={t("global.k_Settings_Roles_Label_Description")}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
          <div className="role-detail__actions">
            <NexButton type="submit" disabled={saving || !title.trim()}>
              {saving ? t("global.k_Common_Saving") : t("global.k_Common_Save")}
            </NexButton>
            <NexButton type="button" variant="danger" disabled={saving} onClick={() => void remove()}>
              {t("global.k_Common_Delete")}
            </NexButton>
          </div>
        </form>
      ) : (
        <div className="role-form">
          <NexText>{role.description || t("global.k_Settings_Roles_NoDescription")}</NexText>
          <NexText color="muted">{t("global.k_Settings_Roles_AdminProtected")}</NexText>
        </div>
      )}

      <section className="role-form">
        <NexText variant="subheading">{t("global.k_Settings_Roles_PermissionsTitle")}</NexText>
        {role.grants_all_permissions ? (
          <>
            <NexText>{t("global.k_Settings_Roles_AllPermissions")}</NexText>
            {role.permissions.map((permission) => (
              <label key={permission.id}>
                <input type="checkbox" checked disabled /> {permission.permission_key}
              </label>
            ))}
          </>
        ) : canAssignPermissions ? (
          permissions.map((permission)=>{const granted=role.permissions.some((item)=>item.id===permission.id);return <label key={permission.id}><input type="checkbox" checked={granted} disabled={saving} onChange={(event)=>void togglePermission(permission,event.target.checked)}/> {permission.permission_key}{permission.high_risk ? ` (${t("global.k_Settings_Roles_HighRisk")})` : ""}</label>})
        ) : role.permissions.length ? (
          role.permissions.map((permission) => (
            <NexText key={permission.id}>{permission.permission_key}</NexText>
          ))
        ) : (
          <NexText color="muted">{t("global.k_Settings_Roles_NoPermissions")}</NexText>
        )}
      </section>
    </section>
  );
}
