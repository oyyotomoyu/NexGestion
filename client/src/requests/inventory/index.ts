import { request } from "@/requests/core/client";
export interface Entity { id: string; name: string; address: string; status: string; warehouse_id: string; sku: string; unit_of_measure: string; barcode: string }
export interface Line { item_id: string; location_id: string; quantity: string }
export interface Stock { item_id: string; warehouse_id: string; location_id: string; quantity: string }
export interface Record { id: string; kind: string; status: string; warehouse_id: string; to_warehouse_id: string; to_location_id: string; date: string; note: string; tracking_reference: string; created_at: string; created_by: string; lines: Line[] }
const root = "/api/inventory";
export const listEntities = (kind: string) => request<Entity[]>(`${root}/${kind}`);
export const saveEntity = (kind: string, value: Partial<Entity>) => request<Entity>(`${root}/${kind}${value.id ? `/${encodeURIComponent(value.id)}` : ""}`, {method: value.id ? "PUT" : "POST", body: JSON.stringify(value)});
export const getStock = () => request<Stock[]>(`${root}/stock`);
export const getRecords = () => request<Record[]>(`${root}/records`);
export const scanItem = (code: string) => request<Entity>(`${root}/scan?code=${encodeURIComponent(code)}`);
export const postRecord = (value: Partial<Record>) => request<Record>(`${root}/records`, {method: "POST", body: JSON.stringify(value)});
export const transition = (id: string, status: string) => request<Record>(`${root}/records/${encodeURIComponent(id)}`, {method: "PATCH", body: JSON.stringify({status})});
