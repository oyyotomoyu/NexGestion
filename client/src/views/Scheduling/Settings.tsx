import { useEffect, useState, type FormEvent } from "react";

import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { getSchedulingConfig, updateSchedulingConfig, type SchedulingConfig } from "@/requests/scheduling";

import { errorLabelKey, SchedulingTabs, useSchedulingLabels } from "./shared";
import "./style.css";

// The API stores minutes; the form edits hours, where 0 turns a rule off.
const toHours = (minutes: number) => String(minutes / 60);
const toMinutes = (hours: string) => Math.round((Number(hours) || 0) * 60);

export default function SchedulingSettings() {
  const label = useSchedulingLabels();
  useDocumentTitle(label("Settings_Title"));
  const [config, setConfig] = useState<SchedulingConfig | null>(null);
  const [weeklyHours, setWeeklyHours] = useState("0");
  const [restHours, setRestHours] = useState("0");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  function apply(next: SchedulingConfig) {
    setConfig(next);
    setWeeklyHours(toHours(next.max_weekly_minutes));
    setRestHours(toHours(next.min_rest_minutes));
  }

  useEffect(() => {
    getSchedulingConfig().then(apply).catch((cause) => setError(label(errorLabelKey(cause))));
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!config) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      apply(await updateSchedulingConfig({
        group_mode: config.group_mode,
        allow_open_shifts: config.allow_open_shifts,
        max_weekly_minutes: toMinutes(weeklyHours),
        min_rest_minutes: toMinutes(restHours),
      }));
      setMessage(label("Done_Saved"));
    } catch (cause) {
      setError(label(errorLabelKey(cause)));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="scheduling-page">
      <header className="scheduling-header">
        <NexText variant="title">{label("Settings_Title")}</NexText>
      </header>
      <SchedulingTabs />
      {error && <div role="alert"><NexText color="danger">{error}</NexText></div>}
      {message && <div role="status"><NexText color="success">{message}</NexText></div>}
      {!config ? (
        !error && <NexText role="status">{label("Loading")}</NexText>
      ) : (
        <form className="scheduling-panel scheduling-settings" onSubmit={submit}>
          <div className="scheduling-field">
            <NexText as="span" variant="label">{label("Settings_GroupMode")}</NexText>
            <NexSelect
              ariaLabel={label("Settings_GroupMode")}
              value={config.group_mode}
              onChange={(group_mode) => setConfig({ ...config, group_mode })}
              options={[
                { value: "advisory", label: label("Settings_GroupMode_advisory") },
                { value: "required", label: label("Settings_GroupMode_required") },
              ]}
            />
            <NexText variant="caption" color="muted">{label(`Settings_GroupModeHelp_${config.group_mode}`)}</NexText>
          </div>
          <div className="scheduling-field">
            <NexText as="span" variant="label">{label("Settings_OpenShifts")}</NexText>
            <NexSelect
              ariaLabel={label("Settings_OpenShifts")}
              value={config.allow_open_shifts ? "yes" : "no"}
              onChange={(value) => setConfig({ ...config, allow_open_shifts: value === "yes" })}
              options={[
                { value: "yes", label: label("Settings_OpenShifts_yes") },
                { value: "no", label: label("Settings_OpenShifts_no") },
              ]}
            />
          </div>
          <div className="scheduling-field">
            <NexInput label={label("Settings_MaxWeekly")} type="number" min="0" max="168" step="0.5" value={weeklyHours} onChange={(event) => setWeeklyHours(event.target.value)} />
            <NexText variant="caption" color="muted">{label("Settings_ZeroDisables")}</NexText>
          </div>
          <div className="scheduling-field">
            <NexInput label={label("Settings_MinRest")} type="number" min="0" max="24" step="0.5" value={restHours} onChange={(event) => setRestHours(event.target.value)} />
            <NexText variant="caption" color="muted">{label("Settings_ZeroDisables")}</NexText>
          </div>
          <div className="scheduling-actions">
            <NexButton disabled={busy}>{label("Save")}</NexButton>
          </div>
        </form>
      )}
    </section>
  );
}
