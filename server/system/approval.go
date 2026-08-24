package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Approver/target resolution kinds. See docs/System/approval-system.md
// Section 2.2 and 2.5.
const (
	ApprovalApproverSpecificUser     = "specific_user"
	ApprovalApproverRole             = "role"
	ApprovalApproverRequesterManager = "requester_manager"
	ApprovalApproverGroupManager     = "group_manager"
)

const (
	ApprovalTargetSpecificUser = "specific_user"
	ApprovalTargetRole         = "role"
	ApprovalTargetGroupManager = "group_manager"
)

const (
	ApprovalNotifyOnApproved = "approved"
	ApprovalNotifyOnRejected = "rejected"
	ApprovalNotifyOnBoth     = "both"
)

// Request lifecycle states. See docs/System/approval-system.md Section 3.
const (
	ApprovalRequestPending            = "pending"
	ApprovalRequestApproved           = "approved"
	ApprovalRequestRejected           = "rejected"
	ApprovalRequestCancelled          = "cancelled"
	ApprovalRequestRequiresAssignment = "requires_assignment"
)

// Step decision states. See docs/System/approval-system.md Section 2.4.
const (
	ApprovalStepPending  = "pending"
	ApprovalStepApproved = "approved"
	ApprovalStepRejected = "rejected"
	ApprovalStepSkipped  = "skipped"
)

var (
	ErrApprovalNotFound      = errors.New("approval record not found")
	ErrApprovalInvalidInput  = errors.New("invalid approval input")
	ErrApprovalDecision      = errors.New("approval step cannot be decided")
	ErrApprovalPermission    = errors.New("approval permission denied")
	ErrApprovalTemplateInUse = errors.New("approval flow template is in use")
)

var (
	approvalAmountPattern      = regexp.MustCompile(`^\d+(\.\d{1,6})?$`)
	approvalRequestTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
)

// ApprovalFlowTemplate is an organization-defined, reusable approval sequence
// for one kind of request. See docs/System/approval-system.md Section 2.1.
type ApprovalFlowTemplate struct {
	ID                  string                       `json:"id"`
	Name                string                       `json:"name"`
	RequestType         string                       `json:"request_type"`
	Status              string                       `json:"status"`
	CreatedAt           string                       `json:"created_at"`
	Steps               []ApprovalStepTemplate       `json:"steps"`
	NotificationTargets []ApprovalNotificationTarget `json:"notification_targets"`
}

// ApprovalStepTemplate is one ordered step within a Flow Template. See
// docs/System/approval-system.md Section 2.2.
type ApprovalStepTemplate struct {
	ID              string  `json:"id"`
	FlowTemplateID  string  `json:"flow_template_id"`
	StepOrder       int     `json:"step_order"`
	ApproverType    string  `json:"approver_type"`
	ApproverUserID  *string `json:"approver_user_id"`
	ApproverRoleID  *string `json:"approver_role_id"`
	ApproverGroupID *string `json:"approver_group_id"`
	MinAmount       *string `json:"min_amount"`
}

// ApprovalNotificationTarget is who is told once a request reaches a final
// decision, beyond the requester. See docs/System/approval-system.md Section
// 2.5.
type ApprovalNotificationTarget struct {
	ID             string  `json:"id"`
	FlowTemplateID string  `json:"flow_template_id"`
	TargetType     string  `json:"target_type"`
	TargetUserID   *string `json:"target_user_id"`
	TargetRoleID   *string `json:"target_role_id"`
	TargetGroupID  *string `json:"target_group_id"`
	NotifyOn       string  `json:"notify_on"`
}

// ApprovalRequest is one instantiated request awaiting sign-off. See
// docs/System/approval-system.md Section 2.3.
type ApprovalRequest struct {
	ID                string         `json:"id"`
	FlowTemplateID    string         `json:"flow_template_id"`
	SourceModule      string         `json:"source_module"`
	SourceReferenceID string         `json:"source_reference_id"`
	RequestedByUserID string         `json:"requested_by_user_id"`
	Amount            *string        `json:"amount"`
	Status            string         `json:"status"`
	CurrentStepOrder  int            `json:"current_step_order"`
	CreatedAt         string         `json:"created_at"`
	CompletedAt       *string        `json:"completed_at"`
	Steps             []ApprovalStep `json:"steps"`
}

