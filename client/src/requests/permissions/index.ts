import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type { Permission } from "@/requests/users/types";

const mockPermissions:Permission[]=["users.read","users.manage","roles.read","roles.manage","roles.assign","groups.read","groups.manage","groups.assign","permissions.read","permissions.assign","logs.read","attendance.read.self","attendance.clock.self","attendance.read","attendance.manage","attendance.reports.read"].map((key)=>({id:key,permission_key:key,module:key.split(".")[0],description:null}));
export async function listPermissions(query:ListQuery={}) { if(import.meta.env.DEV)return mockPermissions;const response=await request<ListResponse<Permission,"permissions">>(buildListPath("/api/permissions",{sort:"permission_key",order:"asc",page_size:100,...query}));return listItems(response,"permissions"); }
export function setRolePermission(roleId:string,permissionId:string,grant:boolean){return request<void>(`/api/roles/${encodeURIComponent(roleId)}/permissions/${encodeURIComponent(permissionId)}`,{method:grant?"PUT":"DELETE"});}
