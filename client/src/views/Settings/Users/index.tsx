import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexText } from "@/components/NexText";
import { createUser, listUsers } from "@/requests/users";
import type { User } from "@/requests/users/types";

import "./style.css";

export default function Users() {
  const { t } = useTranslation("ui"); const { user: actor } = useAuth();
  const [users,setUsers]=useState<User[]>([]); const [name,setName]=useState(""); const [email,setEmail]=useState(""); const [password,setPassword]=useState(""); const [query,setQuery]=useState(""); const [loading,setLoading]=useState(true); const [saving,setSaving]=useState(false); const [error,setError]=useState(false);
  const canManage=actor?.is_protected===true||actor?.roles.some((role)=>role.grants_all_permissions||role.permissions.some((permission)=>permission.permission_key==="users.manage"))===true;
  useEffect(()=>{listUsers().then(setUsers).catch(()=>setError(true)).finally(()=>setLoading(false))},[]);
  const visibleUsers=users.filter((user)=>`${user.id} ${user.display_name} ${user.email}`.toLowerCase().includes(query.trim().toLowerCase()));
  async function submit(event:FormEvent){event.preventDefault();if(!name.trim()||!email.trim()||password.length<12)return;setSaving(true);setError(false);try{const created=await createUser({display_name:name,email,password,status:"active",must_change_password:true});setUsers((items)=>[...items,created]);setName("");setEmail("");setPassword("")}catch{setError(true)}finally{setSaving(false)}}
  return <section className="users-page"><header><NexText variant="heading">{t("global.k_Settings_Users_Title")}</NexText><NexText color="muted">{t("global.k_Settings_Users_Description")}</NexText></header>
    {canManage?<form className="user-form" onSubmit={(event)=>void submit(event)}><NexText variant="subheading">{t("global.k_Settings_Users_CreateTitle")}</NexText><div className="user-form__fields"><NexInput id="new-user-name" label={t("global.k_Settings_Users_Name")} value={name} required onChange={(event)=>setName(event.target.value)}/><NexInput id="new-user-email" type="email" label={t("global.k_Settings_Users_Email")} value={email} required onChange={(event)=>setEmail(event.target.value)}/><NexInput id="new-user-password" type="password" minLength={12} label={t("global.k_Settings_Users_Password")} value={password} required onChange={(event)=>setPassword(event.target.value)}/></div><NexButton type="submit" disabled={saving||!name.trim()||!email.trim()||password.length<12}>{saving?t("global.k_Common_Saving"):t("global.k_Settings_Users_Create")}</NexButton></form>:null}
    {error?<NexText color="danger">{t("global.k_Settings_Users_Error")}</NexText>:null}{loading?<NexText color="muted">{t("global.k_Common_Loading")}</NexText>:null}{!loading&&!users.length?<NexText color="muted">{t("global.k_Settings_Users_Empty")}</NexText>:null}
    {users.length?<><NexInput id="user-search" type="search" label={t("global.k_Settings_Users_Search")} value={query} onChange={(event)=>setQuery(event.target.value)}/><div className="user-table-wrap"><table className="user-table"><thead><tr><th>{t("global.k_Settings_Users_ID")}</th><th>{t("global.k_Settings_Users_Name")}</th><th>{t("global.k_Settings_Users_Email")}</th><th>{t("global.k_Settings_Users_Status")}</th><th>{t("global.k_Settings_Users_Roles")}</th><th>{t("global.k_Settings_Users_Groups")}</th></tr></thead><tbody>{visibleUsers.map((user)=><tr key={user.id}><td data-label={t("global.k_Settings_Users_ID")}>{user.id}</td><td data-label={t("global.k_Settings_Users_Name")}><Link to={encodeURIComponent(user.id)}><NexText as="span" color="primary" weight={600}>{user.display_name}</NexText></Link></td><td data-label={t("global.k_Settings_Users_Email")}>{user.email}</td><td data-label={t("global.k_Settings_Users_Status")}>{user.status}</td><td data-label={t("global.k_Settings_Users_Roles")}>{user.roles.map((role)=>role.title).join(", ")||"-"}</td><td data-label={t("global.k_Settings_Users_Groups")}>{user.groups.map((group)=>group.name).join(", ")||"-"}</td></tr>)}</tbody></table></div></>:null}
  </section>
}
