import { request } from "@/requests/core/client";
import { buildListPath, listItems, type ListQuery, type ListResponse } from "@/requests/core/list";
import type {
  ApprovalFlowTemplate,
  ApprovalRequest,
  CreateFlowTemplateInput,
  DecideApprovalInput,
  ReassignApprovalInput,
  SubmitApprovalRequestInput,
  UpdateFlowTemplateInput,
} from "./types";

const CURRENT_USER_ID = "0";

let mockTemplates: ApprovalFlowTemplate[] = [
  {
    id: "template-supply",
    name: "Office supply requisition",
    request_type: "general_affairs.supply_requisition",
    status: "active",
    created_at: "2026-01-05T02:00:00.000Z",
    steps: [
      {
        id: "step-1",
        flow_template_id: "template-supply",
        step_order: 1,
        approver_type: "requester_manager",
        approver_user_id: null,
        approver_role_id: null,
        approver_group_id: null,
        min_amount: null,
      },
    ],
    notification_targets: [],
  },
];

let mockRequests: ApprovalRequest[] = [
  {
    id: "request-1",
    flow_template_id: "template-supply",
    source_module: "general_affairs",
    source_reference_id: "requisition-100",
    requested_by_user_id: "1",
    amount: null,
    status: "pending",
    current_step_order: 1,
    created_at: "2026-08-01T03:00:00.000Z",
    completed_at: null,
    steps: [
      {
        id: "request-1-step-1",
        approval_request_id: "request-1",
        step_order: 1,
        assigned_user_ids: [CURRENT_USER_ID],
        decision: "pending",
        decided_by_user_id: null,
        decided_at: null,
        comment: "",
      },
    ],
  },
];

function nowISO() {
  return new Date().toISOString();
}

function nextStepOrder(template: ApprovalFlowTemplate) {
  return template.steps.length + 1;
}

export async function listFlowTemplates(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockTemplates];
  const response = await request<ListResponse<ApprovalFlowTemplate, "flow_templates">>(
    buildListPath("/api/approvals/templates", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "flow_templates");
}

export function getFlowTemplate(id: string) {
  if (import.meta.env.DEV) {
    const template = mockTemplates.find((item) => item.id === id);
    return template ? Promise.resolve({ ...template }) : Promise.reject(new Error("Flow template not found"));
  }
  return request<ApprovalFlowTemplate>(`/api/approvals/templates/${encodeURIComponent(id)}`);
}

