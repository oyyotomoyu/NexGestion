import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { getOrder, changeOrderStatus, listOrderEvents, listShipments, type DocumentKind, type OrderDocument, type OrderEvent, type Shipment } from "@/requests/orders";
import type { Pagination } from "@/requests/core/list";
import OrderEditor from "./Editor";
import ShipmentForm from "./ShipmentForm";
import { can, errorKey } from "./shared";
import "./style.css";

export default function OrderDetail({kind}:{kind:DocumentKind}) {
 const {t}=useTranslation("ui");const label=(key:string)=>t(`global.k_Orders_${key}`);const {user}=useAuth();const navigate=useNavigate();const {id="new"}=useParams();const root=kind==="quote"?"/quotes":"/orders";const isNew=id==="new";const canManage=can(user,"orders.manage");
 const [document,setDocument]=useState<OrderDocument|null>(null),[loading,setLoading]=useState(!isNew),[error,setError]=useState(""),[busy,setBusy]=useState(false),[editing,setEditing]=useState(false),[shipping,setShipping]=useState(false),[revision,setRevision]=useState(0);
 const [events,setEvents]=useState<OrderEvent[]>([]),[eventPage,setEventPage]=useState(1),[eventPagination,setEventPagination]=useState<Pagination|null>(null);
 const [shipments,setShipments]=useState<Shipment[]>([]),[shipmentPage,setShipmentPage]=useState(1),[shipmentPagination,setShipmentPagination]=useState<Pagination|null>(null);
 useDocumentTitle(label(kind==="quote"?"Quote":"Order"));
 useEffect(()=>{setEditing(false);setShipping(false);setEventPage(1);setShipmentPage(1);setEvents([]);setShipments([]);setEventPagination(null);setShipmentPagination(null)},[id,kind]);
 useEffect(()=>{if(isNew){setDocument(null);setLoading(false);return}let active=true;setLoading(true);setDocument(null);setError("");getOrder(kind,id).then(d=>{if(active)setDocument(d)}).catch(e=>{if(active)setError(label(errorKey(e)))}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[kind,id,revision,t]);
 useEffect(()=>{if(isNew)return;let active=true;listOrderEvents(kind,id,eventPage).then(response=>{if(active){setEvents(response.events as OrderEvent[]);setEventPagination(response.pagination)}}).catch(e=>{if(active)setError(label(errorKey(e)))});return()=>{active=false}},[kind,id,eventPage,revision,t]);
 useEffect(()=>{if(isNew||kind!=="order")return;let active=true;listShipments(id,shipmentPage).then(response=>{if(active){setShipments(response.shipments as Shipment[]);setShipmentPagination(response.pagination)}}).catch(e=>{if(active)setError(label(errorKey(e)))});return()=>{active=false}},[kind,id,shipmentPage,revision,t]);
 function refresh(){setEditing(false);setShipping(false);setEventPage(1);setShipmentPage(1);setRevision(v=>v+1)}
 async function transition(status:string){if(!document)return;setBusy(true);setError("");try{await changeOrderStatus(kind,document,status);refresh()}catch(e){setError(label(errorKey(e)))}finally{setBusy(false)}}
 function saved(d:OrderDocument){setEditing(false);if(isNew)navigate(`${root}/${d.id}`,{replace:true});else refresh()}
 function pager(page:number,pagination:Pagination|null,setPage:(p:number)=>void){return <div className="orders-toolbar"><NexButton variant="secondary" disabled={page<=1} onClick={()=>setPage(page-1)}>{label("Previous")}</NexButton><span>{page} / {Math.max(1,pagination?.total_pages||1)}</span><NexButton variant="secondary" disabled={page>=(pagination?.total_pages||0)} onClick={()=>setPage(page+1)}>{label("Next")}</NexButton></div>}
 const actions=document?.status==="draft"?(kind==="quote"?["sent"]:["confirmed","cancelled"]):document?.status==="sent"&&kind==="quote"?["accepted","rejected"]:document?.status==="confirmed"&&kind==="order"&&!shipments.length&&shipmentPagination?.total===0?["cancelled"]:[];
 return <section className="orders-page"><header className="orders-header"><NexText variant="title">{label(kind==="quote"?"Quote":"Order")}</NexText><Link className="orders-link" to={root}>{label("Back")}</Link></header>
  {error&&<div className="orders-panel" role="alert">{error}{!isNew&&<NexButton variant="secondary" onClick={refresh}>{label("Reload")}</NexButton>}</div>}
  {loading&&<p role="status">{t("global.k_Common_Loading")}</p>}
  {isNew ? canManage?<OrderEditor key={`${kind}-new`} kind={kind} document={null} onSaved={saved} onCancel={()=>navigate(root)}/>:<p>{label("Forbidden")}</p> : !loading&&document&&document.id===id&&<>
   <section className="orders-panel"><div className="orders-header"><div><NexText variant="heading">{document.customer_name}</NexText><p>{label(`Status_${document.status}`)} · {document.quote_date||document.order_date}</p><p className="orders-reference">{document.id}</p></div><NexText variant="heading">{document.total_amount} {document.currency}</NexText></div>
    {document.valid_until&&<p>{label("ValidUntil")}: {document.valid_until}</p>}{document.note&&<p>{document.note}</p>}
    {document.source_quote_id&&<Link to={`/quotes/${document.source_quote_id}`}>{label("SourceQuote")}</Link>}{document.resulting_order_id&&<Link to={`/orders/${document.resulting_order_id}`}>{label("ResultingOrder")}</Link>}
    <div className="orders-toolbar">{canManage&&document.status==="draft"&&<NexButton variant="secondary" disabled={busy} onClick={()=>{setEditing(true);setShipping(false)}}>{label("EditDraft")}</NexButton>}{canManage&&actions.map(status=><NexButton key={status} variant={status==="cancelled"?"danger":"primary"} disabled={busy} onClick={()=>void transition(status)}>{label(`Action_${status}`)}</NexButton>)}{canManage&&document.status==="confirmed"&&kind==="order"&&<NexButton disabled={busy} onClick={()=>{setShipping(true);setEditing(false)}}>{label("RecordShipment")}</NexButton>}</div>
   </section>
   {editing&&<OrderEditor key={document.updated_at} kind={kind} document={document} onSaved={saved} onCancel={()=>setEditing(false)}/>}
   {shipping&&<ShipmentForm key={document.updated_at} document={document} onSaved={refresh} onCancel={()=>setShipping(false)}/>}
   <section className="orders-panel"><NexText variant="heading">{label("Lines")}</NexText><div className="orders-table"><table><thead><tr><th>{label("Description")}</th><th>{label("Quantity")}</th><th>{label("UnitPrice")}</th>{kind==="order"&&<><th>{label("Shipped")}</th><th>{label("Remaining")}</th></>}</tr></thead><tbody>{document.lines.map(line=><tr key={line.id}><td>{line.description}{line.inventory_item_id&&<span className="orders-stock-label"> · {label("LinkedItem")}</span>}</td><td>{line.quantity}</td><td>{line.unit_price}</td>{kind==="order"&&<><td>{line.fulfilled_quantity||"0"}</td><td>{line.remaining_quantity||line.quantity}</td></>}</tr>)}</tbody></table></div></section>
   {kind==="order"&&<section className="orders-panel"><NexText variant="heading">{label("Shipments")}</NexText>{!shipments.length&&<p>{label("NoShipments")}</p>}{shipments.map(shipment=><article className="orders-shipment" key={shipment.id}><NexText variant="subheading">{shipment.shipped_date} · {label(`Status_${shipment.status}`)}</NexText><p className="orders-reference">{shipment.id}</p>{shipment.tracking_reference&&<p>{label("Tracking")}: {shipment.tracking_reference}</p>}{shipment.note&&<p>{shipment.note}</p>}{shipment.lines.map((line,i)=><p key={line.id||i}>{line.description} · {line.quantity}</p>)}</article>)}{pager(shipmentPage,shipmentPagination,setShipmentPage)}</section>}
   <section className="orders-panel"><NexText variant="heading">{label("History")}</NexText>{events.map(event=><div className="orders-event" key={event.id}><span>{event.action.startsWith("status:")?label(`Status_${event.action.slice(7)}`):label(`Event_${event.action}`)}</span><time dateTime={event.occurred_at}>{new Date(event.occurred_at).toLocaleString()}</time><span className="orders-reference">{event.actor_user_id}</span></div>)}{pager(eventPage,eventPagination,setEventPage)}</section>
  </>}
 </section>;
}
