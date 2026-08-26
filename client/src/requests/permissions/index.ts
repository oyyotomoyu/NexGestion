import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type { Permission } from "@/requests/users/types";

const mockPermissions:Permission[]=[
  "users.access","users.read","users.manage",
  "roles.access","roles.read","roles.manage","roles.assign",
  "permissions.access","permissions.read","permissions.assign",
  "groups.access","groups.read","groups.manage","groups.assign",
  "logs.read",
  "attendance.access","attendance.read.self","attendance.clock.self","attendance.read","attendance.manage","attendance.reports.read",
  "notifications.access","notifications.read","notifications.send.own_group","notifications.send.group","notifications.send.organization","notifications.manage","notifications.export","notifications.type.info","notifications.type.success","notifications.type.warning","notifications.type.important","notifications.type.urgent",
  "templates.access","templates.read","templates.upload","templates.manage",
  "salary.access","salary.read.self","salary.read","salary.settlement.configure",
  "approvals.access","approvals.templates.manage","approvals.read.self","approvals.decide","approvals.read","approvals.reassign",
  "checkout.access","crm.access","finance.access","general_affairs.access",
  "hr.access","hr.employment.read.self","hr.employment.read","hr.employment.manage","hr.tasks.manage","hr.performance.read.self","hr.performance.read","hr.performance.cycles.manage","hr.performance.review","hr.employee_relations.read.self","hr.employee_relations.manage","hr.employee_relations.read",
  "inventory.access","operations.access","orders.access","procurement.access","production.access","scheduling.access",
].map((key)=>({id:key,permission_key:key,module:key.split(".")[0],description:null,high_risk:false,high_risk_reason:null,requires_password:false}));
export async function listPermissions(query:ListQuery={}) { if(import.meta.env.DEV)return mockPermissions;const response=await request<ListResponse<Permission,"permissions">>(buildListPath("/api/permissions",{sort:"permission_key",order:"asc",page_size:100,...query}));return listItems(response,"permissions"); }
export function setRolePermission(roleId:string,permissionId:string,grant:boolean,currentPassword?:string){return request<void>(`/api/roles/${encodeURIComponent(roleId)}/permissions/${encodeURIComponent(permissionId)}`,{method:grant?"PUT":"DELETE",...(currentPassword?{body:JSON.stringify({current_password:currentPassword})}:{})});}
