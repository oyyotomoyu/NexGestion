import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/auth/AuthProvider";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { getGroup, listGroupMembers, listGroups, removeGroupMember, setGroupMember, updateGroup } from "@/requests/groups";
import type { Group, GroupMember, GroupMemberRole, GroupType, OrganizationLevel } from "@/requests/groups/types";
import { listUsers } from "@/requests/users";
import type { User } from "@/requests/users/types";

export default function GroupDetail() {
  const { groupId = "" } = useParams();
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<GroupMemberRole>("member");
  const [title, setTitle] = useState("");
  const [isPrimaryOrganization, setIsPrimaryOrganization] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);
  const [name,setName]=useState(""); const [type,setType]=useState<GroupType>("organization"); const [level,setLevel]=useState<OrganizationLevel>(1); const [status,setStatus]=useState<"active"|"inactive">("active");
  const [parent,setParent]=useState(""); const [allGroups,setAllGroups]=useState<Group[]>([]);
  const hasPermission=(key:string)=>user?.is_protected===true||user?.roles.some((item)=>item.grants_all_permissions||item.permissions.some((permission)=>permission.permission_key===key))===true;
  const canEdit=hasPermission("groups.manage");
  const canManageMembers=hasPermission("groups.assign");

  useEffect(() => {
    getGroup(groupId).then(async(groupResult)=>{setGroup(groupResult);setName(groupResult.name);setType(groupResult.type);setLevel(groupResult.organization_level||1);setStatus(groupResult.status);setParent(groupResult.parent_group_id||"");const manageMembers=hasPermission("groups.assign");const [userResult,memberResult,groupsResult]=await Promise.all([listUsers(),manageMembers?listGroupMembers(groupId):Promise.resolve([]),listGroups()]);setUsers(userResult);setMembers(memberResult);setAllGroups(groupsResult)}).catch(() => setError(true));
  }, [groupId,user]);

  const candidates = useMemo(() => users.filter((user) => !members.some((member) => member.user_id === user.id)), [members, users]);
  const userOptions = [{ value: "", label: t("global.k_Settings_Groups_SelectUser") }, ...candidates.map((user) => ({ value: user.id, label: `${user.display_name} (${user.email})` }))];
  const roleOptions = [
    { value: "member" as const, label: t("global.k_Settings_Groups_Member") },
    { value: "manager" as const, label: t("global.k_Settings_Groups_Manager") },
  ];
  const parentOptions=[{value:"",label:t("global.k_Settings_Groups_NoParent")},...allGroups.filter((item)=>item.id!==groupId&&item.type==="organization"&&item.status==="active"&&item.organization_level===level-1).map((item)=>({value:item.id,label:item.name}))];

  async function submit(event: FormEvent) {
    event.preventDefault(); if (!userId) return; setSaving(true); setError(false);
    try { const member = await setGroupMember(groupId, userId, { role, title, is_primary_organization: isPrimaryOrganization }); setMembers((items) => [...items, member]); setUserId(""); setTitle(""); setIsPrimaryOrganization(false); }
    catch { setError(true); } finally { setSaving(false); }
  }

  async function changeRole(member: GroupMember, nextRole: GroupMemberRole) {
    setSaving(true); setError(false);
    try { const updated = await setGroupMember(groupId, member.user_id, { role: nextRole, title: member.title || undefined, joined_at: member.joined_at || undefined, is_primary_organization: member.is_primary_organization }); setMembers((items) => items.map((item) => item.user_id === updated.user_id ? updated : item)); }
    catch { setError(true); } finally { setSaving(false); }
  }

  async function makePrimary(member: GroupMember) {
    setSaving(true); setError(false);
    try {
      const updated = await setGroupMember(groupId, member.user_id, {
        role: member.role, title: member.title || undefined,
        joined_at: member.joined_at || undefined, is_primary_organization: true,
      });
      setMembers((items) => items.map((item) => item.user_id === updated.user_id ? updated : item));
    } catch { setError(true); } finally { setSaving(false); }
  }

  async function remove(member: GroupMember) {
    if (!window.confirm(t("global.k_Settings_Groups_RemoveMemberConfirm"))) return;
    setSaving(true); setError(false);
    try { await removeGroupMember(groupId, member.user_id); setMembers((items) => items.filter((item) => item.user_id !== member.user_id)); }
    catch { setError(true); } finally { setSaving(false); }
  }
  async function saveGroup(event:FormEvent){event.preventDefault();setSaving(true);setError(false);try{const updated=await updateGroup(groupId,{name,type,status,organization_level:type==="organization"?level:undefined,parent_group_id:type==="organization"?parent:""});setGroup(updated)}catch{setError(true)}finally{setSaving(false)}}

  return <section className="groups-page">
    <Link to="..">{t("global.k_Settings_Groups_Back")}</Link>
    <header><NexText variant="heading">{group?.name || t("global.k_Settings_Groups_Title")}</NexText><NexText color="muted">{t("global.k_Settings_Groups_MembersDescription")}</NexText></header>
    {error ? <NexText color="danger">{t("global.k_Settings_Groups_Error")}</NexText> : null}
    {canEdit?<form className="group-form" onSubmit={(event)=>void saveGroup(event)}><NexText variant="subheading">{t("global.k_Settings_Groups_EditTitle")}</NexText><div className="group-form__fields"><NexInput id="edit-group-name" label={t("global.k_Settings_Groups_Label_Name")} value={name} required onChange={(event)=>setName(event.target.value)}/><div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Type")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Type")} value={type} options={[{value:"organization",label:t("global.k_Settings_Groups_Type_Organization")},{value:"project",label:t("global.k_Settings_Groups_Type_Project")}]} onChange={(value)=>{setType(value);setParent("");}}/></div>{type==="organization"?<div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Level")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Level")} value={String(level)} options={[1,2,3,4,5].map((value)=>({value:String(value),label:t("global.k_Settings_Groups_Level",{level:value})}))} onChange={(value)=>{setLevel(Number(value) as OrganizationLevel);setParent("");}}/></div>:null}{type==="organization"&&level>1?<div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Parent")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Parent")} value={parent} options={parentOptions} onChange={setParent}/></div>:null}<div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Status")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Status")} value={status} options={[{value:"active",label:"active"},{value:"inactive",label:"inactive"}]} onChange={setStatus}/></div></div><NexButton type="submit" disabled={saving||!name.trim()||(type==="organization"&&level>1&&!parent)}>{t("global.k_Common_Save")}</NexButton></form>:null}
    {canManageMembers?<form className="group-form" onSubmit={(event) => void submit(event)}>
      <NexText variant="subheading">{t("global.k_Settings_Groups_AddMember")}</NexText>
      <div className="group-form__fields">
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_User")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_User")} value={userId} options={userOptions} onChange={setUserId} /></div>
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_GroupRole")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_GroupRole")} value={role} options={roleOptions} onChange={setRole} /></div>
        <NexInput id="member-title" label={t("global.k_Settings_Groups_Label_MemberTitle")} value={title} onChange={(event) => setTitle(event.target.value)} />
        {group?.type === "organization" ? <NexInput id="member-primary-organization" label={t("global.k_Settings_Groups_PrimaryOrganization")} type="checkbox" checked={isPrimaryOrganization} onChange={(event)=>setIsPrimaryOrganization(event.target.checked)} /> : null}
      </div>
      <NexButton type="submit" disabled={saving || !userId}>{t("global.k_Settings_Groups_AddMember")}</NexButton>
    </form>:null}
    {canManageMembers?<div className="group-table-wrap"><table className="group-table"><thead><tr><th>{t("global.k_Settings_Groups_Label_User")}</th><th>{t("global.k_Settings_Groups_Label_GroupRole")}</th><th>{t("global.k_Settings_Groups_Label_MemberTitle")}</th><th>{t("global.k_Settings_Groups_PrimaryOrganization")}</th><th>{t("global.k_Common_Actions")}</th></tr></thead>
      <tbody>{members.map((member) => <tr key={member.user_id}><td data-label={t("global.k_Settings_Groups_Label_User")}>{member.display_name}<br/><NexText as="span" variant="caption" color="muted">{member.email}</NexText></td><td data-label={t("global.k_Settings_Groups_Label_GroupRole")}><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_GroupRole")} value={member.role} options={roleOptions} onChange={(next) => void changeRole(member, next)} /></td><td data-label={t("global.k_Settings_Groups_Label_MemberTitle")}>{member.title || "-"}</td><td data-label={t("global.k_Settings_Groups_PrimaryOrganization")}>{member.is_primary_organization?t("global.k_Dashboard_Value_True"):group?.type==="organization"?<NexButton type="button" variant="secondary" size="compact" disabled={saving} onClick={()=>void makePrimary(member)}>{t("global.k_Settings_Groups_MakePrimary")}</NexButton>:"-"}</td><td data-label={t("global.k_Common_Actions")}><NexButton type="button" variant="danger" size="compact" disabled={saving} onClick={() => void remove(member)}>{t("global.k_Common_Delete")}</NexButton></td></tr>)}</tbody>
    </table></div>:null}
  </section>;
}
