import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { createRole, listRoles } from "@/requests/roles";
import type { Role } from "@/requests/roles/types";

import "./style.css";

export default function Roles() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canManage = user?.is_protected === true;
  const [roles, setRoles] = useState<Role[]>([]);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    let active = true;
    listRoles()
      .then((result) => {
        if (active) setRoles(result);
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
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!title.trim()) return;
    setSaving(true);
    setError(false);
    try {
      const role = await createRole({ title, description });
      setRoles((current) => [...current, role].sort((a, b) => a.title.localeCompare(b.title)));
      setTitle("");
      setDescription("");
    } catch {
      setError(true);
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="roles-page">
      <header className="roles-page__header">
        <div>
          <NexText variant="heading">{t("global.k_Settings_Roles_Title")}</NexText>
          <NexText color="muted">{t("global.k_Settings_Roles_Description")}</NexText>
        </div>
      </header>

      {canManage ? (
        <form className="role-form" onSubmit={(event) => void submit(event)}>
          <NexText variant="subheading">{t("global.k_Settings_Roles_CreateTitle")}</NexText>
          <div className="role-form__fields">
            <NexInput
              id="role-title"
              label={t("global.k_Settings_Roles_Label_Title")}
              value={title}
              required
              onChange={(event) => setTitle(event.target.value)}
            />
            <NexInput
              id="role-description"
              label={t("global.k_Settings_Roles_Label_Description")}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          <NexButton type="submit" disabled={saving || !title.trim()}>
            {saving ? t("global.k_Common_Saving") : t("global.k_Settings_Roles_Button_Create")}
          </NexButton>
        </form>
      ) : null}

      {error ? <NexText color="danger">{t("global.k_Settings_Roles_Error")}</NexText> : null}
      {loading ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
      {!loading && roles.length === 0 ? (
        <NexText color="muted">{t("global.k_Settings_Roles_Empty")}</NexText>
      ) : null}

      {roles.length ? (
        <div className="role-table-wrap">
          <table className="role-table">
            <thead>
              <tr>
                <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Roles_Label_Title")}</NexText></th>
                <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Roles_Label_Description")}</NexText></th>
                <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Roles_Label_Type")}</NexText></th>
                <th scope="col"><NexText as="span" variant="label">{t("global.k_Settings_Roles_PermissionsTitle")}</NexText></th>
              </tr>
            </thead>
            <tbody>
              {roles.map((role) => (
                <tr key={role.id}>
                  <td>
                    <Link to={encodeURIComponent(role.id)}>
                      <NexText as="span" weight={600} color="primary">{role.title}</NexText>
                    </Link>
                  </td>
                  <td><NexText as="span" color="muted">{role.description || t("global.k_Settings_Roles_NoDescription")}</NexText></td>
                  <td>
                    <NexText as="span" color={role.is_system ? "primary" : "content"}>
                      {role.is_system
                        ? t("global.k_Settings_Roles_SystemBadge")
                        : t("global.k_Settings_Roles_CustomBadge")}
                    </NexText>
                  </td>
                  <td>
                    <NexText as="span" color="muted">
                      {role.grants_all_permissions
                        ? t("global.k_Settings_Roles_AllPermissions")
                        : t("global.k_Settings_Roles_PermissionCount", { count: role.permissions.length })}
                    </NexText>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  );
}
