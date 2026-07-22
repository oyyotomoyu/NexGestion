import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/auth/AuthProvider";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { getGroup, listGroupMembers, listGroups, removeGroupMember, setGroupMember, updateGroup } from "@/requests/groups";
import { listPermissions, setGroupPermission } from "@/requests/permissions";
import type { Permission } from "@/requests/users/types";
import type { Group, GroupMember, GroupMemberRole } from "@/requests/groups/types";
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
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);
  const [name,setName]=useState(""); const [type,setType]=useState(""); const [status,setStatus]=useState<"active"|"inactive">("active"); const [permissions,setPermissions]=useState<Permission[]>([]);
  const [parent,setParent]=useState(""); const [allGroups,setAllGroups]=useState<Group[]>([]);
  const hasPermission=(key:string)=>user?.is_protected===true||user?.roles.some((item)=>item.grants_all_permissions||item.permissions.some((permission)=>permission.permission_key===key))===true;
  const canEdit=hasPermission("groups.manage"); const canAssignPermissions=hasPermission("permissions.assign");
  const canManageMembers=hasPermission("groups.assign")||user?.roles.some((item)=>item.id===group?.manager_role_id)===true;

  useEffect(() => {
    getGroup(groupId).then(async(groupResult)=>{setGroup(groupResult);setName(groupResult.name);setType(groupResult.type);setStatus(groupResult.status);setParent(groupResult.parent_group_id||"");const manageMembers=hasPermission("groups.assign")||user?.roles.some((item)=>item.id===groupResult.manager_role_id)===true;const [userResult,catalog,memberResult,groupsResult]=await Promise.all([listUsers(),listPermissions(),manageMembers?listGroupMembers(groupId):Promise.resolve([]),listGroups()]);setUsers(userResult);setPermissions(catalog);setMembers(memberResult);setAllGroups(groupsResult)}).catch(() => setError(true));
  }, [groupId,user]);

  const candidates = useMemo(() => users.filter((user) => !members.some((member) => member.user_id === user.id)), [members, users]);
  const userOptions = [{ value: "", label: t("global.k_Settings_Groups_SelectUser") }, ...candidates.map((user) => ({ value: user.id, label: `${user.display_name} (${user.email})` }))];
  const roleOptions = [
    { value: "member" as const, label: t("global.k_Settings_Groups_Member") },
    { value: "manager" as const, label: t("global.k_Settings_Groups_Manager") },
  ];
  const parentOptions=[{value:"",label:t("global.k_Settings_Groups_NoParent")},...allGroups.filter((item)=>item.id!==groupId).map((item)=>({value:item.id,label:item.name}))];

  async function submit(event: FormEvent) {
    event.preventDefault(); if (!userId) return; setSaving(true); setError(false);
    try { const member = await setGroupMember(groupId, userId, { role, title }); setMembers((items) => [...items, member]); setUserId(""); setTitle(""); }
    catch { setError(true); } finally { setSaving(false); }
  }

  async function changeRole(member: GroupMember, nextRole: GroupMemberRole) {
    setSaving(true); setError(false);
    try { const updated = await setGroupMember(groupId, member.user_id, { role: nextRole, title: member.title || undefined, joined_at: member.joined_at || undefined }); setMembers((items) => items.map((item) => item.user_id === updated.user_id ? updated : item)); }
    catch { setError(true); } finally { setSaving(false); }
  }

  async function remove(member: GroupMember) {
    if (!window.confirm(t("global.k_Settings_Groups_RemoveMemberConfirm"))) return;
    setSaving(true); setError(false);
    try { await removeGroupMember(groupId, member.user_id); setMembers((items) => items.filter((item) => item.user_id !== member.user_id)); }
    catch { setError(true); } finally { setSaving(false); }
  }
  async function saveGroup(event:FormEvent){event.preventDefault();setSaving(true);setError(false);try{const updated=await updateGroup(groupId,{name,type,status,parent_group_id:parent});setGroup(updated)}catch{setError(true)}finally{setSaving(false)}}
  async function togglePermission(permission:Permission,grant:boolean){setSaving(true);try{await setGroupPermission(groupId,permission.id,grant);setGroup(await getGroup(groupId))}catch{setError(true)}finally{setSaving(false)}}

  return <section className="groups-page">
    <Link to="..">{t("global.k_Settings_Groups_Back")}</Link>
    <header><NexText variant="heading">{group?.name || t("global.k_Settings_Groups_Title")}</NexText><NexText color="muted">{t("global.k_Settings_Groups_MembersDescription")}</NexText></header>
    {error ? <NexText color="danger">{t("global.k_Settings_Groups_Error")}</NexText> : null}
    {canEdit?<form className="group-form" onSubmit={(event)=>void saveGroup(event)}><NexText variant="subheading">{t("global.k_Settings_Groups_EditTitle")}</NexText><div className="group-form__fields"><NexInput id="edit-group-name" label={t("global.k_Settings_Groups_Label_Name")} value={name} required onChange={(event)=>setName(event.target.value)}/><NexInput id="edit-group-type" label={t("global.k_Settings_Groups_Label_Type")} value={type} required onChange={(event)=>setType(event.target.value)}/><div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Parent")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Parent")} value={parent} options={parentOptions} onChange={setParent}/></div><div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_Status")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_Status")} value={status} options={[{value:"active",label:"active"},{value:"inactive",label:"inactive"}]} onChange={setStatus}/></div></div><NexButton type="submit" disabled={saving||!name.trim()||!type.trim()}>{t("global.k_Common_Save")}</NexButton></form>:null}
    {canManageMembers?<form className="group-form" onSubmit={(event) => void submit(event)}>
      <NexText variant="subheading">{t("global.k_Settings_Groups_AddMember")}</NexText>
      <div className="group-form__fields">
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_User")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_User")} value={userId} options={userOptions} onChange={setUserId} /></div>
        <div><NexText as="span" variant="label">{t("global.k_Settings_Groups_Label_GroupRole")}</NexText><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_GroupRole")} value={role} options={roleOptions} onChange={setRole} /></div>
        <NexInput id="member-title" label={t("global.k_Settings_Groups_Label_MemberTitle")} value={title} onChange={(event) => setTitle(event.target.value)} />
      </div>
      <NexButton type="submit" disabled={saving || !userId}>{t("global.k_Settings_Groups_AddMember")}</NexButton>
    </form>:null}
    {canManageMembers?<div className="group-table-wrap"><table className="group-table"><thead><tr><th>{t("global.k_Settings_Groups_Label_User")}</th><th>{t("global.k_Settings_Groups_Label_GroupRole")}</th><th>{t("global.k_Settings_Groups_Label_MemberTitle")}</th><th>{t("global.k_Common_Actions")}</th></tr></thead>
      <tbody>{members.map((member) => <tr key={member.user_id}><td>{member.display_name}<br/><NexText as="span" variant="caption" color="muted">{member.email}</NexText></td><td><NexSelect ariaLabel={t("global.k_Settings_Groups_Label_GroupRole")} value={member.role} options={roleOptions} onChange={(next) => void changeRole(member, next)} /></td><td>{member.title || "-"}</td><td><NexButton type="button" variant="danger" size="compact" disabled={saving} onClick={() => void remove(member)}>{t("global.k_Common_Delete")}</NexButton></td></tr>)}</tbody>
    </table></div>:null}
    <section className="group-form"><NexText variant="subheading">{t("global.k_Settings_Roles_PermissionsTitle")}</NexText>{group?.permissions.map((permission)=><NexText key={permission.id}>{permission.permission_key}</NexText>)}{canAssignPermissions?permissions.map((permission)=>{const granted=group?.permissions.some((item)=>item.id===permission.id)===true;return <label key={permission.id}><input type="checkbox" checked={granted} disabled={saving||!hasPermission(permission.permission_key)} onChange={(event)=>void togglePermission(permission,event.target.checked)}/> {permission.permission_key}</label>}):null}</section>
  </section>;
}
