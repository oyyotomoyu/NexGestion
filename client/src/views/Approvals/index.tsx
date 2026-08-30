import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  cancelApprovalRequest,
  decideApprovalRequest,
  listFlowTemplates,
  listMyApprovalAssignments,
  listMyApprovalRequests,
  submitApprovalRequest,
} from "@/requests/approvals";
import type { ApprovalFlowTemplate, ApprovalRequest } from "@/requests/approvals/types";
import { listUsers } from "@/requests/users";
import type { User } from "@/requests/users/types";

import "./style.css";

type Tab = "assigned" | "mine";

function hasPermission(user: ReturnType<typeof useAuth>["user"], key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true
  );
}

export default function Approvals() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canReadSelf = hasPermission(user, "approvals.read.self");
  const canDecide = hasPermission(user, "approvals.decide");
  const canBrowseTemplates = hasPermission(user, "approvals.templates.manage");
  const canManage = canBrowseTemplates || hasPermission(user, "approvals.read") || hasPermission(user, "approvals.reassign");

  const [tab, setTab] = useState<Tab>("assigned");
  const [assignments, setAssignments] = useState<ApprovalRequest[]>([]);
  const [myRequests, setMyRequests] = useState<ApprovalRequest[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [templates, setTemplates] = useState<ApprovalFlowTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [comments, setComments] = useState<Record<string, string>>({});
  const [savingID, setSavingID] = useState<string | null>(null);

  const [flowTemplateID, setFlowTemplateID] = useState("");
  const [sourceModule, setSourceModule] = useState("");
  const [sourceReferenceID, setSourceReferenceID] = useState("");
  const [amount, setAmount] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitNotice, setSubmitNotice] = useState<string | null>(null);

  useDocumentTitle(t("global.k_Approvals_PageTitle"));

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    const loaders: Promise<unknown>[] = [
      listUsers().then((result) => { if (active) setUsers(result); }).catch(() => undefined),
    ];
    if (canReadSelf) {
      loaders.push(
        listMyApprovalAssignments().then((result) => { if (active) setAssignments(result); }),
        listMyApprovalRequests().then((result) => { if (active) setMyRequests(result); }),
      );
    }
    if (canBrowseTemplates) {
      loaders.push(
        listFlowTemplates().then((result) => {
          if (active) setTemplates(result.filter((template) => template.status === "active"));
        }),
      );
    }
    Promise.all(loaders)
      .catch(() => { if (active) setError(true); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [canReadSelf, canBrowseTemplates]);

  useEffect(() => {
    if (canBrowseTemplates && templates.length && !flowTemplateID) {
      setFlowTemplateID(templates[0].id);
    }
  }, [canBrowseTemplates, templates, flowTemplateID]);

  function userName(id: string) {
    return users.find((item) => item.id === id)?.display_name ?? id;
  }

  async function decide(requestItem: ApprovalRequest, decision: "approved" | "rejected") {
    setSavingID(requestItem.id);
    setError(false);
    try {
      const updated = await decideApprovalRequest(requestItem.id, {
        decision,
        comment: comments[requestItem.id]?.trim() || undefined,
      });
      setAssignments((items) => items.filter((item) => item.id !== updated.id));
      setMyRequests((items) => items.map((item) => (item.id === updated.id ? updated : item)));
      setComments((current) => ({ ...current, [requestItem.id]: "" }));
    } catch {
      setError(true);
    } finally {
      setSavingID(null);
    }
  }

  async function cancel(requestItem: ApprovalRequest) {
    if (!window.confirm(t("global.k_Approvals_CancelConfirm"))) return;
    setSavingID(requestItem.id);
    setError(false);
    try {
      const updated = await cancelApprovalRequest(requestItem.id);
      setMyRequests((items) => items.map((item) => (item.id === updated.id ? updated : item)));
    } catch {
      setError(true);
    } finally {
      setSavingID(null);
    }
  }

  const canSubmit = flowTemplateID.trim() !== "" && sourceModule.trim() !== "" && sourceReferenceID.trim() !== "";

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!canSubmit) return;
    setSubmitting(true);
    setSubmitError(null);
    setSubmitNotice(null);
    try {
      await submitApprovalRequest({
        flow_template_id: flowTemplateID.trim(),
        source_module: sourceModule.trim(),
        source_reference_id: sourceReferenceID.trim(),
        amount: amount.trim() || undefined,
      });
      const refreshed = await listMyApprovalRequests();
      setMyRequests(refreshed);
      setSourceModule("");
      setSourceReferenceID("");
      setAmount("");
      setSubmitNotice(t("global.k_Approvals_SubmitSuccess"));
    } catch {
      setSubmitError(t("global.k_Approvals_SubmitError"));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <section className="approvals-page">
      <header className="approvals-page__header">
        <div>
          <NexText variant="title">{t("global.k_Approvals_Title")}</NexText>
          <NexText color="muted">{t("global.k_Approvals_Description")}</NexText>
        </div>
        {canManage ? (
          <Link className="salary-download" to="/approvals/manage">
            {t("global.k_Approvals_ManageLink")}
          </Link>
        ) : null}
      </header>

      {error ? (
        <div className="approvals-alert approvals-alert--error" role="alert">
          <NexText color="danger">{t("global.k_Approvals_Error")}</NexText>
        </div>
      ) : null}
      {loading ? (
        <div className="approvals-loading">
          <NexText color="muted">{t("global.k_Common_Loading")}</NexText>
        </div>
      ) : null}

      {canReadSelf ? (
        <>
          <div className="approvals-tabs" role="tablist">
            <button
              type="button"
              role="tab"
              aria-selected={tab === "assigned"}
              className={`approvals-tab${tab === "assigned" ? " is-active" : ""}`}
              onClick={() => setTab("assigned")}
            >
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Approvals_TabAssigned")} ({assignments.length})
              </NexText>
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={tab === "mine"}
              className={`approvals-tab${tab === "mine" ? " is-active" : ""}`}
              onClick={() => setTab("mine")}
            >
              <NexText as="span" variant="label" color="inherit">
                {t("global.k_Approvals_TabMine")}
              </NexText>
            </button>
          </div>

          {tab === "assigned" ? (
            <div className="approvals-section">
              {!loading && !assignments.length ? (
                <div className="approvals-empty">
                  <NexText color="muted">{t("global.k_Approvals_AssignedEmpty")}</NexText>
                </div>
              ) : null}
              <div className="approvals-list">
                {assignments.map((item) => (
                  <article className="approvals-card" key={item.id}>
                    <div className="approvals-card__header">
                      <div>
                        <NexText variant="subheading">
                          {item.source_module} / {item.source_reference_id}
                        </NexText>
                        <NexText color="muted">
                          {t("global.k_Approvals_RequestedBy")}: {userName(item.requested_by_user_id)}
                        </NexText>
                      </div>
                      <span className={`approvals-badge approvals-badge--${item.status}`}>
                        {t(`global.k_Approvals_Status_${item.status}`)}
                      </span>
                    </div>
                    <dl className="approvals-card__details">
                      <div>
                        <dt>{t("global.k_Approvals_CurrentStep")}</dt>
                        <dd>{item.current_step_order}</dd>
                      </div>
                      <div>
                        <dt>{t("global.k_Approvals_Amount")}</dt>
                        <dd>{item.amount ?? "-"}</dd>
                      </div>
                    </dl>
                    {canDecide ? (
                      <div className="approvals-card__decision">
                        <NexInput
                          id={`approvals-comment-${item.id}`}
                          label={t("global.k_Approvals_Comment")}
                          value={comments[item.id] || ""}
                          onChange={(event) =>
                            setComments((current) => ({ ...current, [item.id]: event.target.value }))
                          }
                        />
                        <div className="approvals-card__actions">
                          <NexButton
                            type="button"
                            variant="secondary"
                            disabled={savingID === item.id}
                            onClick={() => void decide(item, "rejected")}
                          >
                            {t("global.k_Approvals_Reject")}
                          </NexButton>
                          <NexButton
                            type="button"
                            disabled={savingID === item.id}
                            onClick={() => void decide(item, "approved")}
                          >
                            {savingID === item.id ? t("global.k_Common_Saving") : t("global.k_Approvals_Approve")}
                          </NexButton>
                        </div>
                      </div>
                    ) : null}
                  </article>
                ))}
              </div>
            </div>
          ) : (
            <div className="approvals-section">
              <form className="approvals-form" onSubmit={(event) => void submit(event)}>
                <div className="approvals-form__full">
                  <NexText variant="subheading">{t("global.k_Approvals_SubmitTitle")}</NexText>
                  <NexText color="muted">{t("global.k_Approvals_SubmitDescription")}</NexText>
                </div>
                {submitNotice ? (
                  <div className="approvals-alert approvals-alert--success approvals-form__full" role="status">
                    <NexText color="primary">{submitNotice}</NexText>
                  </div>
                ) : null}
                {submitError ? (
                  <div className="approvals-alert approvals-alert--error approvals-form__full" role="alert">
                    <NexText color="danger">{submitError}</NexText>
                  </div>
                ) : null}
                {canBrowseTemplates ? (
                  <label className="approvals-select-field">
                    <NexText as="span" variant="label">
                      {t("global.k_Approvals_FlowTemplate")}
                    </NexText>
                    <NexSelect
                      ariaLabel={t("global.k_Approvals_FlowTemplate")}
                      value={flowTemplateID}
                      options={templates.map((template) => ({
                        value: template.id,
                        label: `${template.name} (${template.request_type})`,
                      }))}
                      onChange={setFlowTemplateID}
                    />
                  </label>
                ) : (
                  <NexInput
                    id="approvals-submit-template-id"
                    label={t("global.k_Approvals_FlowTemplateID")}
                    required
                    value={flowTemplateID}
                    onChange={(event) => setFlowTemplateID(event.target.value)}
                  />
                )}
                <NexInput
                  id="approvals-submit-source-module"
                  label={t("global.k_Approvals_SourceModuleLabel")}
                  required
                  value={sourceModule}
                  onChange={(event) => setSourceModule(event.target.value)}
                />
                <NexInput
                  id="approvals-submit-source-reference"
                  label={t("global.k_Approvals_SourceReferenceLabel")}
                  required
                  value={sourceReferenceID}
                  onChange={(event) => setSourceReferenceID(event.target.value)}
                />
                <NexInput
                  id="approvals-submit-amount"
                  label={t("global.k_Approvals_AmountLabel")}
                  inputMode="decimal"
                  value={amount}
                  onChange={(event) => setAmount(event.target.value)}
                />
                <div className="approvals-form__full">
                  <NexButton type="submit" disabled={submitting || !canSubmit}>
                    {submitting ? t("global.k_Common_Saving") : t("global.k_Approvals_Submit")}
                  </NexButton>
                </div>
              </form>

              {!loading && !myRequests.length ? (
                <div className="approvals-empty">
                  <NexText color="muted">{t("global.k_Approvals_MineEmpty")}</NexText>
                </div>
              ) : null}
              <div className="approvals-list">
                {myRequests.map((item) => (
                  <article className="approvals-card" key={item.id}>
                    <div className="approvals-card__header">
                      <div>
                        <NexText variant="subheading">
                          {item.source_module} / {item.source_reference_id}
                        </NexText>
                        <NexText color="muted">
                          {t("global.k_Approvals_CreatedAt")}: {item.created_at}
                        </NexText>
                      </div>
                      <span className={`approvals-badge approvals-badge--${item.status}`}>
                        {t(`global.k_Approvals_Status_${item.status}`)}
                      </span>
                    </div>
                    <dl className="approvals-card__details">
                      <div>
                        <dt>{t("global.k_Approvals_CurrentStep")}</dt>
                        <dd>{item.current_step_order}</dd>
                      </div>
                      <div>
                        <dt>{t("global.k_Approvals_Amount")}</dt>
                        <dd>{item.amount ?? "-"}</dd>
                      </div>
                    </dl>
                    {item.status === "pending" ? (
                      <div className="approvals-card__actions">
                        <NexButton
                          type="button"
                          variant="secondary"
                          disabled={savingID === item.id}
                          onClick={() => void cancel(item)}
                        >
                          {t("global.k_Approvals_Cancel")}
                        </NexButton>
                      </div>
                    ) : null}
                  </article>
                ))}
              </div>
            </div>
          )}
        </>
      ) : null}
    </section>
  );
}