// InvolvesUser reports whether userID submitted the request or is (or was)
// assigned to decide one of its steps.
func (r *ApprovalRequest) InvolvesUser(userID string) bool {
	if r.RequestedByUserID == userID {
		return true
	}
	for _, step := range r.Steps {
		for _, assignee := range step.AssignedUserIDs {
			if assignee == userID {
				return true
			}
		}
	}
	return false
}

// ApprovalStep is the decision record for one step of one request,
// snapshotted from the Step Template at the moment the step became current.
// See docs/System/approval-system.md Section 2.4.
type ApprovalStep struct {
	ID                string   `json:"id"`
	ApprovalRequestID string   `json:"approval_request_id"`
	StepOrder         int      `json:"step_order"`
	AssignedUserIDs   []string `json:"assigned_user_ids"`
	Decision          string   `json:"decision"`
	DecidedByUserID   *string  `json:"decided_by_user_id"`
	DecidedAt         *string  `json:"decided_at"`
	Comment           string   `json:"comment"`
}

type StepTemplateInput struct {
	ApproverType    string `json:"approver_type"`
	ApproverUserID  string `json:"approver_user_id"`
	ApproverRoleID  string `json:"approver_role_id"`
	ApproverGroupID string `json:"approver_group_id"`
	MinAmount       string `json:"min_amount"`
}

type NotificationTargetInput struct {
	TargetType    string `json:"target_type"`
	TargetUserID  string `json:"target_user_id"`
	TargetRoleID  string `json:"target_role_id"`
	TargetGroupID string `json:"target_group_id"`
	NotifyOn      string `json:"notify_on"`
}

type CreateFlowTemplateInput struct {
	Name                string                    `json:"name"`
	RequestType         string                    `json:"request_type"`
	Steps               []StepTemplateInput       `json:"steps"`
	NotificationTargets []NotificationTargetInput `json:"notification_targets"`
}

type UpdateFlowTemplateInput struct {
	Name                *string                    `json:"name"`
	Status              *string                    `json:"status"`
	Steps               *[]StepTemplateInput       `json:"steps"`
	NotificationTargets *[]NotificationTargetInput `json:"notification_targets"`
}

type SubmitApprovalRequestInput struct {
	FlowTemplateID    string  `json:"flow_template_id"`
	SourceModule      string  `json:"source_module"`
	SourceReferenceID string  `json:"source_reference_id"`
	Amount            *string `json:"amount"`
}

type DecideApprovalInput struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type ReassignApprovalInput struct {
	UserIDs []string `json:"user_ids"`
}

type ApprovalService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewApprovalService(databaseDirectory string, users *UserService) *ApprovalService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &ApprovalService{
		databasePath: filepath.Join(databaseDirectory, "approval.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *ApprovalService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ---- Flow templates ----

func (s *ApprovalService) CreateFlowTemplate(ctx context.Context, actorUserID string, input CreateFlowTemplateInput) (*ApprovalFlowTemplate, error) {
	name := strings.TrimSpace(input.Name)
	requestType := strings.TrimSpace(input.RequestType)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrApprovalInvalidInput)
	}
	if !approvalRequestTypePattern.MatchString(requestType) {
		return nil, fmt.Errorf("%w: request_type must use the module.request convention", ErrApprovalInvalidInput)
	}
	if len(input.Steps) == 0 {
		return nil, fmt.Errorf("%w: at least one step is required", ErrApprovalInvalidInput)
	}
	steps, err := s.normalizeStepTemplates(ctx, input.Steps)
	if err != nil {
		return nil, err
	}
	targets, err := s.normalizeNotificationTargets(ctx, input.NotificationTargets)
	if err != nil {
		return nil, err
	}

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO approval_flow_templates (id, name, request_type, status, created_at)
		VALUES (?, ?, ?, 'active', ?)`, id, name, requestType, now); err != nil {
		return nil, err
	}
	if err := insertStepTemplates(ctx, tx, id, steps); err != nil {
		return nil, err
	}
	if err := insertNotificationTargets(ctx, tx, id, targets); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getFlowTemplate(ctx, db, id)
}

func (s *ApprovalService) UpdateFlowTemplate(ctx context.Context, actorUserID, id string, input UpdateFlowTemplateInput) (*ApprovalFlowTemplate, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_flow_templates WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrApprovalNotFound
	}

	sets := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrApprovalInvalidInput)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != "active" && status != "inactive" {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrApprovalInvalidInput)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := tx.ExecContext(ctx, `UPDATE approval_flow_templates SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
			return nil, err
		}
	}
	if input.Steps != nil {
		steps, err := s.normalizeStepTemplates(ctx, *input.Steps)
		if err != nil {
			return nil, err
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("%w: at least one step is required", ErrApprovalInvalidInput)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM approval_step_templates WHERE flow_template_id=?`, id); err != nil {
			return nil, err
		}
		if err := insertStepTemplates(ctx, tx, id, steps); err != nil {
			return nil, err
		}
	}
	if input.NotificationTargets != nil {
		targets, err := s.normalizeNotificationTargets(ctx, *input.NotificationTargets)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM approval_notification_targets WHERE flow_template_id=?`, id); err != nil {
			return nil, err
		}
		if err := insertNotificationTargets(ctx, tx, id, targets); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getFlowTemplate(ctx, db, id)
}

