import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { getShippingOptions, shipOrder, type OrderDocument, type ShippingOptions } from "@/requests/orders";
import { errorKey, scaledDecimal, today } from "./shared";

export default function ShipmentForm({document,onSaved,onCancel}:{document:OrderDocument;onSaved:()=>void;onCancel:()=>void}) {
 const {t}=useTranslation("ui");const label=(key:string)=>t(`global.k_Orders_${key}`);
 const [options,setOptions]=useState<ShippingOptions|null>(null),[error,setError]=useState(""),[saving,setSaving]=useState(false);
 const [warehouse,setWarehouse]=useState(""),[date,setDate]=useState(today()),[tracking,setTracking]=useState(""),[note,setNote]=useState("");
 const pending=document.lines.filter(l=>(scaledDecimal(l.remaining_quantity||l.quantity)||0n)>0n);
 const [lines,setLines]=useState(pending.map(l=>({id:l.id,quantity:l.remaining_quantity||l.quantity,location_id:""})));
 const requestKey=useRef(crypto.randomUUID());
 useEffect(()=>{let active=true;getShippingOptions(document.id).then(result=>{if(active){setOptions(result);setWarehouse(result.warehouses[0]?.id||"")}}).catch(e=>{if(active)setError(label(errorKey(e)))});return()=>{active=false}},[document.id,t]);
 const needsStock=options?.inventory_available&&lines.some(l=>document.lines.find(d=>d.id===l.id)?.inventory_item_id&&(scaledDecimal(l.quantity)||0n)>0n);
 const valid=lines.every(line=>{const q=scaledDecimal(line.quantity),remaining=scaledDecimal(pending.find(l=>l.id===line.id)?.remaining_quantity||"0");return q!==null&&remaining!==null&&q<=remaining})&&lines.some(l=>(scaledDecimal(l.quantity)||0n)>0n);
 async function submit(e:FormEvent){e.preventDefault();setSaving(true);setError("");try{await shipOrder(document.id,{expected_updated_at:document.updated_at,request_key:requestKey.current,warehouse_id:warehouse,shipped_date:date,tracking_reference:tracking,note,lines:lines.filter(l=>(scaledDecimal(l.quantity)||0n)>0n).map(l=>({order_line_id:l.id,quantity:l.quantity,location_id:l.location_id}))});onSaved()}catch(e){setError(label(errorKey(e)))}finally{setSaving(false)}}
 return <form className="orders-panel" onSubmit={submit}><NexText variant="heading">{label("RecordShipment")}</NexText><p>{label("ShipmentHelp")}</p>{error&&<div role="alert">{error}</div>}
  {!options&&!error&&<p role="status">{t("global.k_Common_Loading")}</p>}
  <fieldset disabled={saving||!options} className="orders-lines"><div className="orders-form">
   <NexInput label={label("ShippedDate")} required type="date" value={date} onChange={e=>setDate(e.target.value)}/>
   {options?.inventory_available&&<div className="orders-field"><NexText variant="label">{label("Warehouse")}</NexText><NexSelect ariaLabel={label("Warehouse")} value={warehouse} options={[{value:"",label:label("SelectWarehouse")},...options.warehouses.map(w=>({value:w.id,label:w.name}))]} onChange={value=>{setWarehouse(value);setLines(lines.map(l=>({...l,location_id:""})))}}/></div>}
   <NexInput label={label("Tracking")} value={tracking} onChange={e=>setTracking(e.target.value)}/>
  </div>
  {lines.map(line=>{const original=pending.find(l=>l.id===line.id)!;return <div className="orders-line" key={line.id}><div><NexText variant="label">{original.description}</NexText><p>{label("Remaining")}: {original.remaining_quantity}</p></div><NexInput label={label("ShipQuantity")} type="number" min="0" max={original.remaining_quantity} step="0.000001" required value={line.quantity} onChange={e=>setLines(lines.map(l=>l.id===line.id?{...l,quantity:e.target.value}:l))}/>
   {options?.inventory_available&&original.inventory_item_id&&<div className="orders-field"><NexText variant="label">{label("Location")}</NexText><NexSelect ariaLabel={label("Location")} value={line.location_id} options={[{value:"",label:label("Unassigned")},...options.locations.filter(l=>l.warehouse_id===warehouse).map(l=>({value:l.id,label:l.name}))]} onChange={value=>setLines(lines.map(l=>l.id===line.id?{...l,location_id:value}:l))}/></div>}
  </div>})}
  <NexInput label={label("Note")} value={note} onChange={e=>setNote(e.target.value)}/></fieldset>
  <div className="orders-toolbar"><NexButton disabled={saving||!options||!valid||Boolean(needsStock&&!warehouse)}>{t(saving?"global.k_Common_Saving":"global.k_Orders_RecordShipment")}</NexButton><NexButton type="button" variant="secondary" disabled={saving} onClick={onCancel}>{label("Cancel")}</NexButton></div>
 </form>;
}
