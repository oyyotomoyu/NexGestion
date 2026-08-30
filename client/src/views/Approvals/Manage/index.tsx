import { useEffect, useMemo, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/auth/AuthProvider";
import { NexButton } from "@/components/NexButton";
import { NexInput } from "@/components/NexInput";
import { NexSelect } from "@/components/NexSelect";
import { NexText } from "@/components/NexText";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import {
  createFlowTemplate,
  deleteFlowTemplate,
  listApprovalRequests,
  listFlowTemplates,
  reassignApprovalRequest,
  updateFlowTemplate,
} from "@/requests/approvals";
import type {
  ApprovalApproverType,
  ApprovalFlowTemplate,
  ApprovalNotifyOn,
  ApprovalRequest,
  ApprovalRequestStatus,
  ApprovalTargetType,
  NotificationTargetInput,
  StepTemplateInput,
} from "@/requests/approvals/types";
import { listGroups } from "@/requests/groups";
import type { Group } from "@/requests/groups/types";
import { listRoles } from "@/requests/roles";
import type { Role } from "@/requests/roles/types";
import { listUsers } from "@/requests/users";
import type { User } from "@/requests/users/types";

import "../style.css";

const requestTypePattern = /^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$/;
const amountPattern = /^\d+(\.\d{1,6})?$/;

const approverTypes: ApprovalApproverType[] = ["specific_user", "role", "requester_manager", "group_manager"];
const targetTypes: ApprovalTargetType[] = ["specific_user", "role", "group_manager"];
const notifyOnOptions: ApprovalNotifyOn[] = ["approved", "rejected", "both"];
const statusFilters: (ApprovalRequestStatus | "all")[] = [
  "all",
  "pending",
  "approved",
  "rejected",
  "cancelled",
  "requires_assignment",
];

function hasPermission(user: ReturnType<typeof useAuth>["user"], key: string) {
  return (
    user?.is_protected === true ||
    user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true
  );
}

function emptyStep(): StepTemplateInput {
  return { approver_type: "requester_manager" };
}

function emptyTarget(): NotificationTargetInput {
  return { target_type: "specific_user", notify_on: "both" };
}

function stepValid(step: StepTemplateInput) {
  if (step.approver_type === "specific_user" && !step.approver_user_id) return false;
  if (step.approver_type === "role" && !step.approver_role_id) return false;
  if (step.approver_type === "group_manager" && !step.approver_group_id) return false;
  if (step.min_amount && !amountPattern.test(step.min_amount)) return false;
  return true;
}

function targetValid(target: NotificationTargetInput) {
  if (target.target_type === "specific_user" && !target.target_user_id) return false;
  if (target.target_type === "role" && !target.target_role_id) return false;
  if (target.target_type === "group_manager" && !target.target_group_id) return false;
  return true;
}

export default function ApprovalsManage() {
  const { t } = useTranslation("ui");
  const { user } = useAuth();
  const canTemplates = hasPermission(user, "approvals.templates.manage");
  const canReadAll = hasPermission(user, "approvals.read");
  const canReassign = hasPermission(user, "approvals.reassign");

  const [templates, setTemplates] = useState<ApprovalFlowTemplate[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const [formOpen, setFormOpen] = useState(false);
  const [editingTemplateID, setEditingTemplateID] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [requestType, setRequestType] = useState("");
  const [status, setStatus] = useState<"active" | "inactive">("active");
  const [steps, setSteps] = useState<StepTemplateInput[]>([emptyStep()]);
  const [targets, setTargets] = useState<NotificationTargetInput[]>([]);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const [requests, setRequests] = useState<ApprovalRequest[]>([]);
  const [requestsLoading, setRequestsLoading] = useState(false);
  const [requestsError, setRequestsError] = useState(false);
  const [statusFilter, setStatusFilter] = useState<ApprovalRequestStatus | "all">("all");
  const [reassigningID, setReassigningID] = useState<string | null>(null);
  const [reassignSelection, setReassignSelection] = useState<string[]>([]);
  const [reassignSaving, setReassignSaving] = useState(false);
  const [reassignError, setReassignError] = useState<string | null>(null);

  useDocumentTitle(t("global.k_ApprovalsManage_PageTitle"));

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(false);
    const loaders: Promise<unknown>[] = [
      listUsers().then((result) => { if (active) setUsers(result); }).catch(() => undefined),
    ];
    if (canTemplates) {
      loaders.push(
        listFlowTemplates().then((result) => { if (active) setTemplates(result); }),
        listRoles().then((result) => { if (active) setRoles(result); }),
        listGroups().then((result) => { if (active) setGroups(result); }),
      );
    }
    Promise.all(loaders)
      .catch(() => { if (active) setError(true); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [canTemplates]);

  useEffect(() => {
    if (!canReadAll) return;
    let active = true;
    setRequestsLoading(true);
    setRequestsError(false);
    listApprovalRequests(statusFilter === "all" ? {} : { status: statusFilter })
      .then((result) => { if (active) setRequests(result); })
      .catch(() => { if (active) setRequestsError(true); })
      .finally(() => { if (active) setRequestsLoading(false); });
    return () => { active = false; };
  }, [canReadAll, statusFilter]);

  function userName(id: string) {
    return users.find((item) => item.id === id)?.display_name ?? id;
  }

  function openCreate() {
    setEditingTemplateID(null);
    setName("");
    setRequestType("");
    setStatus("active");
    setSteps([emptyStep()]);
    setTargets([]);
    setFormError(null);
    setFormOpen(true);
  }

  function openEdit(template: ApprovalFlowTemplate) {
    setEditingTemplateID(template.id);
    setName(template.name);
    setRequestType(template.request_type);
    setStatus(template.status);
    setSteps(
      template.steps
        .slice()
        .sort((a, b) => a.step_order - b.step_order)
        .map((step) => ({
          approver_type: step.approver_type,
          approver_user_id: step.approver_user_id ?? undefined,
          approver_role_id: step.approver_role_id ?? undefined,
          approver_group_id: step.approver_group_id ?? undefined,
          min_amount: step.min_amount ?? undefined,
        })),
    );
    setTargets(
      template.notification_targets.map((target) => ({
        target_type: target.target_type,
        target_user_id: target.target_user_id ?? undefined,
        target_role_id: target.target_role_id ?? undefined,
        target_group_id: target.target_group_id ?? undefined,
        notify_on: target.notify_on,
      })),
    );
    setFormError(null);
    setFormOpen(true);
  }

  function updateStep(index: number, patch: Partial<StepTemplateInput>) {
    setSteps((current) => current.map((step, i) => (i === index ? { ...step, ...patch } : step)));
  }

  function updateTarget(index: number, patch: Partial<NotificationTargetInput>) {
    setTargets((current) => current.map((target, i) => (i === index ? { ...target, ...patch } : target)));
  }

  const canSubmitTemplate = useMemo(
    () =>
      name.trim() !== "" &&
      requestTypePattern.test(requestType.trim()) &&
      steps.length > 0 &&
      steps.every(stepValid) &&
      targets.every(targetValid),
    [name, requestType, steps, targets],
  );

  async function submitTemplate(event: FormEvent) {
    event.preventDefault();
    if (!canSubmitTemplate) return;
    setSaving(true);
    setFormError(null);
    try {
      if (editingTemplateID) {
        const updated = await updateFlowTemplate(editingTemplateID, { name, status, steps, notification_targets: targets });
        setTemplates((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      } else {
        const created = await createFlowTemplate({ name, request_type: requestType.trim(), steps, notification_targets: targets });
        setTemplates((current) => [created, ...current]);
      }
      setFormOpen(false);
    } catch {
      setFormError(t("global.k_ApprovalsManage_SaveError"));
    } finally {
      setSaving(false);
    }
  }

  async function removeTemplate(template: ApprovalFlowTemplate) {
    if (!window.confirm(t("global.k_ApprovalsManage_DeleteConfirm"))) return;
    setError(false);
    try {
      await deleteFlowTemplate(template.id);
      setTemplates((current) => current.filter((item) => item.id !== template.id));
    } catch {
      setError(true);
    }
  }

  function toggleReassignTarget(requestItem: ApprovalRequest) {
    if (reassigningID === requestItem.id) {
      setReassigningID(null);
      return;
    }
    setReassigningID(requestItem.id);
    setReassignSelection([]);
    setReassignError(null);
  }

  function toggleReassignUser(userId: string) {
    setReassignSelection((current) =>
      current.includes(userId) ? current.filter((id) => id !== userId) : [...current, userId],
    );
  }

  async function submitReassign(requestItem: ApprovalRequest) {
    if (!reassignSelection.length) return;
    setReassignSaving(true);
    setReassignError(null);
    try {
      const updated = await reassignApprovalRequest(requestItem.id, { user_ids: reassignSelection });
      setRequests((current) => current.map((item) => (item.id === updated.id ? updated : item)));
      setReassigningID(null);
    } catch {
      setReassignError(t("global.k_ApprovalsManage_ReassignError"));
    } finally {
      setReassignSaving(false);
    }
  }

  return (
    <section className="approvals-page">
      <header className="approvals-page__header">
        <div>
          <NexText variant="title">{t("global.k_ApprovalsManage_Title")}</NexText>
          <NexText color="muted">{t("global.k_ApprovalsManage_Description")}</NexText>
        </div>
        <Link className="salary-download" to="/approvals">
          {t("global.k_ApprovalsManage_Back")}
        </Link>
      </header>

      {error ? (
        <div className="approvals-alert approvals-alert--error" role="alert">
          <NexText color="danger">{t("global.k_ApprovalsManage_TemplatesError")}</NexText>
        </div>
      ) : null}
      {loading ? (
        <div className="approvals-loading">
          <NexText color="muted">{t("global.k_Common_Loading")}</NexText>
        </div>
      ) : null}

      {canTemplates ? (
        <div className="approvals-section">
          <div className="approvals-section__header">
            <div>
              <NexText variant="subheading">{t("global.k_ApprovalsManage_TemplatesTitle")}</NexText>
              <NexText color="muted">{t("global.k_ApprovalsManage_TemplatesDescription")}</NexText>
            </div>
            <NexButton type="button" onClick={() => (formOpen && !editingTemplateID ? setFormOpen(false) : openCreate())}>
              {t("global.k_ApprovalsManage_NewTemplate")}
            </NexButton>
          </div>

          {!loading && !templates.length ? (
            <div className="approvals-empty">
              <NexText color="muted">{t("global.k_ApprovalsManage_TemplatesEmpty")}</NexText>
            </div>
          ) : null}

          {templates.length ? (
            <div className="approvals-table-wrap">
              <table className="approvals-table">
                <thead>
                  <tr>
                    <th>{t("global.k_ApprovalsManage_TemplateName")}</th>
                    <th>{t("global.k_ApprovalsManage_TemplateRequestType")}</th>
                    <th>{t("global.k_ApprovalsManage_StepsTitle")}</th>
                    <th>{t("global.k_ApprovalsManage_TemplateStatus")}</th>
                    <th>{t("global.k_Common_Actions")}</th>
                  </tr>
                </thead>
                <tbody>
                  {templates.map((template) => (
                    <tr key={template.id}>
                      <td data-label={t("global.k_ApprovalsManage_TemplateName")}>{template.name}</td>
                      <td data-label={t("global.k_ApprovalsManage_TemplateRequestType")}>{template.request_type}</td>
                      <td data-label={t("global.k_ApprovalsManage_StepsTitle")}>{template.steps.length}</td>
                      <td data-label={t("global.k_ApprovalsManage_TemplateStatus")}>
                        {t(`global.k_ApprovalsManage_TemplateStatus_${template.status}`)}
                      </td>
                      <td data-label={t("global.k_Common_Actions")}>
                        <NexButton type="button" size="compact" variant="secondary" onClick={() => openEdit(template)}>
                          {t("global.k_Common_Edit")}
                        </NexButton>{" "}
                        <NexButton type="button" size="compact" variant="danger" onClick={() => void removeTemplate(template)}>
                          {t("global.k_Common_Delete")}
                        </NexButton>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}

          {formOpen ? (
            <form className="approvals-form" onSubmit={(event) => void submitTemplate(event)}>
              {formError ? (
                <div className="approvals-alert approvals-alert--error approvals-form__full" role="alert">
                  <NexText color="danger">{formError}</NexText>
                </div>
              ) : null}
              <NexInput
                id="approvals-template-name"
                label={t("global.k_ApprovalsManage_TemplateName")}
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
              <div>
                <NexInput
                  id="approvals-template-request-type"
                  label={t("global.k_ApprovalsManage_TemplateRequestType")}
                  required
                  disabled={editingTemplateID !== null}
                  value={requestType}
                  onChange={(event) => setRequestType(event.target.value)}
                />
                <NexText variant="caption" color="muted">
                  {t("global.k_ApprovalsManage_TemplateRequestTypeHint")}
                </NexText>
              </div>
              {editingTemplateID ? (
                <label className="approvals-select-field">
                  <NexText as="span" variant="label">
                    {t("global.k_ApprovalsManage_TemplateStatus")}
                  </NexText>
                  <NexSelect
                    ariaLabel={t("global.k_ApprovalsManage_TemplateStatus")}
                    value={status}
                    options={[
                      { value: "active", label: t("global.k_ApprovalsManage_TemplateStatus_active") },
                      { value: "inactive", label: t("global.k_ApprovalsManage_TemplateStatus_inactive") },
                    ]}
                    onChange={setStatus}
                  />
                </label>
              ) : null}

              <div className="approvals-form__full approvals-builder">
                <div className="approvals-section__header">
                  <NexText variant="label">{t("global.k_ApprovalsManage_StepsTitle")}</NexText>
                  <NexButton type="button" size="compact" variant="secondary" onClick={() => setSteps((current) => [...current, emptyStep()])}>
                    {t("global.k_ApprovalsManage_AddStep")}
                  </NexButton>
                </div>
                {steps.map((step, index) => (
                  <div className="approvals-builder-row" key={index}>
                    <label className="approvals-select-field">
                      <NexText as="span" variant="label">
                        {t("global.k_ApprovalsManage_StepOrder")} {index + 1} — {t("global.k_ApprovalsManage_ApproverType")}
                      </NexText>
                      <NexSelect
                        ariaLabel={t("global.k_ApprovalsManage_ApproverType")}
                        value={step.approver_type}
                        options={approverTypes.map((value) => ({ value, label: t(`global.k_Approvals_Actor_${value}`) }))}
                        onChange={(value) =>
                          updateStep(index, {
                            approver_type: value,
                            approver_user_id: undefined,
                            approver_role_id: undefined,
                            approver_group_id: undefined,
                          })
                        }
                      />
                    </label>
                    {step.approver_type === "specific_user" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverUser")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverUser")}
                          value={step.approver_user_id ?? ""}
                          options={users.map((item) => ({ value: item.id, label: item.display_name }))}
                          onChange={(value) => updateStep(index, { approver_user_id: value })}
                        />
                      </label>
                    ) : null}
                    {step.approver_type === "role" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverRole")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverRole")}
                          value={step.approver_role_id ?? ""}
                          options={roles.map((item) => ({ value: item.id, label: item.title }))}
                          onChange={(value) => updateStep(index, { approver_role_id: value })}
                        />
                      </label>
                    ) : null}
                    {step.approver_type === "group_manager" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverGroup")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverGroup")}
                          value={step.approver_group_id ?? ""}
                          options={groups.map((item) => ({ value: item.id, label: item.name }))}
                          onChange={(value) => updateStep(index, { approver_group_id: value })}
                        />
                      </label>
                    ) : null}
                    <NexInput
                      id={`approvals-step-min-amount-${index}`}
                      label={t("global.k_ApprovalsManage_MinAmount")}
                      inputMode="decimal"
                      value={step.min_amount ?? ""}
                      onChange={(event) => updateStep(index, { min_amount: event.target.value || undefined })}
                    />
                    <NexButton
                      type="button"
                      size="compact"
                      variant="danger"
                      disabled={steps.length <= 1}
                      onClick={() => setSteps((current) => current.filter((_, i) => i !== index))}
                    >
                      {t("global.k_ApprovalsManage_RemoveStep")}
                    </NexButton>
                  </div>
                ))}
              </div>

              <div className="approvals-form__full approvals-builder">
                <div className="approvals-section__header">
                  <div>
                    <NexText variant="label">{t("global.k_ApprovalsManage_NotificationTargetsTitle")}</NexText>
                    <NexText color="muted" variant="caption">
                      {t("global.k_ApprovalsManage_NotificationTargetsDescription")}
                    </NexText>
                  </div>
                  <NexButton type="button" size="compact" variant="secondary" onClick={() => setTargets((current) => [...current, emptyTarget()])}>
                    {t("global.k_ApprovalsManage_AddTarget")}
                  </NexButton>
                </div>
                {targets.map((target, index) => (
                  <div className="approvals-builder-row" key={index}>
                    <label className="approvals-select-field">
                      <NexText as="span" variant="label">{t("global.k_ApprovalsManage_TargetType")}</NexText>
                      <NexSelect
                        ariaLabel={t("global.k_ApprovalsManage_TargetType")}
                        value={target.target_type}
                        options={targetTypes.map((value) => ({ value, label: t(`global.k_Approvals_Actor_${value}`) }))}
                        onChange={(value) =>
                          updateTarget(index, { target_type: value, target_user_id: undefined, target_role_id: undefined, target_group_id: undefined })
                        }
                      />
                    </label>
                    {target.target_type === "specific_user" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverUser")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverUser")}
                          value={target.target_user_id ?? ""}
                          options={users.map((item) => ({ value: item.id, label: item.display_name }))}
                          onChange={(value) => updateTarget(index, { target_user_id: value })}
                        />
                      </label>
                    ) : null}
                    {target.target_type === "role" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverRole")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverRole")}
                          value={target.target_role_id ?? ""}
                          options={roles.map((item) => ({ value: item.id, label: item.title }))}
                          onChange={(value) => updateTarget(index, { target_role_id: value })}
                        />
                      </label>
                    ) : null}
                    {target.target_type === "group_manager" ? (
                      <label className="approvals-select-field">
                        <NexText as="span" variant="label">{t("global.k_ApprovalsManage_ApproverGroup")}</NexText>
                        <NexSelect
                          ariaLabel={t("global.k_ApprovalsManage_ApproverGroup")}
                          value={target.target_group_id ?? ""}
                          options={groups.map((item) => ({ value: item.id, label: item.name }))}
                          onChange={(value) => updateTarget(index, { target_group_id: value })}
                        />
                      </label>
                    ) : null}
                    <label className="approvals-select-field">
                      <NexText as="span" variant="label">{t("global.k_ApprovalsManage_NotifyOn")}</NexText>
                      <NexSelect
                        ariaLabel={t("global.k_ApprovalsManage_NotifyOn")}
                        value={target.notify_on}
                        options={notifyOnOptions.map((value) => ({ value, label: t(`global.k_ApprovalsManage_NotifyOn_${value}`) }))}
                        onChange={(value) => updateTarget(index, { notify_on: value })}
                      />
                    </label>
                    <NexButton
                      type="button"
                      size="compact"
                      variant="danger"
                      onClick={() => setTargets((current) => current.filter((_, i) => i !== index))}
                    >
                      {t("global.k_ApprovalsManage_RemoveTarget")}
                    </NexButton>
                  </div>
                ))}
              </div>

              <div className="approvals-form__full approvals-card__actions">
                <NexButton type="button" variant="secondary" onClick={() => setFormOpen(false)}>
                  {t("global.k_Common_Cancel")}
                </NexButton>
                <NexButton type="submit" disabled={saving || !canSubmitTemplate}>
                  {saving ? t("global.k_Common_Saving") : editingTemplateID ? t("global.k_ApprovalsManage_Update") : t("global.k_ApprovalsManage_Create")}
                </NexButton>
              </div>
            </form>
          ) : null}
        </div>
      ) : null}

      {canReadAll ? (
        <div className="approvals-section">
          <div className="approvals-section__header">
            <div>
              <NexText variant="subheading">{t("global.k_ApprovalsManage_RequestsTitle")}</NexText>
              <NexText color="muted">{t("global.k_ApprovalsManage_RequestsDescription")}</NexText>
            </div>
            <label className="approvals-select-field">
              <NexText as="span" variant="label">{t("global.k_ApprovalsManage_RequestsFilter")}</NexText>
              <NexSelect
                ariaLabel={t("global.k_ApprovalsManage_RequestsFilter")}
                value={statusFilter}
                options={statusFilters.map((value) => ({
                  value,
                  label: value === "all" ? t("global.k_ApprovalsManage_RequestsFilterAll") : t(`global.k_Approvals_Status_${value}`),
                }))}
                onChange={setStatusFilter}
              />
            </label>
          </div>

          {requestsError ? (
            <div className="approvals-alert approvals-alert--error" role="alert">
              <NexText color="danger">{t("global.k_ApprovalsManage_RequestsError")}</NexText>
            </div>
          ) : null}
          {requestsLoading ? (
            <div className="approvals-loading">
              <NexText color="muted">{t("global.k_Common_Loading")}</NexText>
            </div>
          ) : null}
          {!requestsLoading && !requests.length ? (
            <div className="approvals-empty">
              <NexText color="muted">{t("global.k_ApprovalsManage_RequestsEmpty")}</NexText>
            </div>
          ) : null}

          <div className="approvals-list">
            {requests.map((item) => (
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
                    <dt>{t("global.k_Approvals_CreatedAt")}</dt>
                    <dd>{item.created_at}</dd>
                  </div>
                </dl>
                {canReassign && item.status === "requires_assignment" ? (
                  <div className="approvals-card__actions">
                    <NexButton type="button" variant="secondary" onClick={() => toggleReassignTarget(item)}>
                      {t("global.k_ApprovalsManage_Reassign")}
                    </NexButton>
                  </div>
                ) : null}
                {reassigningID === item.id ? (
                  <div className="approvals-section">
                    <NexText variant="label">{t("global.k_ApprovalsManage_ReassignTitle")}</NexText>
                    <NexText color="muted" variant="caption">{t("global.k_ApprovalsManage_ReassignHint")}</NexText>
                    {reassignError ? <NexText color="danger">{reassignError}</NexText> : null}
                    <div className="approvals-checkbox-list">
                      {users.map((candidate) => (
                        <label key={candidate.id}>
                          <input
                            type="checkbox"
                            checked={reassignSelection.includes(candidate.id)}
                            onChange={() => toggleReassignUser(candidate.id)}
                          />
                          {candidate.display_name}
                        </label>
                      ))}
                    </div>
                    <div className="approvals-card__actions">
                      <NexButton type="button" variant="secondary" onClick={() => setReassigningID(null)}>
                        {t("global.k_Common_Cancel")}
                      </NexButton>
                      <NexButton
                        type="button"
                        disabled={reassignSaving || !reassignSelection.length}
                        onClick={() => void submitReassign(item)}
                      >
                        {reassignSaving ? t("global.k_Common_Saving") : t("global.k_ApprovalsManage_Reassign")}
                      </NexButton>
                    </div>
                  </div>
                ) : null}
              </article>
            ))}
          </div>
        </div>
      ) : null}

      {!canTemplates && !canReadAll ? (
        <div className="approvals-empty">
          <NexText color="muted">{t("global.k_ApprovalsManage_NoAccess")}</NexText>
        </div>
      ) : null}
    </section>
  );
}
