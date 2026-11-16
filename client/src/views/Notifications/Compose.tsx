import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { createNotification, updateNotification, listNotificationTypes } from "@/requests/notifications";
import type { Notification, NotificationType, NotificationAudienceInput, NotificationAudienceScope, NotificationDuration } from "@/requests/notifications/types";
import type { User } from "@/requests/users/types";

export default function Compose({user, editing, onSaved, onCancel}: {user: User|null; editing: Notification|null; onSaved:()=>void; onCancel:()=>void}) {
 const {t}=useTranslation("ui");
 const has=(key:string)=>user?.is_protected===true||user?.roles.some(r=>r.grants_all_permissions||r.permissions.some(p=>p.permission_key===key))===true;
 const scopes: NotificationAudienceScope[]=[];
 if(has("notifications.send.organization"))scopes.push("organization");
 if(has("notifications.send.group"))scopes.push("group");
 if(has("notifications.send.own_group"))scopes.push("own_group");
 if(has("notifications.send.group")||has("notifications.send.organization"))scopes.push("role","user");
 const [types,setTypes]=useState<NotificationType[]>([]),[title,setTitle]=useState(editing?.title||""),[message,setMessage]=useState(editing?.message||"");
 const [type,setType]=useState(editing?.type.code||""),[duration,setDuration]=useState<NotificationDuration>(editing?.duration_code||"day");
 const [audiences,setAudiences]=useState<NotificationAudienceInput[]>(editing?.audiences.map(a=>({scope:a.scope,target_group_id:a.target_group_id||undefined,target_role_id:a.target_role_id||undefined,target_user_id:a.target_user_id||undefined}))||[]);
 const [scope,setScope]=useState<NotificationAudienceScope>(scopes[0]||"organization"),[target,setTarget]=useState("");
 const [busy,setBusy]=useState(false),[error,setError]=useState("");
 useEffect(()=>{let active=true;listNotificationTypes().then(result=>{if(active)setTypes(result)}).catch(()=>{if(active)setError(t("global.k_Notifications_Error"))});return()=>{active=false}},[t]);
 const allowed=types.filter(v=>v.is_active&&(!v.required_permission_key||has(v.required_permission_key)));
 const selectedType=type||allowed[0]?.code||"";
 async function submit(e:FormEvent){e.preventDefault();setBusy(true);setError("");try{const input={title,message,type:selectedType,show_time:duration,audiences};if(editing)await updateNotification(editing.id,input);else await createNotification(input);onSaved()}catch{setError(t("global.k_Notifications_SendError"))}finally{setBusy(false)}}
 function addAudience(){const value=target.trim();const a:NotificationAudienceInput={scope};if(scope==="group"||scope==="own_group")a.target_group_id=value;if(scope==="role")a.target_role_id=value;if(scope==="user")a.target_user_id=value;setAudiences([...audiences,a]);setTarget("")}
 return <form className="notifications-compose" onSubmit={submit}>
  <NexText variant="heading">{t(editing?"global.k_Notifications_Edit":"global.k_Notifications_Compose")}</NexText>
  {error&&<div role="alert">{error}</div>}
  <NexInput label={t("global.k_Notifications_MessageTitle")} required value={title} onChange={e=>setTitle(e.target.value)} />
  <label className="notifications-compose__message">{t("global.k_Notifications_Message")}<textarea required rows={4} value={message} onChange={e=>setMessage(e.target.value)}/></label>
  <NexSelect ariaLabel={t("global.k_Notifications_Type")} value={selectedType} onChange={setType} options={allowed.map(v=>({value:v.code,label:v.name}))}/>
  <NexSelect ariaLabel={t("global.k_Notifications_Duration")} value={duration} onChange={setDuration} options={(["hour","day","week","month","year","forever"] as NotificationDuration[]).map(value=>({value,label:t(`global.k_Notifications_Duration_${value}`)}))}/>
  <NexSelect ariaLabel={t("global.k_Notifications_Audience")} value={scope} onChange={setScope} options={scopes.map(value=>({value,label:t(`global.k_Notifications_Scope_${value}`)}))}/>
  {scope!=="organization"&&<NexInput label={t("global.k_Notifications_TargetID")} value={target} onChange={e=>setTarget(e.target.value)}/>}
  <NexButton type="button" variant="secondary" disabled={busy||!scopes.length||(scope!=="organization"&&!target.trim())} onClick={addAudience}>{t("global.k_Notifications_AddAudience")}</NexButton>
  {audiences.map((a,i)=><div className="notifications-compose__audience" key={i}><span>{t(`global.k_Notifications_Scope_${a.scope}`)} {a.target_group_id||a.target_role_id||a.target_user_id}</span><NexButton type="button" variant="secondary" disabled={busy} onClick={()=>setAudiences(audiences.filter((_,index)=>index!==i))}>{t("global.k_Notifications_Remove")}</NexButton></div>)}
  <div className="notifications-toolbar"><NexButton disabled={busy||!audiences.length||!allowed.some(v=>v.code===selectedType)}>{t(busy?"global.k_Common_Saving":editing?"global.k_Notifications_Save":"global.k_Notifications_Send")}</NexButton><NexButton type="button" variant="secondary" disabled={busy} onClick={onCancel}>{t("global.k_Notifications_Cancel")}</NexButton></div>
 </form>;
}
