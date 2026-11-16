import { useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import type { Pagination } from "@/requests/core/list";
import {
  createPeriod,
  listPeriods,
  listSchedulingGroups,
  type PeriodInput,
  type SchedulePeriod,
  type SchedulingGroup,
} from "@/requests/scheduling";

import {
  addDays,
  errorLabelKey,
  SchedulingTabs,
  StatusBadge,
  toDateInput,
  usePermissions,
  useSchedulingLabels,
} from "./shared";
import "./style.css";

const statuses = ["draft", "published", "locked", "archived", "cancelled"];

function blankPeriod(): PeriodInput {
  const start = toDateInput(new Date());
  return {
    group_id: "",
    name: "",
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
    start_date: start,
    end_date: addDays(start, 6),
  };
}

export default function SchedulePeriods() {
  const label = useSchedulingLabels();
  const has = usePermissions();
  const navigate = useNavigate();
  useDocumentTitle(label("Periods_Title"));
  const [params, setParams] = useSearchParams();
  const page = Math.max(1, Number(params.get("page")) || 1);
  const [search, setSearch] = useState(params.get("keyword") || "");
  const [periods, setPeriods] = useState<SchedulePeriod[]>([]);
  const [pagination, setPagination] = useState<Pagination | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const canManage = has("scheduling.manage");
  const [groups, setGroups] = useState<SchedulingGroup[]>([]);
  const [draft, setDraft] = useState<PeriodInput>(blankPeriod);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  function update(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    if (key !== "page") next.delete("page");
    setParams(next);
  }

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    listPeriods({
      page,
      page_size: 20,
      keyword: params.get("keyword") || "",
      status: params.get("status") || "",
      sort: "start_date",
      order: "desc",
    })
      .then((response) => {
        if (!active) return;
        setPeriods(response.periods as SchedulePeriod[]);
        setPagination(response.pagination);
      })
      .catch((cause) => { if (active) setError(label(errorLabelKey(cause))); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [page, params]);

  useEffect(() => {
    if (!canManage) return;
    listSchedulingGroups({ status: "active" })
      .then((response) => setGroups(response.groups as SchedulingGroup[]))
      .catch(() => setGroups([]));
  }, [canManage]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setCreating(true);
    setCreateError("");
    try {
      const period = await createPeriod(draft);
      navigate(`/scheduling/periods/${period.id}`);
    } catch (cause) {
      setCreateError(label(errorLabelKey(cause)));
    } finally {
      setCreating(false);
    }
  }

  return (
    <section className="scheduling-page">
      <header className="scheduling-header">
        <NexText variant="title">{label("Periods_Title")}</NexText>
      </header>
      <SchedulingTabs />

      {canManage && (
        <section className="scheduling-panel">
          <NexText variant="subheading">{label("Periods_Create")}</NexText>
          <form className="scheduling-form scheduling-form--grid" onSubmit={submit}>
            <div className="scheduling-field">
              <NexText as="span" variant="label">{label("Group")}</NexText>
              <NexSelect
                ariaLabel={label("Group")}
                value={draft.group_id}
                onChange={(group_id) => setDraft({ ...draft, group_id })}
                options={[
                  { value: "", label: label("SelectGroup") },
                  ...groups.map((group) => ({ value: group.id, label: group.name })),
                ]}
              />
            </div>
            <NexInput label={label("Name")} required value={draft.name} placeholder={label("Periods_NamePlaceholder")} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
            <NexInput label={label("Timezone")} required value={draft.timezone} onChange={(event) => setDraft({ ...draft, timezone: event.target.value })} />
            <NexInput label={label("StartDate")} type="date" required value={draft.start_date} onChange={(event) => setDraft({ ...draft, start_date: event.target.value })} />
            <NexInput label={label("EndDate")} type="date" required min={draft.start_date} value={draft.end_date} onChange={(event) => setDraft({ ...draft, end_date: event.target.value })} />
            <div className="scheduling-actions scheduling-form__submit">
              <NexButton disabled={creating || !draft.group_id || !draft.name.trim()}>{label("Periods_CreateSubmit")}</NexButton>
            </div>
          </form>
          {groups.length === 0 && <NexText variant="caption" color="muted">{label("Periods_NoGroups")}</NexText>}
          {createError && <div role="alert"><NexText color="danger">{createError}</NexText></div>}
        </section>
      )}

      <form className="scheduling-toolbar" onSubmit={(event) => { event.preventDefault(); update("keyword", search); }}>
        <NexInput label={label("Search")} value={search} onChange={(event) => setSearch(event.target.value)} />
        <NexButton variant="secondary">{label("Search")}</NexButton>
        <div className="scheduling-field">
          <NexText as="span" variant="label">{label("Status")}</NexText>
          <NexSelect
            ariaLabel={label("Status")}
            value={params.get("status") || ""}
            onChange={(value) => update("status", value)}
            options={[{ value: "", label: label("AllStatuses") }, ...statuses.map((value) => ({ value, label: label(`Status_${value}`) }))]}
          />
        </div>
      </form>

      {error && <div role="alert"><NexText color="danger">{error}</NexText></div>}

      <section className="scheduling-panel">
        {loading ? (
          <NexText role="status">{label("Loading")}</NexText>
        ) : periods.length === 0 ? (
          <NexText color="muted">{label("Periods_Empty")}</NexText>
        ) : (
          <div className="scheduling-table-wrap">
            <table className="scheduling-table">
              <thead>
                <tr>
                  <th><NexText as="span" variant="label">{label("Name")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Group")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Dates")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Status")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Shifts")}</NexText></th>
                </tr>
              </thead>
              <tbody>
                {periods.map((period) => (
                  <tr key={period.id}>
                    <td data-label={label("Name")}>
                      <Link className="scheduling-link" to={`/scheduling/periods/${period.id}`}>
                        <NexText as="span" color="inherit">{period.name}</NexText>
                      </Link>
                    </td>
                    <td data-label={label("Group")}><NexText as="span">{period.group_name}</NexText></td>
                    <td data-label={label("Dates")}><NexText as="span">{period.start_date} – {period.end_date}</NexText></td>
                    <td data-label={label("Status")}><StatusBadge prefix="Status" status={period.status} /></td>
                    <td data-label={label("Shifts")}><NexText as="span">{period.shift_count}</NexText></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <div className="scheduling-actions scheduling-pagination">
          <NexButton variant="secondary" disabled={page <= 1} onClick={() => update("page", String(page - 1))}>{label("Previous")}</NexButton>
          <NexText as="span">{label("PageSummary", { page, pages: Math.max(1, pagination?.total_pages || 1), total: pagination?.total || 0 })}</NexText>
          <NexButton variant="secondary" disabled={page >= (pagination?.total_pages || 0)} onClick={() => update("page", String(page + 1))}>{label("Next")}</NexButton>
        </div>
      </section>
    </section>
  );
}
