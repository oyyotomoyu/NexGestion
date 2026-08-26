package system

import (
	"context"
	"testing"
	"time"
)

func testApprovalService(t *testing.T) (*ApprovalService, *NotificationService, *UserService) {
	t.Helper()
	directory := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	if err := EnsureRequiredDatabases(context.Background(), directory); err != nil {
		t.Fatal(err)
	}
	users := NewUserService(directory)
	notifications := NewNotificationService(directory, users)
	approvals := NewApprovalService(directory, users, notifications)
	approvals.now = func() time.Time {
		return time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	}
	return approvals, notifications, users
}

func createTestPlainUser(t *testing.T, users *UserService, email string) string {
	t.Helper()
	user, err := users.Create(context.Background(), adminUserID, CreateUserInput{
		DisplayName: email, Email: email, Password: "a-secure-user-password", Status: "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	return user.ID
}

func notificationTitles(t *testing.T, notifications *NotificationService, userID string) []string {
	t.Helper()
	inbox, err := notifications.ListForUser(context.Background(), userID, ListQuery{PageSize: MaxPageSize})
	if err != nil {
		t.Fatalf("list notifications for %s: %v", userID, err)
	}
	titles := make([]string, 0, len(inbox.Items))
	for _, item := range inbox.Items {
		titles = append(titles, item.Title)
	}
	return titles
}

// TestApprovalNotifiesAssigneeThenRequesterAndTargetsOnApproval covers
// docs/System/approval-system.md Section 4.3 and Section 3: the assigned
// approver is notified once the step becomes current, and once the request
// reaches a final decision both the requester and any matching Completion
// Notification Target (Section 2.5) are notified.
func TestApprovalNotifiesAssigneeThenRequesterAndTargetsOnApproval(t *testing.T) {
	approvals, notifications, users := testApprovalService(t)
	ctx := context.Background()

	requester := createTestPlainUser(t, users, "requester@example.com")
	approver := createTestPlainUser(t, users, "approver@example.com")
	watcher := createTestPlainUser(t, users, "watcher@example.com")

	template, err := approvals.CreateFlowTemplate(ctx, adminUserID, CreateFlowTemplateInput{
		Name:        "Office Supply Requisition",
		RequestType: "general_affairs.supply_requisition",
		Steps: []StepTemplateInput{
			{ApproverType: ApprovalApproverSpecificUser, ApproverUserID: approver},
		},
		NotificationTargets: []NotificationTargetInput{
			{TargetType: ApprovalTargetSpecificUser, TargetUserID: watcher, NotifyOn: ApprovalNotifyOnApproved},
		},
	})
	if err != nil {
		t.Fatalf("create flow template: %v", err)
	}

	request, err := approvals.SubmitRequest(ctx, requester, SubmitApprovalRequestInput{
		FlowTemplateID:    template.ID,
		SourceModule:      "general_affairs",
		SourceReferenceID: "requisition-1",
	})
	if err != nil {
		t.Fatalf("submit request: %v", err)
	}
	if request.Status != ApprovalRequestPending {
		t.Fatalf("expected pending status, got %q", request.Status)
	}

	approverTitles := notificationTitles(t, notifications, approver)
	if len(approverTitles) != 1 || approverTitles[0] != "Approval needed: Office Supply Requisition" {
		t.Fatalf("expected approver to be notified of the pending step, got %v", approverTitles)
	}
	if titles := notificationTitles(t, notifications, requester); len(titles) != 0 {
		t.Fatalf("requester should not be notified before a decision, got %v", titles)
	}

	decided, err := approvals.DecideRequest(ctx, approver, request.ID, DecideApprovalInput{Decision: ApprovalStepApproved})
	if err != nil {
		t.Fatalf("decide request: %v", err)
	}
	if decided.Status != ApprovalRequestApproved {
		t.Fatalf("expected approved status, got %q", decided.Status)
	}

	requesterTitles := notificationTitles(t, notifications, requester)
	if len(requesterTitles) != 1 || requesterTitles[0] != "Request approved: Office Supply Requisition" {
		t.Fatalf("expected requester to be notified of the approval, got %v", requesterTitles)
	}
	watcherTitles := notificationTitles(t, notifications, watcher)
	if len(watcherTitles) != 1 || watcherTitles[0] != "Approval approved: Office Supply Requisition" {
		t.Fatalf("expected completion notification target to be notified, got %v", watcherTitles)
	}
}

// TestApprovalNotifiesRequesterOnRejectionOnly covers the notify_on=rejected
// half of Section 2.5, and that a rejection never notifies an
// approved-only target.
func TestApprovalNotifiesRequesterOnRejectionOnly(t *testing.T) {
	approvals, notifications, users := testApprovalService(t)
	ctx := context.Background()

	requester := createTestPlainUser(t, users, "requester2@example.com")
	approver := createTestPlainUser(t, users, "approver2@example.com")
	watcher := createTestPlainUser(t, users, "watcher2@example.com")

	template, err := approvals.CreateFlowTemplate(ctx, adminUserID, CreateFlowTemplateInput{
		Name:        "Seal Usage Request",
		RequestType: "general_affairs.seal_usage",
		Steps: []StepTemplateInput{
			{ApproverType: ApprovalApproverSpecificUser, ApproverUserID: approver},
		},
		NotificationTargets: []NotificationTargetInput{
			{TargetType: ApprovalTargetSpecificUser, TargetUserID: watcher, NotifyOn: ApprovalNotifyOnApproved},
		},
	})
	if err != nil {
		t.Fatalf("create flow template: %v", err)
	}

	request, err := approvals.SubmitRequest(ctx, requester, SubmitApprovalRequestInput{
		FlowTemplateID:    template.ID,
		SourceModule:      "general_affairs",
		SourceReferenceID: "seal-1",
	})
	if err != nil {
		t.Fatalf("submit request: %v", err)
	}

	decided, err := approvals.DecideRequest(ctx, approver, request.ID, DecideApprovalInput{Decision: ApprovalStepRejected, Comment: "not needed"})
	if err != nil {
		t.Fatalf("decide request: %v", err)
	}
	if decided.Status != ApprovalRequestRejected {
		t.Fatalf("expected rejected status, got %q", decided.Status)
	}

	requesterTitles := notificationTitles(t, notifications, requester)
	if len(requesterTitles) != 1 || requesterTitles[0] != "Request rejected: Seal Usage Request" {
		t.Fatalf("expected requester to be notified of the rejection, got %v", requesterTitles)
	}
	if titles := notificationTitles(t, notifications, watcher); len(titles) != 0 {
		t.Fatalf("approved-only target should not be notified on rejection, got %v", titles)
	}
}
