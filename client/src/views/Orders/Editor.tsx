import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { listEntities, type Entity } from "@/requests/inventory";
import { saveOrder, type OrderDocument, type DocumentKind } from "@/requests/orders";
import { can, draftTotal, errorKey, today } from "./shared";

export default function OrderEditor({kind,document,onSaved,onCancel}:{kind:DocumentKind;document:OrderDocument|null;onSaved:(d:OrderDocument)=>void;onCancel:()=>void}) {
 const {t}=useTranslation("ui");const label=(key:string)=>t(`global.k_Orders_${key}`);const {user}=useAuth();
 const [customer,setCustomer]=useState(document?.customer_name||""),[date,setDate]=useState(document?.quote_date||document?.order_date||today()),[validUntil,setValidUntil]=useState(document?.valid_until||""),[currency,setCurrency]=useState(document?.currency||"TWD"),[note,setNote]=useState(document?.note||"");
 const [lines,setLines]=useState(document?.lines.map(l=>({...l,key:crypto.randomUUID()}))||[{key:crypto.randomUUID(),id:"",description:"",quantity:"1",unit_price:"0",inventory_item_id:""}]);
 const [inventory,setInventory]=useState<Entity[]>([]),[inventoryError,setInventoryError]=useState(false),[error,setError]=useState(""),[saving,setSaving]=useState(false);
 const canReadInventory=can(user,"inventory.read");
 useEffect(()=>{if(!canReadInventory)return;let active=true;listEntities("items").then(result=>{if(active)setInventory(result)}).catch(()=>{if(active)setInventoryError(true)});return()=>{active=false}},[canReadInventory]);
 const editLine=(key:string,field:"description"|"quantity"|"unit_price"|"inventory_item_id",value:string)=>setLines(previous=>previous.map(line=>line.key===key?{...line,[field]:value}:line));
 async function submit(e:FormEvent){e.preventDefault();setSaving(true);setError("");try{const result=await saveOrder(kind,document?.id,{customer_name:customer.trim(),customer_id:document?.customer_id,currency,note,expected_updated_at:document?.updated_at,...(kind==="quote"?{quote_date:date,valid_until:validUntil}:{order_date:date}),lines:lines.map(l=>({description:l.description.trim(),quantity:l.quantity,unit_price:l.unit_price,inventory_item_id:l.inventory_item_id}))});onSaved(result)}catch(e){setError(label(errorKey(e)))}finally{setSaving(false)}}
 return <form className="orders-panel" onSubmit={submit}><NexText variant="heading">{label(document?"EditDraft":kind==="quote"?"NewQuote":"NewOrder")}</NexText>{error&&<div role="alert">{error}</div>}
  <div className="orders-form"><NexInput label={label("Customer")} required value={customer} onChange={e=>setCustomer(e.target.value)}/><NexInput label={label("Date")} type="date" required value={date} onChange={e=>setDate(e.target.value)}/>{kind==="quote"&&<NexInput label={label("ValidUntil")} type="date" min={date} value={validUntil} onChange={e=>setValidUntil(e.target.value)}/>}<NexInput label={label("Currency")} required pattern="[A-Z]{3}" maxLength={3} value={currency} onChange={e=>setCurrency(e.target.value.toUpperCase())}/></div>
  {inventoryError&&<p role="status">{label("InventoryUnavailable")}</p>}
  <fieldset disabled={saving} className="orders-lines"><legend>{label("Lines")}</legend>{lines.map(line=><div className="orders-line" key={line.key}><NexInput label={label("Description")} required value={line.description} onChange={e=>editLine(line.key,"description",e.target.value)}/><NexInput label={label("Quantity")} type="number" step="0.000001" min="0.000001" required value={line.quantity} onChange={e=>editLine(line.key,"quantity",e.target.value)}/><NexInput label={label("UnitPrice")} type="number" step="0.000001" min="0" required value={line.unit_price} onChange={e=>editLine(line.key,"unit_price",e.target.value)}/>
   {canReadInventory&&<div className="orders-field"><NexText variant="label">{label("InventoryItem")}</NexText><NexSelect ariaLabel={label("InventoryItem")} value={line.inventory_item_id||""} onChange={value=>editLine(line.key,"inventory_item_id",value)} options={[{value:"",label:label("NoStockLink")},...inventory.map(i=>({value:i.id,label:`${i.sku} · ${i.name}`})),...(line.inventory_item_id&&!inventory.some(i=>i.id===line.inventory_item_id)?[{value:line.inventory_item_id,label:label("LinkedItem")}]:[])]}/></div>}
   {!canReadInventory&&line.inventory_item_id&&<span>{label("LinkedItem")}</span>}
   <NexButton type="button" variant="secondary" disabled={lines.length===1} onClick={()=>setLines(lines.filter(l=>l.key!==line.key))}>{label("Remove")}</NexButton>
  </div>)}</fieldset>
  <NexButton type="button" variant="secondary" disabled={saving||lines.length>=1000} onClick={()=>setLines([...lines,{key:crypto.randomUUID(),id:"",description:"",quantity:"1",unit_price:"0",inventory_item_id:""}])}>{label("AddLine")}</NexButton>
  <NexInput label={label("Note")} value={note} onChange={e=>setNote(e.target.value)}/>
  <NexText variant="heading">{label("Total")}: {draftTotal(lines)} {currency}</NexText>
  <div className="orders-toolbar"><NexButton disabled={saving}>{t(saving?"global.k_Common_Saving":"global.k_Orders_SaveDraft")}</NexButton><NexButton type="button" variant="secondary" disabled={saving} onClick={onCancel}>{label("Cancel")}</NexButton></div>
 </form>;
}
