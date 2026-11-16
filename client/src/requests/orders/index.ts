import { request } from "@/requests/core/client";
import { buildListPath, type ListQuery, type ListResponse } from "@/requests/core/list";
export type DocumentKind = "quote" | "order";
export interface OrderLine { id: string; description: string; inventory_item_id?: string; quantity: string; unit_price: string; fulfilled_quantity?: string; remaining_quantity?: string }
export interface OrderDocument { id: string; kind: string; status: string; customer_name: string; customer_id?: string; quote_date?: string; valid_until?: string; order_date?: string; currency: string; total_amount: string; note?: string; source_quote_id?: string; resulting_order_id?: string; created_at: string; updated_at: string; created_by: string; lines: OrderLine[] }
export interface OrderInput { customer_name: string; customer_id?: string; quote_date?: string; valid_until?: string; order_date?: string; currency: string; note?: string; expected_updated_at?: string; lines: Omit<OrderLine, "id">[] }
export interface ShipmentLine { id?: string; order_line_id: string; description?: string; inventory_item_id?: string; quantity: string; location_id?: string }
export interface Shipment { id: string; status: string; order_id: string; warehouse_id?: string; shipped_date: string; tracking_reference?: string; note?: string; inventory_record_id?: string; created_by: string; created_at: string; lines: ShipmentLine[] }
export interface ShipmentInput { warehouse_id?: string; shipped_date: string; tracking_reference?: string; note?: string; request_key: string; expected_updated_at: string; lines: ShipmentLine[] }
export interface OrderEvent { id: string; actor_user_id: string; action: string; occurred_at: string }
const path = (kind: DocumentKind) => kind === "quote" ? "/api/quotes" : "/api/orders";
export const listOrders = (kind: DocumentKind, query: ListQuery) => request<ListResponse<OrderDocument,"orders"|"quotes">>(buildListPath(path(kind),query));
export const getOrder = (kind: DocumentKind,id: string) => request<OrderDocument>(`${path(kind)}/${encodeURIComponent(id)}`);
export const saveOrder = (kind: DocumentKind,id: string|undefined,input: OrderInput) => request<OrderDocument>(`${path(kind)}${id ? `/${encodeURIComponent(id)}` : ""}`,{method:id?"PUT":"POST",body:JSON.stringify(input)});
export const changeOrderStatus = (kind: DocumentKind,doc: OrderDocument,status: string) => request<OrderDocument>(`${path(kind)}/${encodeURIComponent(doc.id)}/status`,{method:"PATCH",body:JSON.stringify({status,expected_updated_at:doc.updated_at})});
export const listShipments = (id: string,page: number) => request<ListResponse<Shipment,"shipments">>(buildListPath("/api/order-shipments",{parent_id:id,page,page_size:10,sort:"created_at",order:"desc"}));
export const listOrderEvents = (kind: DocumentKind,id: string,page: number) => request<ListResponse<OrderEvent,"events">>(buildListPath(`${path(kind)}/${encodeURIComponent(id)}/events`,{page,page_size:10}));
export const shipOrder = (id: string,input: ShipmentInput) => request<Shipment>(`/api/orders/${encodeURIComponent(id)}/shipments`,{method:"POST",body:JSON.stringify(input)});
export interface ShippingOptions { inventory_available: boolean; warehouses: {id:string;name:string}[]; locations: {id:string;name:string;warehouse_id:string}[] }
export const getShippingOptions = (id: string) => request<ShippingOptions>(`/api/orders/${encodeURIComponent(id)}/shipping-options`);
