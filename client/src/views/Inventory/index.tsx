import { useEffect, useState, type FormEvent } from "react";
import { NavLink } from "react-router-dom";
import { useAuth } from "@/auth/AuthProvider";
import { NexInput } from "@/components/NexInput";
import { NexButton } from "@/components/NexButton";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { listEntities, saveEntity, getStock, getRecords, postRecord, scanItem, transition, type Entity, type Line, type Stock, type Record as InventoryRecord } from "@/requests/inventory";
import "./style.css";

export const inventoryPages = [["stock", "Stock"], ["items", "Items"], ["warehouses", "Warehouses"], ["locations", "Storage locations"], ["inbound", "Receive goods"], ["outbound", "Ship goods"], ["transfer", "Transfer stock"], ["adjustment", "Adjust stock"], ["history", "History"]];
const blankEntity = {name: "", sku: "", unit_of_measure: "piece", barcode: "", address: "", status: "active", warehouse_id: "default"};
const today = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth()+1).padStart(2,"0")}-${String(d.getDate()).padStart(2,"0")}`; };
export default function Inventory({page = "stock"}: {page?: string}) {
  const { user } = useAuth();
  const canManage = user?.is_protected || user?.roles.some(r => r.grants_all_permissions || r.permissions.some(p => p.permission_key === "inventory.manage"));
  const [items,setItems] = useState<Entity[]>([]), [warehouses,setWarehouses] = useState<Entity[]>([]), [locations,setLocations] = useState<Entity[]>([]);
  const [stock,setStock] = useState<Stock[]>([]), [records,setRecords] = useState<InventoryRecord[]>([]);
  const [entity,setEntity] = useState<Partial<Entity>>(blankEntity);
  const [warehouse,setWarehouse] = useState("default"), [location,setLocation] = useState(""), [toWarehouse,setToWarehouse] = useState(""), [toLocation,setToLocation] = useState("");
  const [item,setItem] = useState(""), [quantity,setQuantity] = useState("1"), [lines,setLines] = useState<Line[]>([]), [code,setCode] = useState("");
  const [date,setDate] = useState(today), [note,setNote] = useState(""), [tracking,setTracking] = useState("");
  const [error,setError] = useState(""), [message,setMessage] = useState(""), [busy,setBusy] = useState(false), [loading,setLoading] = useState(true);
  async function load() { const [i,w,l,s,r] = await Promise.all([listEntities("items"),listEntities("warehouses"),listEntities("locations"),getStock(),getRecords()]); setItems(i);setWarehouses(w);setLocations(l);setStock(s);setRecords(r); }
  useEffect(() => { let active = true; load().catch(e => { if(active) setError(e.message); }).finally(() => {if(active)setLoading(false)}); return () => {active=false}; }, []);
  useEffect(() => {setEntity(blankEntity);setLines([]);setError("");setMessage("");}, [page]);
  async function run(action: () => Promise<unknown>, success: string) {setBusy(true);setError("");setMessage("");try {await action();setMessage(success);await load();} catch(e){setError(e instanceof Error ? e.message : "Request failed");} finally{setBusy(false);} }
  const name = (id: string, data: Entity[]) => data.find(e => e.id === id)?.name || id || "Unassigned";
  function select(label: string, value: string, change: (v:string)=>void, data: Entity[], optional = false) {return <div className="inventory-field"><NexText as="span" variant="label">{label}</NexText><NexSelect ariaLabel={label} value={value} onChange={change} options={[{value:"",label:optional ? "Unassigned" : "Select…"},...data.map(e=>({value:e.id,label:e.name}))]} /></div>;}
  function addLine(id: string, amount: string) {
    setLines(previous => [...previous,{item_id:id,location_id:location,quantity:amount}]);
  }
  async function scan(event: FormEvent) {event.preventDefault();await run(async()=>{const found=await scanItem(code);addLine(found.id,"1");setCode("");},"Item added");}
  const master = ["items","warehouses","locations"].includes(page);
  const operation = ["inbound","outbound","transfer","adjustment"].includes(page);
  const activeWarehouses = warehouses.filter(e=>e.status==="active");
  const zones = (w:string) => locations.filter(e=>e.warehouse_id===w && e.status==="active");
  return <div className="inventory-page"><header><NexText as="h1" variant="heading">Inventory · {inventoryPages.find(p=>p[0]===page)?.[1]}</NexText></header>
    <nav className="inventory-tabs">{inventoryPages.map(([key,label])=><NavLink key={key} to={`/inventory/${key}`}>{label}</NavLink>)}</nav>
    {error && <div role="alert">{error}</div>}{message && <div role="status">{message}</div>}{loading && <div role="status">Loading inventory…</div>}
    {master && <><section className="inventory-panel"><NexText as="h2" variant="heading">{entity.id ? "Edit" : "Create"} {page}</NexText>
      {canManage ? <form className="inventory-form" onSubmit={e=>{e.preventDefault();void run(async()=>{await saveEntity(page,entity);setEntity(blankEntity);},"Saved");}}>
      <NexInput label="Name" required value={entity.name||""} onChange={e=>setEntity({...entity,name:e.target.value})}/>
      {page==="items" ? <><NexInput label="SKU" required value={entity.sku||""} onChange={e=>setEntity({...entity,sku:e.target.value})}/><NexInput label="Unit of measure" required value={entity.unit_of_measure||""} onChange={e=>setEntity({...entity,unit_of_measure:e.target.value})}/><NexInput label="Barcode" value={entity.barcode||""} onChange={e=>setEntity({...entity,barcode:e.target.value})}/></> : <>
      {page==="warehouses" ? <NexInput label="Address" value={entity.address||""} onChange={e=>setEntity({...entity,address:e.target.value})}/> : select("Warehouse",entity.warehouse_id||"",v=>setEntity({...entity,warehouse_id:v}),activeWarehouses)}
      <NexSelect ariaLabel="Status" value={entity.status||"active"} onChange={v=>setEntity({...entity,status:v})} options={[{value:"active",label:"Active"},{value:"inactive",label:"Inactive"}]}/></>}
      <NexButton disabled={busy}>Save</NexButton>{entity.id && <NexButton type="button" variant="secondary" onClick={()=>setEntity(blankEntity)}>Cancel edit</NexButton>}</form> : <p>Inventory management permission is required to make changes.</p>}</section>
      <section className="inventory-panel">{(page==="items"?items:page==="warehouses"?warehouses:locations).map(e=><div className="inventory-row" key={e.id}><span>{e.name} {e.sku && `· ${e.sku} · ${e.unit_of_measure} · ${e.barcode || "No barcode"}`} {e.status && `· ${e.status}`} {e.warehouse_id && `· ${name(e.warehouse_id,warehouses)}`}</span>{canManage && <NexButton variant="secondary" onClick={()=>setEntity(e)}>Edit</NexButton>}</div>)}</section></>}
    {operation && (canManage ? <section className="inventory-panel"><div className="inventory-form">
      {select("Warehouse",warehouse,v=>{setWarehouse(v);setLocation("");setLines([])},activeWarehouses)}
      {select("Storage location",location,setLocation,zones(warehouse),true)}
      {page==="transfer" && <>{select("Destination warehouse",toWarehouse,v=>{setToWarehouse(v);setToLocation("")},activeWarehouses)}{select("Destination location",toLocation,setToLocation,zones(toWarehouse),true)}</>}
      <NexInput label="Date" type="date" value={date} onChange={e=>setDate(e.target.value)}/><NexInput label="Note" value={note} onChange={e=>setNote(e.target.value)}/>
      {page==="outbound" && <NexInput label="Tracking reference" value={tracking} onChange={e=>setTracking(e.target.value)}/>}
      </div>
      {(page==="inbound"||page==="outbound") && <form className="inventory-form" onSubmit={scan}><NexInput label="Scan barcode or SKU" value={code} onChange={e=>setCode(e.target.value)} autoComplete="off"/><NexButton disabled={busy||!code}>Add scanned item</NexButton></form>}
      <form className="inventory-form" onSubmit={e=>{e.preventDefault();addLine(item,quantity)}}>
        {select("Item",item,setItem,items)}<NexInput label={page==="adjustment"?"Quantity change (positive or negative)":"Quantity"} type="number" step="0.000001" min={page==="adjustment"?undefined:"0.000001"} required value={quantity} onChange={e=>setQuantity(e.target.value)}/><NexButton disabled={!item||!warehouse||busy}>Add line</NexButton>
      </form>
      {lines.map((l,index)=><div className="inventory-row" key={index}><span>{name(l.item_id,items)} · {l.quantity} · {name(l.location_id,locations)}</span><NexButton variant="secondary" onClick={()=>setLines(lines.filter((_,i)=>i!==index))}>Remove</NexButton></div>)}
      <NexButton disabled={busy||!warehouse||!lines.length||!date||(page==="transfer"&&!toWarehouse)} onClick={()=>void run(async()=>{await postRecord({kind:page,warehouse_id:warehouse,to_warehouse_id:toWarehouse,to_location_id:toLocation,date,note,tracking_reference:tracking,lines});setLines([]);setNote("");setTracking("");},page==="outbound"?"Shipment created. Pack or ship it from History.":"Stock updated")}>{page==="outbound"?"Create pending shipment":"Record movement"}</NexButton>
    </section> : <p>Inventory management permission is required to record stock movements.</p>)}
    {page==="stock" && <section className="inventory-panel"><NexText as="h2" variant="heading">On-hand stock</NexText>{stock.length===0 && <p>No stock movements yet. Receive goods to get started.</p>}<div className="inventory-table"><table><thead><tr><th>Item</th><th>Warehouse</th><th>Location</th><th>Quantity</th></tr></thead><tbody>{stock.map((s,i)=><tr key={i}><td>{name(s.item_id,items)}</td><td>{name(s.warehouse_id,warehouses)}</td><td>{name(s.location_id,locations)}</td><td>{s.quantity} {items.find(e=>e.id===s.item_id)?.unit_of_measure}</td></tr>)}</tbody></table></div></section>}
    {page==="history" && <section className="inventory-panel">{!records.length && <p>No inventory records yet.</p>}{records.map(r=><article className="inventory-record" key={r.id}><NexText as="h2" variant="label">{r.kind} · {r.status} · {r.date}</NexText><p>{name(r.warehouse_id,warehouses)}{r.kind==="transfer"&&` → ${name(r.to_warehouse_id,warehouses)}`} · {r.id}</p><p>{r.note} {r.tracking_reference}</p>{r.lines.map((l,i)=><p key={i}>{name(l.item_id,items)} · {l.quantity} · {name(l.location_id,locations)}</p>)}<p>Recorded {new Date(r.created_at).toLocaleString()} · {r.created_by}</p>{canManage && r.kind==="outbound" && ["pending","packed"].includes(r.status) && <div className="inventory-form">{(r.status==="pending"?["packed","shipped","cancelled"]:["shipped","cancelled"]).map(status=><NexButton key={status} disabled={busy} onClick={()=>void run(()=>transition(r.id,status),"Shipment updated")}>{status==="packed"?"Mark packed":status==="shipped"?"Ship and deduct stock":"Cancel shipment"}</NexButton>)}</div>}</article>)}</section>}
  </div>;
}
