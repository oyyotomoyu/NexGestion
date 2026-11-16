import { useCallback, useEffect, useState } from "react";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { listMyShifts, respondToAssignment, type MyScheduledShift } from "@/requests/scheduling";

import {
  addDays,
  errorLabelKey,
  useFormatMinutes,
  formatShiftTime,
  SchedulingTabs,
  StatusBadge,
  toDateInput,
  useSchedulingLabels,
} from "./shared";
import "./style.css";

const rangeOptions = ["7", "14", "28", "56"] as const;
type RangeDays = (typeof rangeOptions)[number];

export default function MyShifts() {
  const label = useSchedulingLabels();
  useDocumentTitle(label("MyShifts_Title"));
  const formatMinutes = useFormatMinutes();
  const [from, setFrom] = useState(() => toDateInput(new Date()));
  const [days, setDays] = useState<RangeDays>("14");
  const [shifts, setShifts] = useState<MyScheduledShift[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const to = addDays(from, Number(days) - 1);

  const load = useCallback(async () => {
    const response = await listMyShifts(from, to);
    setShifts(response.shifts);
  }, [from, to]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError("");
    load()
      .catch((cause) => { if (active) setError(label(errorLabelKey(cause))); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [load]);

  async function respond(shift: MyScheduledShift, status: "confirmed" | "declined") {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await respondToAssignment(shift.assignment_id, status);
      setMessage(label(status === "confirmed" ? "MyShifts_Confirmed" : "MyShifts_Declined"));
      await load();
    } catch (cause) {
      setError(label(errorLabelKey(cause)));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="scheduling-page">
      <header className="scheduling-header">
        <NexText variant="title">{label("MyShifts_Title")}</NexText>
      </header>
      <SchedulingTabs />

      <div className="scheduling-toolbar">
        <NexInput label={label("MyShifts_From")} type="date" value={from} onChange={(event) => event.target.value && setFrom(event.target.value)} />
        <div className="scheduling-field">
          <NexText as="span" variant="label">{label("MyShifts_Range")}</NexText>
          <NexSelect
            ariaLabel={label("MyShifts_Range")}
            value={days}
            onChange={setDays}
            options={rangeOptions.map((value) => ({ value, label: label("MyShifts_Days", { count: Number(value) }) }))}
          />
        </div>
        <div className="scheduling-actions">
          <NexButton variant="secondary" onClick={() => setFrom(addDays(from, -Number(days)))}>{label("Previous")}</NexButton>
          <NexButton variant="secondary" onClick={() => setFrom(toDateInput(new Date()))}>{label("Today")}</NexButton>
          <NexButton variant="secondary" onClick={() => setFrom(addDays(from, Number(days)))}>{label("Next")}</NexButton>
        </div>
      </div>

      {error && <div role="alert"><NexText color="danger">{error}</NexText></div>}
      {message && <div role="status"><NexText color="success">{message}</NexText></div>}

      <section className="scheduling-panel">
        <NexText variant="caption" color="muted">{label("MyShifts_RangeSummary", { from, to })}</NexText>
        {loading ? (
          <NexText role="status">{label("Loading")}</NexText>
        ) : shifts.length === 0 ? (
          <NexText color="muted">{label("MyShifts_Empty")}</NexText>
        ) : (
          <div className="scheduling-table-wrap">
            <table className="scheduling-table">
              <thead>
                <tr>
                  <th><NexText as="span" variant="label">{label("Date")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Time")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Shift")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Schedule")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Status")}</NexText></th>
                  <th><NexText as="span" variant="label">{label("Actions")}</NexText></th>
                </tr>
              </thead>
              <tbody>
                {shifts.map((shift) => {
                  const respondable = shift.period_status === "published" && ["planned", "confirmed"].includes(shift.assignment_status);
                  return (
                    <tr key={shift.assignment_id}>
                      <td data-label={label("Date")}><NexText as="span">{shift.local_start_at.slice(0, 10)}</NexText></td>
                      <td data-label={label("Time")}>
                        <div className="scheduling-cell">
                          <NexText as="span">{formatShiftTime(shift)} · {formatMinutes(shift.planned_minutes)}</NexText>
                          <NexText as="span" variant="caption" color="muted">{shift.timezone}</NexText>
                        </div>
                      </td>
                      <td data-label={label("Shift")}><NexText as="span">{shift.title || label("Untitled")}</NexText></td>
                      <td data-label={label("Schedule")}><NexText as="span">{shift.period_name} · {shift.group_name}</NexText></td>
                      <td data-label={label("Status")}><StatusBadge prefix="Assignment" status={shift.assignment_status} /></td>
                      <td data-label={label("Actions")}>
                        {respondable ? (
                          <div className="scheduling-actions">
                            {shift.assignment_status === "planned" && (
                              <NexButton size="compact" disabled={busy} onClick={() => void respond(shift, "confirmed")}>{label("MyShifts_Confirm")}</NexButton>
                            )}
                            <NexButton size="compact" variant="secondary" disabled={busy} onClick={() => void respond(shift, "declined")}>{label("MyShifts_Decline")}</NexButton>
                          </div>
                        ) : (
                          <NexText as="span" variant="caption" color="muted">{label(`Status_${shift.period_status}`)}</NexText>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </section>
  );
}
