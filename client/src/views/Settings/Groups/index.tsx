import { useEffect, useMemo, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { createGroup, deleteGroup, listGroups } from "@/requests/groups";
import type { Group } from "@/requests/groups/types";

import "./style.css";

export default function Groups() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canManage = user?.is_protected === true;
  const [groups, setGroups] = useState<Group[]>([]);
  const [name, setName] = useState("");
  const [type, setType] = useState("");
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
    ...groups.map((group) => ({ value: group.id, label: group.name })),
  ], [groups, t]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim() || !type.trim()) return;
    setSaving(true); setError(false);
    try {
      const group = await createGroup({ name, type, parent_group_id: parent || undefined });
      setGroups((items) => [...items, group].sort((a, b) => a.name.localeCompare(b.name)));
      setName(""); setType(""); setParent("");
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
        <NexInput id="group-type" label={t("global.k_Settings_Groups_Label_Type")} value={type} required onChange={(event) => setType(event.target.value)} />
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Parent")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Parent")} value={parent} options={parentOptions} onChange={setParent} /></div>
      </div>
      <NexButton type="submit" disabled={saving || !name.trim() || !type.trim()}>{saving ? t("global.k_Common_Saving") : t("global.k_Settings_Groups_Button_Create")}</NexButton>
    </form> : null}
    {error ? <NexText color="danger">{t("global.k_Settings_Groups_Error")}</NexText> : null}
    {loading ? <NexText color="muted">{t("global.k_Common_Loading")}</NexText> : null}
    {!loading && !groups.length ? <NexText color="muted">{t("global.k_Settings_Groups_Empty")}</NexText> : null}
    {groups.length ? <div className="group-table-wrap"><table className="group-table"><thead><tr>
      <th>{t("global.k_Settings_Groups_Label_Name")}</th><th>{t("global.k_Settings_Groups_Label_Type")}</th><th>{t("global.k_Settings_Groups_Label_Parent")}</th><th>{t("global.k_Settings_Groups_Label_Status")}</th><th>{t("global.k_Settings_Groups_Label_Members")}</th><th>{t("global.k_Common_Actions")}</th>
    </tr></thead><tbody>{groups.map((group) => <tr key={group.id}>
      <td><NexText as="span" weight={600}>{group.name}</NexText></td><td>{group.type}</td><td>{group.parent_group_id ? names.get(group.parent_group_id) || "-" : "-"}</td><td>{group.status}</td><td>{group.member_count}</td>
      <td>{canManage ? <NexButton type="button" variant="danger" size="compact" disabled={saving} onClick={() => void remove(group)}>{t("global.k_Common_Delete")}</NexButton> : null}</td>
    </tr>)}</tbody></table></div> : null}
  </section>;
}
