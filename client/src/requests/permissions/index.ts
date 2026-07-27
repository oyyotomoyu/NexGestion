import { request } from "@/requests/core/client";
import type { Permission } from "@/requests/users/types";

const mockPermissions:Permission[]=["users.read","users.manage","roles.read","roles.manage","roles.assign","groups.read","groups.manage","groups.assign","permissions.read","permissions.assign","logs.read"].map((key)=>({id:key,permission_key:key,module:key.split(".")[0],description:null}));
export async function listPermissions() { if(import.meta.env.DEV)return mockPermissions;const response=await request<{permissions:Permission[]}>("/api/permissions");return response.permissions; }
export function setRolePermission(roleId:string,permissionId:string,grant:boolean){return request<void>(`/api/roles/${encodeURIComponent(roleId)}/permissions/${encodeURIComponent(permissionId)}`,{method:grant?"PUT":"DELETE"});}
