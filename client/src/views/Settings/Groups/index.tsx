import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { createGroup, deleteGroup, listGroups } from "@/requests/groups";
import type { Group, GroupType, OrganizationLevel } from "@/requests/groups/types";

import "./style.css";

export default function Groups() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canManage = user?.is_protected === true || user?.roles.some((role) => role.grants_all_permissions || role.permissions.some((permission) => permission.permission_key === "groups.manage")) === true;
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [type, setType] = useState<GroupType>("organization");
  const [level, setLevel] = useState<OrganizationLevel>(1);
  const [parent, setParent] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);

  useEffect(() => {
    let active = true;
    listGroups().then((items) => { if (active) setGroups(items); })
      .catch(() => { if (active) setError(true); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, []);

  const parentOptions = useMemo(() => [
    { value: "", label: t("global.k_Settings_Groups_NoParent") },
    ...groups.filter((group) => group.type === "organization" && group.status === "active" && group.organization_level === level - 1)
      .map((group) => ({ value: group.id, label: group.name })),
  ], [groups, level, t]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim() || (type === "organization" && level > 1 && !parent)) return;
    setSaving(true); setError(false);
    try {
      const group = await createGroup({ name, type, organization_level: type === "organization" ? level : undefined, parent_group_id: type === "organization" ? parent || undefined : undefined });
      setGroups((items) => [...items, group].sort((a, b) => a.name.localeCompare(b.name)));
      setName(""); setType("organization"); setLevel(1); setParent("");
    } catch { setError(true); } finally { setSaving(false); }
  }

  async function remove(group: Group) {
    if (!window.confirm(t("global.k_Settings_Groups_DeleteConfirm"))) return;
    setSaving(true); setError(false);
    try { await deleteGroup(group.id); setGroups((items) => items.filter((item) => item.id !== group.id)); }
    catch { setError(true); } finally { setSaving(false); }
  }

  const names = new Map(groups.map((group) => [group.id, group.name]));
  return <section className="groups-page">
    <header><NexText variant="heading">{t("global.k_Settings_Groups_Title")}</NexText><NexText color="muted">{t("global.k_Settings_Groups_Description")}</NexText></header>
    {canManage ? <form className="group-form" onSubmit={(event) => void submit(event)}>
      <NexText variant="subheading">{t("global.k_Settings_Groups_CreateTitle")}</NexText>
      <div className="group-form__fields">
        <NexInput id="group-name" label={t("global.k_Settings_Groups_Label_Name")} value={name} required onChange={(event) => setName(event.target.value)} />
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Type")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Type")} value={type} options={[{value:"organization",label:t("global.k_Settings_Groups_Type_Organization")},{value:"project",label:t("global.k_Settings_Groups_Type_Project")}]} onChange={(value)=>{setType(value);setParent("");}} /></div>
        {type === "organization" ? <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Level")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Level")} value={String(level)} options={[1,2,3,4,5].map((value)=>({value:String(value),label:t("global.k_Settings_Groups_Level",{level:value})}))} onChange={(value)=>{setLevel(Number(value) as OrganizationLevel);setParent("");}} /></div> : null}
        {type === "organization" && level > 1 ? <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Parent")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Parent")} value={parent} options={parentOptions} onChange={setParent} /></div> : null}
      </div>
      <NexButton type="submit" disabled={saving || !name.trim() || (type === "organization" && level > 1 && !parent)}>{saving ? t("global.k_Common_Saving") : t("global.k_Settings_Groups_Button_Create")}</NexButton>
    </form> : null}
    {error ? <NexText color="danger">{t("global.k_Settings_Groups_Error")}</NexText> : null}
    {loading ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
    {!loading && !groups.length ? <NexText color="muted">{t("global.k_Settings_Groups_Empty")}</NexText> : null}
    {groups.length ? <div className="group-table-wrap"><table className="group-table"><thead><tr>
      <th>{t("global.k_Settings_Groups_Label_Name")}</th><th>{t("global.k_Settings_Groups_Label_Type")}</th><th>{t("global.k_Settings_Groups_Label_Level")}</th><th>{t("global.k_Settings_Groups_Label_Parent")}</th><th>{t("global.k_Settings_Groups_Label_Status")}</th><th>{t("global.k_Settings_Groups_Label_Members")}</th><th>{t("global.k_Common_Actions")}</th>
    </tr></thead><tbody>{groups.map((group) => <tr key={group.id}>
      <td data-label={t("global.k_Settings_Groups_Label_Name")}>{canManage || user?.roles.some((role)=>role.id===group.manager_role_id)?<Link to={encodeURIComponent(group.id)}><NexText as="span" weight={600} color="primary">{group.name}</NexText></Link>:<NexText as="span" weight={600}>{group.name}</NexText>}</td><td data-label={t("global.k_Settings_Groups_Label_Type")}>{t(`global.k_Settings_Groups_Type_${group.type === "organization" ? "Organization" : "Project"}`)}</td><td data-label={t("global.k_Settings_Groups_Label_Level")}>{group.organization_level ?? "-"}</td><td data-label={t("global.k_Settings_Groups_Label_Parent")}>{group.parent_group_id ? names.get(group.parent_group_id) || "-" : "-"}</td><td data-label={t("global.k_Settings_Groups_Label_Status")}>{group.status}</td><td data-label={t("global.k_Settings_Groups_Label_Members")}>{group.member_count}</td>
      <td data-label={t("global.k_Common_Actions")}>{canManage ? <NexButton type="button" variant="danger" size="compact" disabled={saving} onClick={() => void remove(group)}>{t("global.k_Common_Delete")}</NexButton> : null}</td>
    </tr>)}</tbody></table></div> : null}
  </section>;
}
