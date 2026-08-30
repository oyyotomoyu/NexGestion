export type ApprovalApproverType = "specific_user" | "role" | "requester_manager" | "group_manager";
export type ApprovalTargetType = "specific_user" | "role" | "group_manager";
export type ApprovalNotifyOn = "approved" | "rejected" | "both";
export type ApprovalFlowTemplateStatus = "active" | "inactive";
export type ApprovalRequestStatus = "pending" | "approved" | "rejected" | "cancelled" | "requires_assignment";
export type ApprovalStepDecision = "pending" | "approved" | "rejected" | "skipped";

export interface ApprovalStepTemplate {
  id: string;
  flow_template_id: string;
  step_order: number;
  approver_type: ApprovalApproverType;
  approver_user_id: string | null;
  approver_role_id: string | null;
  approver_group_id: string | null;
  min_amount: string | null;
}

export interface ApprovalNotificationTarget {
  id: string;
  flow_template_id: string;
  target_type: ApprovalTargetType;
  target_user_id: string | null;
  target_role_id: string | null;
  target_group_id: string | null;
  notify_on: ApprovalNotifyOn;
}

export interface ApprovalFlowTemplate {
  id: string;
  name: string;
  request_type: string;
  status: ApprovalFlowTemplateStatus;
  created_at: string;
  steps: ApprovalStepTemplate[];
  notification_targets: ApprovalNotificationTarget[];
}

export interface ApprovalStep {
  id: string;
  approval_request_id: string;
  step_order: number;
  assigned_user_ids: string[];
  decision: ApprovalStepDecision;
  decided_by_user_id: string | null;
  decided_at: string | null;
  comment: string;
}

export interface ApprovalRequest {
  id: string;
  flow_template_id: string;
  source_module: string;
  source_reference_id: string;
  requested_by_user_id: string;
  amount: string | null;
  status: ApprovalRequestStatus;
  current_step_order: number;
  created_at: string;
  completed_at: string | null;
  steps: ApprovalStep[];
}

export interface StepTemplateInput {
  approver_type: ApprovalApproverType;
  approver_user_id?: string;
  approver_role_id?: string;
  approver_group_id?: string;
  min_amount?: string;
}

export interface NotificationTargetInput {
  target_type: ApprovalTargetType;
  target_user_id?: string;
  target_role_id?: string;
  target_group_id?: string;
  notify_on: ApprovalNotifyOn;
}

export interface CreateFlowTemplateInput {
  name: string;
  request_type: string;
  steps: StepTemplateInput[];
  notification_targets: NotificationTargetInput[];
}

export interface UpdateFlowTemplateInput {
  name?: string;
  status?: ApprovalFlowTemplateStatus;
  steps?: StepTemplateInput[];
  notification_targets?: NotificationTargetInput[];
}

export interface SubmitApprovalRequestInput {
  flow_template_id: string;
  source_module: string;
  source_reference_id: string;
  amount?: string;
}

export interface DecideApprovalInput {
  decision: "approved" | "rejected";
  comment?: string;
}

export interface ReassignApprovalInput {
  user_ids: string[];
}