func (s *ApprovalService) DeleteFlowTemplate(ctx context.Context, actorUserID, id string) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_flow_templates WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrApprovalNotFound
	}
	var referenced int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_requests WHERE flow_template_id=?`, id).Scan(&referenced); err != nil {
		return err
	}
	if referenced > 0 {
		return ErrApprovalTemplateInUse
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM approval_step_templates WHERE flow_template_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM approval_notification_targets WHERE flow_template_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM approval_flow_templates WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ApprovalService) GetFlowTemplate(ctx context.Context, id string) (*ApprovalFlowTemplate, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getFlowTemplate(ctx, db, id)
}

// ActiveFlowTemplateByRequestType resolves the active template a consuming
// module should instantiate for one request_type. See
// docs/System/approval-system.md Section 4.1.
func (s *ApprovalService) ActiveFlowTemplateByRequestType(ctx context.Context, requestType string) (*ApprovalFlowTemplate, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var id string
	err = db.QueryRowContext(ctx, `SELECT id FROM approval_flow_templates
		WHERE request_type=? AND status='active' ORDER BY created_at DESC LIMIT 1`, requestType).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.getFlowTemplate(ctx, db, id)
}

func (s *ApprovalService) ListFlowTemplates(ctx context.Context, query ListQuery) (ListResult[ApprovalFlowTemplate], error) {
	query, sortExpression, err := NormalizeListQuery(query, "name", "asc", map[string]string{
		"name":         "name COLLATE NOCASE",
		"request_type": "request_type",
		"status":       "status",
		"created_at":   "created_at",
	})
	if err != nil {
		return ListResult[ApprovalFlowTemplate]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[ApprovalFlowTemplate]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(name LIKE ? COLLATE NOCASE OR request_type LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern)
	}
	if value := strings.TrimSpace(query.Filters["status"]); value != "" {
		where = append(where, "status = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Filters["request_type"]); value != "" {
		where = append(where, "request_type = ?")
		args = append(args, value)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_flow_templates WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[ApprovalFlowTemplate]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id FROM approval_flow_templates WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[ApprovalFlowTemplate]{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ListResult[ApprovalFlowTemplate]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[ApprovalFlowTemplate]{}, err
	}
	templates := make([]ApprovalFlowTemplate, 0, len(ids))
	for _, id := range ids {
		template, err := s.getFlowTemplate(ctx, db, id)
		if err != nil {
			return ListResult[ApprovalFlowTemplate]{}, err
		}
		templates = append(templates, *template)
	}
	return NewListResult(templates, query, total), nil
}

// queryRower is satisfied by both *sql.DB and *sql.Tx, so read helpers can
// run either against the pool directly or against an already-open
// transaction. With SetMaxOpenConns(1), calling a *sql.DB-typed helper while
// a transaction is in flight would deadlock waiting for the connection the
// transaction is holding.
type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *ApprovalService) getFlowTemplate(ctx context.Context, db queryRower, id string) (*ApprovalFlowTemplate, error) {
	var template ApprovalFlowTemplate
	err := db.QueryRowContext(ctx, `SELECT id, name, request_type, status, created_at
		FROM approval_flow_templates WHERE id=?`, id).
		Scan(&template.ID, &template.Name, &template.RequestType, &template.Status, &template.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}

	stepRows, err := db.QueryContext(ctx, `SELECT id, flow_template_id, step_order, approver_type, approver_user_id, approver_role_id, approver_group_id, min_amount
		FROM approval_step_templates WHERE flow_template_id=? ORDER BY step_order`, id)
	if err != nil {
		return nil, err
	}
	defer stepRows.Close()
	template.Steps = []ApprovalStepTemplate{}
	for stepRows.Next() {
		var step ApprovalStepTemplate
		if err := stepRows.Scan(&step.ID, &step.FlowTemplateID, &step.StepOrder, &step.ApproverType,
			&step.ApproverUserID, &step.ApproverRoleID, &step.ApproverGroupID, &step.MinAmount); err != nil {
			return nil, err
		}
		template.Steps = append(template.Steps, step)
	}
	if err := stepRows.Err(); err != nil {
		return nil, err
	}

	targetRows, err := db.QueryContext(ctx, `SELECT id, flow_template_id, target_type, target_user_id, target_role_id, target_group_id, notify_on
		FROM approval_notification_targets WHERE flow_template_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer targetRows.Close()
	template.NotificationTargets = []ApprovalNotificationTarget{}
	for targetRows.Next() {
		var target ApprovalNotificationTarget
		if err := targetRows.Scan(&target.ID, &target.FlowTemplateID, &target.TargetType,
			&target.TargetUserID, &target.TargetRoleID, &target.TargetGroupID, &target.NotifyOn); err != nil {
			return nil, err
		}
		template.NotificationTargets = append(template.NotificationTargets, target)
	}
	if err := targetRows.Err(); err != nil {
		return nil, err
	}
	return &template, nil
}