export function createFlowTemplate(input: CreateFlowTemplateInput) {
  if (import.meta.env.DEV) {
    const id = crypto.randomUUID();
    const template: ApprovalFlowTemplate = {
      id,
      name: input.name.trim(),
      request_type: input.request_type.trim(),
      status: "active",
      created_at: nowISO(),
      steps: input.steps.map((step, index) => ({
        id: crypto.randomUUID(),
        flow_template_id: id,
        step_order: index + 1,
        approver_type: step.approver_type,
        approver_user_id: step.approver_user_id ?? null,
        approver_role_id: step.approver_role_id ?? null,
        approver_group_id: step.approver_group_id ?? null,
        min_amount: step.min_amount ?? null,
      })),
      notification_targets: input.notification_targets.map((target) => ({
        id: crypto.randomUUID(),
        flow_template_id: id,
        target_type: target.target_type,
        target_user_id: target.target_user_id ?? null,
        target_role_id: target.target_role_id ?? null,
        target_group_id: target.target_group_id ?? null,
        notify_on: target.notify_on,
      })),
    };
    mockTemplates = [...mockTemplates, template];
    return Promise.resolve({ ...template });
  }
  return request<ApprovalFlowTemplate>("/api/approvals/templates", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateFlowTemplate(id: string, input: UpdateFlowTemplateInput) {
  if (import.meta.env.DEV) {
    const index = mockTemplates.findIndex((item) => item.id === id);
    if (index < 0) return Promise.reject(new Error("Flow template not found"));
    const current = mockTemplates[index];
    const updated: ApprovalFlowTemplate = {
      ...current,
      name: input.name?.trim() || current.name,
      status: input.status ?? current.status,
      steps: input.steps
        ? input.steps.map((step, stepIndex) => ({
            id: crypto.randomUUID(),
            flow_template_id: id,
            step_order: stepIndex + 1,
            approver_type: step.approver_type,
            approver_user_id: step.approver_user_id ?? null,
            approver_role_id: step.approver_role_id ?? null,
            approver_group_id: step.approver_group_id ?? null,
            min_amount: step.min_amount ?? null,
          }))
        : current.steps,
      notification_targets: input.notification_targets
        ? input.notification_targets.map((target) => ({
            id: crypto.randomUUID(),
            flow_template_id: id,
            target_type: target.target_type,
            target_user_id: target.target_user_id ?? null,
            target_role_id: target.target_role_id ?? null,
            target_group_id: target.target_group_id ?? null,
            notify_on: target.notify_on,
          }))
        : current.notification_targets,
    };
    mockTemplates = mockTemplates.map((item) => (item.id === id ? updated : item));
    return Promise.resolve({ ...updated });
  }
  return request<ApprovalFlowTemplate>(`/api/approvals/templates/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteFlowTemplate(id: string) {
  if (import.meta.env.DEV) {
    if (mockRequests.some((item) => item.flow_template_id === id)) {
      return Promise.reject(new Error("Flow template is in use"));
    }
    mockTemplates = mockTemplates.filter((item) => item.id !== id);
    return Promise.resolve();
  }
  return request<void>(`/api/approvals/templates/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function submitApprovalRequest(input: SubmitApprovalRequestInput) {
  if (import.meta.env.DEV) {
    const template = mockTemplates.find((item) => item.id === input.flow_template_id);
    if (!template) return Promise.reject(new Error("Flow template not found"));
    const firstStep = template.steps[0] ?? null;
    const created: ApprovalRequest = {
      id: crypto.randomUUID(),
      flow_template_id: input.flow_template_id,
      source_module: input.source_module.trim(),
      source_reference_id: input.source_reference_id.trim(),
      requested_by_user_id: CURRENT_USER_ID,
      amount: input.amount ?? null,
      status: firstStep ? "pending" : "requires_assignment",
      current_step_order: firstStep?.step_order ?? 1,
      created_at: nowISO(),
      completed_at: null,
      steps: firstStep
        ? [
            {
              id: crypto.randomUUID(),
              approval_request_id: "",
              step_order: firstStep.step_order,
              assigned_user_ids: firstStep.approver_user_id ? [firstStep.approver_user_id] : [CURRENT_USER_ID],
              decision: "pending",
              decided_by_user_id: null,
              decided_at: null,
              comment: "",
            },
          ]
        : [],
    };
    created.steps = created.steps.map((step) => ({ ...step, approval_request_id: created.id }));
    mockRequests = [created, ...mockRequests];
    return Promise.resolve({ ...created });
  }
  return request<ApprovalRequest>("/api/approvals/requests", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function listApprovalRequests(query: ListQuery = {}) {
  if (import.meta.env.DEV) return [...mockRequests];
  const response = await request<ListResponse<ApprovalRequest, "approval_requests">>(
    buildListPath("/api/approvals/requests", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "approval_requests");
}

export async function listMyApprovalRequests(query: ListQuery = {}) {
  if (import.meta.env.DEV) return mockRequests.filter((item) => item.requested_by_user_id === CURRENT_USER_ID);
  const response = await request<ListResponse<ApprovalRequest, "approval_requests">>(
    buildListPath("/api/approvals/me/requests", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "approval_requests");
}

export async function listMyApprovalAssignments(query: ListQuery = {}) {
  if (import.meta.env.DEV) {
    return mockRequests.filter((item) =>
      item.steps.some((step) => step.decision === "pending" && step.assigned_user_ids.includes(CURRENT_USER_ID)),
    );
  }
  const response = await request<ListResponse<ApprovalRequest, "approval_requests">>(
    buildListPath("/api/approvals/me/assignments", { sort: "created_at", order: "desc", page_size: 100, ...query }),
  );
  return listItems(response, "approval_requests");
}

export function getApprovalRequest(id: string) {
  if (import.meta.env.DEV) {
    const found = mockRequests.find((item) => item.id === id);
    return found ? Promise.resolve({ ...found }) : Promise.reject(new Error("Approval request not found"));
  }
  return request<ApprovalRequest>(`/api/approvals/requests/${encodeURIComponent(id)}`);
}

export function cancelApprovalRequest(id: string) {
  if (import.meta.env.DEV) {
    const index = mockRequests.findIndex((item) => item.id === id);
    if (index < 0) return Promise.reject(new Error("Approval request not found"));
    const updated: ApprovalRequest = { ...mockRequests[index], status: "cancelled", completed_at: nowISO() };
    mockRequests = mockRequests.map((item) => (item.id === id ? updated : item));
    return Promise.resolve({ ...updated });
  }
  return request<ApprovalRequest>(`/api/approvals/requests/${encodeURIComponent(id)}/cancel`, { method: "POST" });
}

export function decideApprovalRequest(id: string, input: DecideApprovalInput) {
  if (import.meta.env.DEV) {
    const index = mockRequests.findIndex((item) => item.id === id);
    if (index < 0) return Promise.reject(new Error("Approval request not found"));
    const current = mockRequests[index];
    const template = mockTemplates.find((item) => item.id === current.flow_template_id) ?? null;
    const decidedAt = nowISO();
    const steps = current.steps.map((step) =>
      step.step_order === current.current_step_order && step.decision === "pending"
        ? {
            ...step,
            decision: input.decision,
            decided_by_user_id: CURRENT_USER_ID,
            decided_at: decidedAt,
            comment: input.comment ?? "",
          }
        : step,
    );
    let status = current.status;
    let currentStepOrder = current.current_step_order;
    let completedAt: string | null = current.completed_at;
    if (input.decision === "rejected") {
      status = "rejected";
      completedAt = decidedAt;
    } else {
      const nextTemplateStep = template?.steps.find((step) => step.step_order === current.current_step_order + 1);
      if (nextTemplateStep) {
        currentStepOrder = nextTemplateStep.step_order;
        steps.push({
          id: crypto.randomUUID(),
          approval_request_id: id,
          step_order: nextTemplateStep.step_order,
          assigned_user_ids: nextTemplateStep.approver_user_id ? [nextTemplateStep.approver_user_id] : [CURRENT_USER_ID],
          decision: "pending",
          decided_by_user_id: null,
          decided_at: null,
          comment: "",
        });
      } else {
        status = "approved";
        completedAt = decidedAt;
      }
    }
    const updated: ApprovalRequest = { ...current, status, current_step_order: currentStepOrder, completed_at: completedAt, steps };
    mockRequests = mockRequests.map((item) => (item.id === id ? updated : item));
    return Promise.resolve({ ...updated });
  }
  return request<ApprovalRequest>(`/api/approvals/requests/${encodeURIComponent(id)}/decide`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function reassignApprovalRequest(id: string, input: ReassignApprovalInput) {
  if (import.meta.env.DEV) {
    const index = mockRequests.findIndex((item) => item.id === id);
    if (index < 0) return Promise.reject(new Error("Approval request not found"));
    const current = mockRequests[index];
    const steps = current.steps.map((step) =>
      step.step_order === current.current_step_order ? { ...step, assigned_user_ids: [...input.user_ids] } : step,
    );
    const updated: ApprovalRequest = { ...current, status: "pending", steps };
    mockRequests = mockRequests.map((item) => (item.id === id ? updated : item));
    return Promise.resolve({ ...updated });
  }
  return request<ApprovalRequest>(`/api/approvals/requests/${encodeURIComponent(id)}/reassign`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}
