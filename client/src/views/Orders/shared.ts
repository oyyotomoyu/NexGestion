import { ApiError } from "@/requests/core/client";
import type { User } from "@/requests/users/types";
export const can = (user: User|null,key: string) => user?.is_protected === true || user?.roles.some(r=>r.grants_all_permissions||r.permissions.some(p=>p.permission_key===key))===true;
export function today() { const date=new Date();return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")}`; }
export function errorKey(error: unknown) { if(error instanceof ApiError){if(error.body?.code==="operations_stale_document")return "Stale";if(error.status===409)return "Conflict";if(error.status===403)return "Forbidden";if(error.status===404)return "NotFound";}return "Error"; }
export { scaledDecimal, draftTotal } from "@/requests/orders/decimal";