func (s *ApprovalService) normalizeStepTemplates(ctx context.Context, inputs []StepTemplateInput) ([]ApprovalStepTemplate, error) {
	steps := make([]ApprovalStepTemplate, 0, len(inputs))
	for index, input := range inputs {
		approverType := strings.TrimSpace(input.ApproverType)
		step := ApprovalStepTemplate{StepOrder: index + 1, ApproverType: approverType}
		switch approverType {
		case ApprovalApproverSpecificUser:
			userID := strings.TrimSpace(input.ApproverUserID)
			if userID == "" {
				return nil, fmt.Errorf("%w: step %d requires approver_user_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.Get(ctx, userID); err != nil {
				return nil, err
			}
			step.ApproverUserID = &userID
		case ApprovalApproverRole:
			roleID := strings.TrimSpace(input.ApproverRoleID)
			if roleID == "" {
				return nil, fmt.Errorf("%w: step %d requires approver_role_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.GetRole(ctx, roleID); err != nil {
				return nil, err
			}
			step.ApproverRoleID = &roleID
		case ApprovalApproverRequesterManager:
			// No additional field: resolved via group-system.md Section 5's
			// Manager Resolution algorithm at submission/advance time.
		case ApprovalApproverGroupManager:
			groupID := strings.TrimSpace(input.ApproverGroupID)
			if groupID == "" {
				return nil, fmt.Errorf("%w: step %d requires approver_group_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.GetGroup(ctx, groupID); err != nil {
				return nil, err
			}
			step.ApproverGroupID = &groupID
		default:
			return nil, fmt.Errorf("%w: step %d has unknown approver_type %q", ErrApprovalInvalidInput, index+1, approverType)
		}
		if minAmount := strings.TrimSpace(input.MinAmount); minAmount != "" {
			if !approvalAmountPattern.MatchString(minAmount) {
				return nil, fmt.Errorf("%w: step %d min_amount must be a non-negative decimal number", ErrApprovalInvalidInput, index+1)
			}
			step.MinAmount = &minAmount
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func (s *ApprovalService) normalizeNotificationTargets(ctx context.Context, inputs []NotificationTargetInput) ([]ApprovalNotificationTarget, error) {
	targets := make([]ApprovalNotificationTarget, 0, len(inputs))
	for index, input := range inputs {
		targetType := strings.TrimSpace(input.TargetType)
		notifyOn := strings.TrimSpace(input.NotifyOn)
		if notifyOn != ApprovalNotifyOnApproved && notifyOn != ApprovalNotifyOnRejected && notifyOn != ApprovalNotifyOnBoth {
			return nil, fmt.Errorf("%w: notification target %d requires a valid notify_on", ErrApprovalInvalidInput, index+1)
		}
		target := ApprovalNotificationTarget{TargetType: targetType, NotifyOn: notifyOn}
		switch targetType {
		case ApprovalTargetSpecificUser:
			userID := strings.TrimSpace(input.TargetUserID)
			if userID == "" {
				return nil, fmt.Errorf("%w: notification target %d requires target_user_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.Get(ctx, userID); err != nil {
				return nil, err
			}
			target.TargetUserID = &userID
		case ApprovalTargetRole:
			roleID := strings.TrimSpace(input.TargetRoleID)
			if roleID == "" {
				return nil, fmt.Errorf("%w: notification target %d requires target_role_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.GetRole(ctx, roleID); err != nil {
				return nil, err
			}
			target.TargetRoleID = &roleID
		case ApprovalTargetGroupManager:
			groupID := strings.TrimSpace(input.TargetGroupID)
			if groupID == "" {
				return nil, fmt.Errorf("%w: notification target %d requires target_group_id", ErrApprovalInvalidInput, index+1)
			}
			if _, err := s.users.GetGroup(ctx, groupID); err != nil {
				return nil, err
			}
			target.TargetGroupID = &groupID
		default:
			return nil, fmt.Errorf("%w: notification target %d has unknown target_type %q", ErrApprovalInvalidInput, index+1, targetType)
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func insertStepTemplates(ctx context.Context, tx *sql.Tx, flowTemplateID string, steps []ApprovalStepTemplate) error {
	for _, step := range steps {
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_step_templates
			(id, flow_template_id, step_order, approver_type, approver_user_id, approver_role_id, approver_group_id, min_amount)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), flowTemplateID, step.StepOrder, step.ApproverType,
			step.ApproverUserID, step.ApproverRoleID, step.ApproverGroupID, step.MinAmount); err != nil {
			return err
		}
	}
	return nil
}

func insertNotificationTargets(ctx context.Context, tx *sql.Tx, flowTemplateID string, targets []ApprovalNotificationTarget) error {
	for _, target := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_notification_targets
			(id, flow_template_id, target_type, target_user_id, target_role_id, target_group_id, notify_on)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), flowTemplateID, target.TargetType,
			target.TargetUserID, target.TargetRoleID, target.TargetGroupID, target.NotifyOn); err != nil {
			return err
		}
	}
	return nil
}

// ---- Requests ----

// SubmitRequest instantiates a Flow Template into a new Approval Request and
// assigns step 1 (or the first step whose min_amount threshold is met). See
// docs/System/approval-system.md Section 3.
func (s *ApprovalService) SubmitRequest(ctx context.Context, requestedByUserID string, input SubmitApprovalRequestInput) (*ApprovalRequest, error) {
	flowTemplateID := strings.TrimSpace(input.FlowTemplateID)
	sourceModule := strings.TrimSpace(input.SourceModule)
	sourceReferenceID := strings.TrimSpace(input.SourceReferenceID)
	if flowTemplateID == "" || sourceModule == "" || sourceReferenceID == "" {
		return nil, fmt.Errorf("%w: flow_template_id, source_module, and source_reference_id are required", ErrApprovalInvalidInput)
	}
	if _, err := s.users.Get(ctx, requestedByUserID); err != nil {
		return nil, err
	}
	amountValue, amountText, err := parseApprovalAmount(input.Amount)
	if err != nil {
		return nil, err
	}

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	template, err := s.getFlowTemplate(ctx, db, flowTemplateID)
	if err != nil {
		return nil, err
	}
	if template.Status != "active" {
		return nil, fmt.Errorf("%w: flow template is not active", ErrApprovalInvalidInput)
	}
	if len(template.Steps) == 0 {
		return nil, fmt.Errorf("%w: flow template has no steps configured", ErrApprovalInvalidInput)
	}

	steps, status, currentStepOrder, err := s.generateStepsFrom(ctx, requestedByUserID, amountValue, template.Steps, 1)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	id := uuid.NewString()
	now := s.now().UTC().Format(time.RFC3339)
	var completedAt any
	if status == ApprovalRequestApproved {
		completedAt = now
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO approval_requests
		(id, flow_template_id, source_module, source_reference_id, requested_by_user_id, amount, status, current_step_order, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, flowTemplateID, sourceModule, sourceReferenceID, requestedByUserID, amountText, status, currentStepOrder, now, completedAt); err != nil {
		return nil, err
	}
	if err := insertGeneratedSteps(ctx, tx, id, steps); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getRequest(ctx, db, id)
}

// DecideRequest records an approve/reject decision on the request's current
// step, from one of its assigned approvers, and advances the request to the
// next applicable step (or a terminal status). See
// docs/System/approval-system.md Section 3.
func (s *ApprovalService) DecideRequest(ctx context.Context, approverID, requestID string, input DecideApprovalInput) (*ApprovalRequest, error) {
	decision := strings.TrimSpace(input.Decision)
	if decision != ApprovalStepApproved && decision != ApprovalStepRejected {
		return nil, fmt.Errorf("%w: decision must be approved or rejected", ErrApprovalDecision)
	}
	comment := strings.TrimSpace(input.Comment)

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var flowTemplateID, requestedByUserID, requestStatus string
	var amountText sql.NullString
	var currentStepOrder int
	err = tx.QueryRowContext(ctx, `SELECT flow_template_id, requested_by_user_id, status, current_step_order, amount
		FROM approval_requests WHERE id=?`, requestID).
		Scan(&flowTemplateID, &requestedByUserID, &requestStatus, &currentStepOrder, &amountText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	if requestStatus != ApprovalRequestPending {
		return nil, ErrApprovalDecision
	}

	var stepID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM approval_steps
		WHERE approval_request_id=? AND step_order=? AND decision='pending'`, requestID, currentStepOrder).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalDecision
	}
	if err != nil {
		return nil, err
	}
	var assigned int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_step_assignees WHERE approval_step_id=? AND user_id=?`, stepID, approverID).Scan(&assigned); err != nil {
		return nil, err
	}
	if assigned == 0 {
		return nil, ErrApprovalDecision
	}

	now := s.now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(ctx, `UPDATE approval_steps SET decision=?, decided_by_user_id=?, decided_at=?, comment=?
		WHERE id=? AND decision='pending'`, decision, approverID, now, comment, stepID)
	if err != nil {
		return nil, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, ErrApprovalDecision
	}

	if decision == ApprovalStepRejected {
		if _, err := tx.ExecContext(ctx, `UPDATE approval_requests SET status='rejected', completed_at=? WHERE id=?`, now, requestID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.getRequest(ctx, db, requestID)
	}

	template, err := s.getFlowTemplate(ctx, tx, flowTemplateID)
	if err != nil {
		return nil, err
	}
	var amountValue *float64
	if amountText.Valid {
		value, err := strconv.ParseFloat(amountText.String, 64)
		if err != nil {
			return nil, err
		}
		amountValue = &value
	}
	nextSteps, nextStatus, nextCurrentStepOrder, err := s.generateStepsFrom(ctx, requestedByUserID, amountValue, template.Steps, currentStepOrder+1)
	if err != nil {
		return nil, err
	}
	if err := insertGeneratedSteps(ctx, tx, requestID, nextSteps); err != nil {
		return nil, err
	}
	var completedAt any
	if nextStatus == ApprovalRequestApproved {
		completedAt = now
	}
	if _, err := tx.ExecContext(ctx, `UPDATE approval_requests SET status=?, current_step_order=?, completed_at=? WHERE id=?`,
		nextStatus, nextCurrentStepOrder, completedAt, requestID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getRequest(ctx, db, requestID)
}

// CancelRequest lets the requester withdraw their own request before any
// step has been decided. See docs/System/approval-system.md Section 3.
func (s *ApprovalService) CancelRequest(ctx context.Context, requesterID, requestID string) (*ApprovalRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var actualRequester, status string
	err = tx.QueryRowContext(ctx, `SELECT requested_by_user_id, status FROM approval_requests WHERE id=?`, requestID).Scan(&actualRequester, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	if actualRequester != requesterID {
		return nil, ErrApprovalPermission
	}
	if status != ApprovalRequestPending && status != ApprovalRequestRequiresAssignment {
		return nil, fmt.Errorf("%w: only a pending request can be cancelled", ErrApprovalDecision)
	}
	var anyDecided int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_steps
		WHERE approval_request_id=? AND decision NOT IN ('pending','skipped')`, requestID).Scan(&anyDecided); err != nil {
		return nil, err
	}
	if anyDecided > 0 {
		return nil, fmt.Errorf("%w: request already has a decision recorded", ErrApprovalDecision)
	}
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE approval_requests SET status='cancelled', completed_at=? WHERE id=?`, now, requestID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getRequest(ctx, db, requestID)
}

// ReassignApprover manually assigns approver(s) to a request stuck in
// requires_assignment, unblocking it. See
// docs/System/approval-system.md Section 3.
func (s *ApprovalService) ReassignApprover(ctx context.Context, actorUserID, requestID string, input ReassignApprovalInput) (*ApprovalRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var requesterID, status string
	var currentStepOrder int
	err = tx.QueryRowContext(ctx, `SELECT requested_by_user_id, status, current_step_order FROM approval_requests WHERE id=?`, requestID).
		Scan(&requesterID, &status, &currentStepOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != ApprovalRequestRequiresAssignment {
		return nil, fmt.Errorf("%w: request does not require assignment", ErrApprovalDecision)
	}
	var stepID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM approval_steps
		WHERE approval_request_id=? AND step_order=? AND decision='pending'`, requestID, currentStepOrder).Scan(&stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalDecision
	}
	if err != nil {
		return nil, err
	}

	assignedAny := false
	seen := map[string]bool{}
	for _, rawUserID := range input.UserIDs {
		userID := strings.TrimSpace(rawUserID)
		if userID == "" || userID == requesterID || seen[userID] {
			continue
		}
		seen[userID] = true
		if _, err := s.users.Get(ctx, userID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO approval_step_assignees (approval_step_id, user_id) VALUES (?, ?)`, stepID, userID); err != nil {
			return nil, err
		}
		assignedAny = true
	}
	if !assignedAny {
		return nil, fmt.Errorf("%w: no valid approvers provided", ErrApprovalInvalidInput)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE approval_requests SET status='pending' WHERE id=?`, requestID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getRequest(ctx, db, requestID)
}

func (s *ApprovalService) GetRequest(ctx context.Context, id string) (*ApprovalRequest, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getRequest(ctx, db, id)
}

func (s *ApprovalService) ListRequests(ctx context.Context, query ListQuery) (ListResult[ApprovalRequest], error) {
	return s.listRequests(ctx, "1=1", nil, query)
}

func (s *ApprovalService) ListMyRequests(ctx context.Context, requesterID string, query ListQuery) (ListResult[ApprovalRequest], error) {
	return s.listRequests(ctx, "requested_by_user_id=?", []any{requesterID}, query)
}

// ListMyAssignments returns requests whose current step is awaiting a
// decision from approverID.
func (s *ApprovalService) ListMyAssignments(ctx context.Context, approverID string, query ListQuery) (ListResult[ApprovalRequest], error) {
	return s.listRequests(ctx, `EXISTS (
		SELECT 1 FROM approval_steps s JOIN approval_step_assignees a ON a.approval_step_id=s.id
		WHERE s.approval_request_id=approval_requests.id AND s.decision='pending' AND a.user_id=?
	)`, []any{approverID}, query)
}

func (s *ApprovalService) listRequests(ctx context.Context, baseWhere string, baseArgs []any, query ListQuery) (ListResult[ApprovalRequest], error) {
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{
		"created_at":         "created_at",
		"status":             "status",
		"source_module":      "source_module",
		"current_step_order": "current_step_order",
	})
	if err != nil {
		return ListResult[ApprovalRequest]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[ApprovalRequest]{}, err
	}
	defer db.Close()
	where := []string{baseWhere}
	args := append([]any{}, baseArgs...)
	if value := strings.TrimSpace(query.Filters["status"]); value != "" {
		where = append(where, "status = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Filters["source_module"]); value != "" {
		where = append(where, "source_module = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Filters["flow_template_id"]); value != "" {
		where = append(where, "flow_template_id = ?")
		args = append(args, value)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_requests WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[ApprovalRequest]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id FROM approval_requests WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[ApprovalRequest]{}, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ListResult[ApprovalRequest]{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return ListResult[ApprovalRequest]{}, err
	}
	requests := make([]ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		request, err := s.getRequest(ctx, db, id)
		if err != nil {
			return ListResult[ApprovalRequest]{}, err
		}
		requests = append(requests, *request)
	}
	return NewListResult(requests, query, total), nil
}

func (s *ApprovalService) getRequest(ctx context.Context, db queryRower, id string) (*ApprovalRequest, error) {
	var request ApprovalRequest
	var amount, completedAt sql.NullString
	err := db.QueryRowContext(ctx, `SELECT id, flow_template_id, source_module, source_reference_id, requested_by_user_id, amount, status, current_step_order, created_at, completed_at
		FROM approval_requests WHERE id=?`, id).Scan(&request.ID, &request.FlowTemplateID, &request.SourceModule, &request.SourceReferenceID,
		&request.RequestedByUserID, &amount, &request.Status, &request.CurrentStepOrder, &request.CreatedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	if amount.Valid {
		request.Amount = &amount.String
	}
	if completedAt.Valid {
		request.CompletedAt = &completedAt.String
	}
	steps, err := getRequestSteps(ctx, db, id)
	if err != nil {
		return nil, err
	}
	request.Steps = steps
	return &request, nil
}

func getRequestSteps(ctx context.Context, db queryRower, requestID string) ([]ApprovalStep, error) {
	rows, err := db.QueryContext(ctx, `SELECT s.id, s.approval_request_id, s.step_order, s.decision, s.decided_by_user_id, s.decided_at, s.comment,
		GROUP_CONCAT(a.user_id)
		FROM approval_steps s LEFT JOIN approval_step_assignees a ON a.approval_step_id = s.id
		WHERE s.approval_request_id=? GROUP BY s.id ORDER BY s.step_order`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	steps := []ApprovalStep{}
	for rows.Next() {
		var step ApprovalStep
		var decidedBy, decidedAt, assignees sql.NullString
		if err := rows.Scan(&step.ID, &step.ApprovalRequestID, &step.StepOrder, &step.Decision,
			&decidedBy, &decidedAt, &step.Comment, &assignees); err != nil {
			return nil, err
		}
		if decidedBy.Valid {
			step.DecidedByUserID = &decidedBy.String
		}
		if decidedAt.Valid {
			step.DecidedAt = &decidedAt.String
		}
		if assignees.Valid && assignees.String != "" {
			step.AssignedUserIDs = strings.Split(assignees.String, ",")
		} else {
			step.AssignedUserIDs = []string{}
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

type generatedStep struct {
	stepOrder       int
	decision        string
	assignedUserIDs []string
}

// generateStepsFrom walks the step templates starting at fromOrder, skipping
// steps whose min_amount threshold the request's amount doesn't meet, and
// resolving approvers for the first step that applies. It stops at the first
// step that becomes current (pending, whether or not approvers were found),
// or reports the request as approved if every remaining step is skipped.
func (s *ApprovalService) generateStepsFrom(ctx context.Context, requesterID string, amount *float64, templates []ApprovalStepTemplate, fromOrder int) ([]generatedStep, string, int, error) {
	last := fromOrder - 1
	var steps []generatedStep
	for _, t := range templates {
		if t.StepOrder < fromOrder {
			continue
		}
		last = t.StepOrder
		if t.MinAmount != nil {
			threshold, err := strconv.ParseFloat(*t.MinAmount, 64)
			if err != nil {
				return nil, "", 0, err
			}
			if amount == nil || *amount < threshold {
				steps = append(steps, generatedStep{stepOrder: t.StepOrder, decision: ApprovalStepSkipped})
				continue
			}
		}
		approverIDs, err := s.resolveStepApprovers(ctx, t, requesterID)
		if err != nil {
			return nil, "", 0, err
		}
		status := ApprovalRequestPending
		if len(approverIDs) == 0 {
			status = ApprovalRequestRequiresAssignment
		}
		steps = append(steps, generatedStep{stepOrder: t.StepOrder, decision: ApprovalStepPending, assignedUserIDs: approverIDs})
		return steps, status, t.StepOrder, nil
	}
	return steps, ApprovalRequestApproved, last, nil
}

func (s *ApprovalService) resolveStepApprovers(ctx context.Context, t ApprovalStepTemplate, requesterID string) ([]string, error) {
	switch t.ApproverType {
	case ApprovalApproverSpecificUser:
		if t.ApproverUserID == nil || *t.ApproverUserID == requesterID {
			return nil, nil
		}
		return []string{*t.ApproverUserID}, nil
	case ApprovalApproverRole:
		return s.users.RoleMemberUserIDs(ctx, *t.ApproverRoleID, requesterID)
	case ApprovalApproverRequesterManager:
		route, err := s.users.ResolveLeaveApproval(ctx, requesterID)
		if err != nil {
			return nil, err
		}
		return route.ApproverUserIDs, nil
	case ApprovalApproverGroupManager:
		return s.users.GroupManagerUserIDs(ctx, *t.ApproverGroupID, requesterID)
	default:
		return nil, fmt.Errorf("%w: unknown approver_type %q", ErrApprovalInvalidInput, t.ApproverType)
	}
}

func insertGeneratedSteps(ctx context.Context, tx *sql.Tx, requestID string, steps []generatedStep) error {
	for _, st := range steps {
		stepID := uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO approval_steps (id, approval_request_id, step_order, decision, comment)
			VALUES (?, ?, ?, ?, '')`, stepID, requestID, st.stepOrder, st.decision); err != nil {
			return err
		}
		for _, userID := range st.assignedUserIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO approval_step_assignees (approval_step_id, user_id) VALUES (?, ?)`, stepID, userID); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseApprovalAmount(raw *string) (*float64, *string, error) {
	if raw == nil {
		return nil, nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil, nil
	}
	if !approvalAmountPattern.MatchString(trimmed) {
		return nil, nil, fmt.Errorf("%w: amount must be a non-negative decimal number", ErrApprovalInvalidInput)
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: amount is not a valid number", ErrApprovalInvalidInput)
	}
	return &value, &trimmed, nil
}
