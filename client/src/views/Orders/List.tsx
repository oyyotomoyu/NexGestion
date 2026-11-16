import { useEffect, useState } from "react";
import { Link, NavLink, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { listOrders, type DocumentKind, type OrderDocument } from "@/requests/orders";
import type { Pagination } from "@/requests/core/list";
import { can, errorKey } from "./shared";
import "./style.css";

export default function OrdersList({kind}: {kind:DocumentKind}) {
 const {t}=useTranslation("ui");const label=(key:string)=>t(`global.k_Orders_${key}`);const {user}=useAuth();
 const [params,setParams]=useSearchParams();const [search,setSearch]=useState(params.get("keyword")||"");const page=Math.max(1,Number(params.get("page"))||1);
 const [items,setItems]=useState<OrderDocument[]>([]),[pagination,setPagination]=useState<Pagination|null>(null),[error,setError]=useState(""),[loading,setLoading]=useState(true);
 const root=kind==="quote"?"/quotes":"/orders";const title=label(kind==="quote"?"Quotes":"Orders");
 useDocumentTitle(title);
 function update(key:string,value:string){const next=new URLSearchParams(params);if(value)next.set(key,value);else next.delete(key);if(key!=="page")next.delete("page");setParams(next)}
 useEffect(()=>{let active=true;setLoading(true);setError("");listOrders(kind,{page,page_size:20,keyword:params.get("keyword")||"",status:params.get("status")||"",sort:params.get("sort")||"created_at",order:"desc"}).then(response=>{if(active){setItems(response[kind==="quote"?"quotes":"orders"] as OrderDocument[]);setPagination(response.pagination)}}).catch(e=>{if(active)setError(label(errorKey(e)))}).finally(()=>{if(active)setLoading(false)});return()=>{active=false}},[kind,params,t]);
 const statuses=kind==="quote"?["draft","sent","accepted","rejected","expired"]:["draft","confirmed","fulfilled","cancelled"];
 return <section className="orders-page"><header className="orders-header"><NexText variant="title">{title}</NexText>{can(user,"orders.manage")&&<Link className="orders-link" to={`${root}/new`}>{label(kind==="quote"?"NewQuote":"NewOrder")}</Link>}</header>
  <nav className="orders-toolbar"><NavLink className="orders-link" to="/orders">{label("Orders")}</NavLink><NavLink className="orders-link" to="/quotes">{label("Quotes")}</NavLink></nav>
  <form className="orders-toolbar" onSubmit={e=>{e.preventDefault();update("keyword",search)}}><NexInput label={label("Search")} value={search} onChange={e=>setSearch(e.target.value)}/><NexButton>{label("Search")}</NexButton>
   <NexSelect ariaLabel={label("Status")} value={params.get("status")||""} onChange={value=>update("status",value)} options={[{value:"",label:label("AllStatuses")},...statuses.map(value=>({value,label:label(`Status_${value}`)}))]}/>
   <NexSelect ariaLabel={label("Sort")} value={params.get("sort")||"created_at"} onChange={value=>update("sort",value)} options={[{value:"created_at",label:label("Created")},{value:"updated_at",label:label("Updated")},{value:"total_amount",label:label("Total")}]}/>
  </form>
  {error&&<div role="alert">{error}</div>}
  {loading?<p role="status">{t("global.k_Common_Loading")}</p>:!error&&<section className="orders-panel"><div className="orders-table"><table><thead><tr><th>{label("Customer")}</th><th>{label("Date")}</th><th>{label("Status")}</th><th>{label("Total")}</th></tr></thead><tbody>{items.map(d=><tr key={d.id}><td><Link to={`${root}/${d.id}`}>{d.customer_name}</Link></td><td>{d.quote_date||d.order_date}</td><td>{label(`Status_${d.status}`)}</td><td>{d.total_amount} {d.currency}</td></tr>)}</tbody></table></div>{!items.length&&<p>{label("Empty")}</p>}
   <div className="orders-toolbar"><NexButton variant="secondary" disabled={page<=1} onClick={()=>update("page",String(page-1))}>{label("Previous")}</NexButton><span>{page} / {Math.max(1,pagination?.total_pages||1)} · {pagination?.total||0} {label("Records")}</span><NexButton variant="secondary" disabled={page>=(pagination?.total_pages||0)} onClick={()=>update("page",String(page+1))}>{label("Next")}</NexButton></div>
  </section>}
 </section>;
}
