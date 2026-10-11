package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Employment statuses. See docs/System/hr-system.md Section 3.2.
const (
	EmploymentStatusProbation  = "probation"
	EmploymentStatusActive     = "active"
	EmploymentStatusOnLeave    = "on_leave"
	EmploymentStatusSuspended  = "suspended"
	EmploymentStatusTerminated = "terminated"
)

// Employment event types. See Section 3.3.
const (
	EmploymentEventOnboarded         = "onboarded"
	EmploymentEventProbationPassed   = "probation_passed"
	EmploymentEventProbationExtended = "probation_extended"
	EmploymentEventTransferred       = "transferred"
	EmploymentEventPromoted          = "promoted"
	EmploymentEventSuspended         = "suspended"
	EmploymentEventReinstated        = "reinstated"
	EmploymentEventTerminated        = "terminated"
	EmploymentEventRehired           = "rehired"
)

// Checklist task values. See Section 3.4.
const (
	HRTaskKindOnboarding  = "onboarding"
	HRTaskKindOffboarding = "offboarding"

	HRTaskPending    = "pending"
	HRTaskInProgress = "in_progress"
	HRTaskDone       = "done"
	HRTaskSkipped    = "skipped"
)

var validHRTaskStatuses = map[string]bool{HRTaskPending: true, HRTaskInProgress: true, HRTaskDone: true, HRTaskSkipped: true}

type EmploymentEvent struct {
	ID                       string  `json:"id"`
	UserID                   string  `json:"user_id"`
	EventType                string  `json:"event_type"`
	EffectiveDate            string  `json:"effective_date"`
	PreviousEmploymentStatus *string `json:"previous_employment_status"`
	NewEmploymentStatus      *string `json:"new_employment_status"`
	RelatedGroupID           *string `json:"related_group_id"`
	RelatedCaseID            *string `json:"related_case_id"`
	ProbationEndDate         *string `json:"probation_end_date"`
	Reason                   *string `json:"reason"`
	ActorUserID              string  `json:"actor_user_id"`
	CreatedAt                string  `json:"created_at"`
}

// EmploymentRecord is the HR view of one employee: the current status from
// employee_profiles plus the HR-owned event history.
type EmploymentRecord struct {
	UserID           string            `json:"user_id"`
	DisplayName      string            `json:"display_name"`
	EmployeeNumber   *string           `json:"employee_number"`
	JobTitle         *string           `json:"job_title"`
	EmploymentStatus *string           `json:"employment_status"`
	HireDate         *string           `json:"hire_date"`
	TerminationDate  *string           `json:"termination_date"`
	ProbationEndDate *string           `json:"probation_end_date"`
	Events           []EmploymentEvent `json:"events"`
}

type RecordEmploymentEventInput struct {
	EventType        string  `json:"event_type"`
	EffectiveDate    string  `json:"effective_date"`
	Reason           *string `json:"reason"`
	RelatedGroupID   *string `json:"related_group_id"`
	RelatedCaseID    *string `json:"related_case_id"`
	ProbationEndDate *string `json:"probation_end_date"`
	// SkipProbation starts an onboarded/rehired employee directly in active
	// status even when the organization requires probation.
	SkipProbation  bool    `json:"skip_probation"`
	EmployeeNumber string  `json:"employee_number"`
	JobTitle       *string `json:"job_title"`
	// Tasks seeds the onboarding (onboarded/rehired) or offboarding
	// (terminated) checklist together with the event.
	Tasks []CreateHRTaskInput `json:"tasks"`
}

type HRTask struct {
	ID               string  `json:"id"`
	UserID           string  `json:"user_id"`
	Kind             string  `json:"kind"`
	Title            string  `json:"title"`
	Status           string  `json:"status"`
	AssignedToUserID *string `json:"assigned_to_user_id"`
	DueDate          *string `json:"due_date"`
	CompletedAt      *string `json:"completed_at"`
	CreatedByUserID  string  `json:"created_by_user_id"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type CreateHRTaskInput struct {
	Kind             string  `json:"kind"`
	Title            string  `json:"title"`
	AssignedToUserID *string `json:"assigned_to_user_id"`
	DueDate          *string `json:"due_date"`
}

type UpdateHRTaskInput struct {
	Title            *string `json:"title"`
	Status           *string `json:"status"`
	AssignedToUserID *string `json:"assigned_to_user_id"`
	DueDate          *string `json:"due_date"`
}

const employmentEventColumns = `id, user_id, event_type, effective_date, previous_employment_status, new_employment_status,
	related_group_id, related_case_id, probation_end_date, reason, actor_user_id, created_at`

func scanEmploymentEvent(scanner interface{ Scan(...any) error }) (EmploymentEvent, error) {
	var e EmploymentEvent
	err := scanner.Scan(&e.ID, &e.UserID, &e.EventType, &e.EffectiveDate, &e.PreviousEmploymentStatus, &e.NewEmploymentStatus,
		&e.RelatedGroupID, &e.RelatedCaseID, &e.ProbationEndDate, &e.Reason, &e.ActorUserID, &e.CreatedAt)
	return e, err
}

// EmploymentRecord returns the employee's current status and full history.
func (s *HRService) EmploymentRecord(ctx context.Context, userID string) (*EmploymentRecord, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	record := &EmploymentRecord{UserID: user.ID, DisplayName: user.DisplayName, Events: []EmploymentEvent{}}
	if profile := user.EmployeeProfile; profile != nil {
		record.EmployeeNumber = &profile.EmployeeNumber
		record.JobTitle = profile.JobTitle
		record.EmploymentStatus = &profile.EmploymentStatus
		record.HireDate = profile.HireDate
		record.TerminationDate = profile.TerminationDate
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT `+employmentEventColumns+` FROM employment_events
		WHERE user_id = ? ORDER BY effective_date DESC, created_at DESC, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		event, err := scanEmploymentEvent(rows)
		if err != nil {
			return nil, err
		}
		record.Events = append(record.Events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if record.EmploymentStatus != nil && *record.EmploymentStatus == EmploymentStatusProbation {
		for _, event := range record.Events {
			if event.ProbationEndDate != nil {
				record.ProbationEndDate = event.ProbationEndDate
				break
			}
		}
	}
	return record, nil
}

// RecordEmploymentEvent applies one Section 3.2 state-machine transition:
// it validates the transition against the current employee_profiles status,
// applies the profile/account change through UserService, and appends the
// HR narrative event.
func (s *HRService) RecordEmploymentEvent(ctx context.Context, actor HRActor, userID string, input RecordEmploymentEventInput) (*EmploymentEvent, error) {
	user, err := s.users.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	effectiveDate, err := parseHRDate("effective_date", input.EffectiveDate)
	if err != nil {
		return nil, err
	}
	probationEnd, err := parseOptionalHRDate("probation_end_date", input.ProbationEndDate)
	if err != nil {
		return nil, err
	}
	reason := trimmedOrNil(input.Reason)
	relatedGroupID := trimmedOrNil(input.RelatedGroupID)
	relatedCaseID := trimmedOrNil(input.RelatedCaseID)
	current := ""
	if user.EmployeeProfile != nil {
		current = user.EmployeeProfile.EmploymentStatus
	}

	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	settings, err := readHRSettings(ctx, db)
	if err != nil {
		return nil, err
	}
	var priorEvents int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM employment_events WHERE user_id = ?`, userID).Scan(&priorEvents); err != nil {
		return nil, err
	}

	change := EmploymentProfileChange{}
	next := current
	taskKind := ""
	switch input.EventType {
	case EmploymentEventOnboarded, EmploymentEventRehired:
		if input.EventType == EmploymentEventOnboarded && (priorEvents > 0 || current == EmploymentStatusTerminated) {
			return nil, fmt.Errorf("%w: employee is already onboarded; use rehired for a returning employee", ErrHRInvalidTransition)
		}
		if input.EventType == EmploymentEventRehired && current != EmploymentStatusTerminated {
			return nil, fmt.Errorf("%w: only a terminated employee can be rehired", ErrHRInvalidTransition)
		}
		next = EmploymentStatusActive
		if settings.ProbationRequired && !input.SkipProbation {
			next = EmploymentStatusProbation
			if probationEnd == nil {
				start, _ := time.Parse(hrDateLayout, effectiveDate)
				end := start.AddDate(0, 0, settings.ProbationDefaultDays).Format(hrDateLayout)
				probationEnd = &end
			}
		} else {
			probationEnd = nil
		}
		change.HireDate = &effectiveDate
		change.EmployeeNumber = input.EmployeeNumber
		change.JobTitle = input.JobTitle
		if input.EventType == EmploymentEventRehired {
			change.ClearTermination, change.Reenable = true, true
		}
		taskKind = HRTaskKindOnboarding
	case EmploymentEventProbationPassed:
		if current != EmploymentStatusProbation {
			return nil, fmt.Errorf("%w: employee is not on probation", ErrHRInvalidTransition)
		}
		next, probationEnd = EmploymentStatusActive, nil
	case EmploymentEventProbationExtended:
		if current != EmploymentStatusProbation {
			return nil, fmt.Errorf("%w: employee is not on probation", ErrHRInvalidTransition)
		}
		if probationEnd == nil || *probationEnd <= effectiveDate {
			return nil, fmt.Errorf("%w: probation_end_date after effective_date is required", ErrHRInvalidInput)
		}
	case EmploymentEventTransferred, EmploymentEventPromoted:
		if current == "" || current == EmploymentStatusTerminated {
			return nil, fmt.Errorf("%w: employee is not currently employed", ErrHRInvalidTransition)
		}
		if relatedGroupID != nil {
			if _, err := s.users.GetGroup(ctx, *relatedGroupID); err != nil {
				return nil, err
			}
		}
		change.JobTitle = input.JobTitle
		probationEnd = nil
	case EmploymentEventSuspended:
		if current != EmploymentStatusActive && current != EmploymentStatusProbation && current != EmploymentStatusOnLeave {
			return nil, fmt.Errorf("%w: cannot suspend an employee in status %q", ErrHRInvalidTransition, current)
		}
		next, probationEnd = EmploymentStatusSuspended, nil
	case EmploymentEventReinstated:
		if current != EmploymentStatusSuspended {
			return nil, fmt.Errorf("%w: employee is not suspended", ErrHRInvalidTransition)
		}
		next, probationEnd = EmploymentStatusActive, nil
	case EmploymentEventTerminated:
		if current == "" || current == EmploymentStatusTerminated {
			return nil, fmt.Errorf("%w: employee is not currently employed", ErrHRInvalidTransition)
		}
		if reason == nil {
			return nil, fmt.Errorf("%w: reason is required for termination", ErrHRInvalidInput)
		}
		next, probationEnd = EmploymentStatusTerminated, nil
		change.TerminationDate, change.Offboard = &effectiveDate, true
		taskKind = HRTaskKindOffboarding
	default:
		return nil, fmt.Errorf("%w: unknown event_type %q", ErrHRInvalidInput, input.EventType)
	}
	if len(input.Tasks) > 0 && taskKind == "" {
		return nil, fmt.Errorf("%w: tasks can only be seeded by onboarded, rehired, or terminated events", ErrHRInvalidInput)
	}
	tasks := make([]CreateHRTaskInput, 0, len(input.Tasks))
	for _, task := range input.Tasks {
		task.Kind = taskKind
		normalized, err := s.normalizeTaskInput(ctx, task)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, normalized)
	}
	if relatedCaseID != nil {
		if _, err := getEmployeeRelationsCase(ctx, db, *relatedCaseID); err != nil {
			return nil, err
		}
	}
	change.EmploymentStatus = next

	if _, err := s.users.ApplyEmploymentChange(ctx, userID, change); err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var previous, newStatus *string
	if current != "" {
		previous = &current
	}
	if next != current || input.EventType == EmploymentEventProbationExtended {
		newStatus = &next
	}
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO employment_events (`+employmentEventColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, input.EventType, effectiveDate, previous, newStatus, relatedGroupID, relatedCaseID,
		probationEnd, reason, actor.UserID, now); err != nil {
		return nil, err
	}
	if relatedCaseID != nil && (input.EventType == EmploymentEventSuspended || input.EventType == EmploymentEventTerminated) {
		// Section 5.3: link the case back to the employment change it caused.
		if _, err := tx.ExecContext(ctx, `UPDATE employee_relations_cases SET related_employment_event_id = ?, updated_at = ?
			WHERE id = ? AND related_employment_event_id IS NULL`, id, now, *relatedCaseID); err != nil {
			return nil, err
		}
	}
	if err := s.audit(ctx, tx, "employment_event", id, "recorded", actor.UserID, map[string]any{
		"user_id": userID, "event_type": input.EventType, "previous_status": current, "new_status": next,
	}); err != nil {
		return nil, err
	}
	createdTasks := make([]HRTask, 0, len(tasks))
	for _, task := range tasks {
		created, err := s.insertTask(ctx, tx, actor.UserID, userID, task)
		if err != nil {
			return nil, err
		}
		createdTasks = append(createdTasks, *created)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for _, task := range createdTasks {
		s.notifyTaskAssignee(ctx, task)
	}
	event, err := scanEmploymentEvent(db.QueryRowContext(ctx, `SELECT `+employmentEventColumns+` FROM employment_events WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *HRService) normalizeTaskInput(ctx context.Context, input CreateHRTaskInput) (CreateHRTaskInput, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	if input.Kind != HRTaskKindOnboarding && input.Kind != HRTaskKindOffboarding {
		return input, fmt.Errorf("%w: kind must be onboarding or offboarding", ErrHRInvalidInput)
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return input, fmt.Errorf("%w: task title is required", ErrHRInvalidInput)
	}
	due, err := parseOptionalHRDate("due_date", input.DueDate)
	if err != nil {
		return input, err
	}
	input.DueDate = due
	input.AssignedToUserID = trimmedOrNil(input.AssignedToUserID)
	if input.AssignedToUserID != nil {
		if _, err := s.users.Get(ctx, *input.AssignedToUserID); err != nil {
			return input, err
		}
	}
	return input, nil
}

const hrTaskColumns = `id, user_id, kind, title, status, assigned_to_user_id, due_date, completed_at,
	created_by_user_id, created_at, updated_at`

func scanHRTask(scanner interface{ Scan(...any) error }) (HRTask, error) {
	var t HRTask
	err := scanner.Scan(&t.ID, &t.UserID, &t.Kind, &t.Title, &t.Status, &t.AssignedToUserID, &t.DueDate,
		&t.CompletedAt, &t.CreatedByUserID, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *HRService) insertTask(ctx context.Context, tx *sql.Tx, actorUserID, userID string, input CreateHRTaskInput) (*HRTask, error) {
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO hr_tasks (`+hrTaskColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		id, userID, input.Kind, input.Title, HRTaskPending, input.AssignedToUserID, input.DueDate, actorUserID, now, now); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "hr_task", id, "created", actorUserID, input); err != nil {
		return nil, err
	}
	task, err := scanHRTask(tx.QueryRowContext(ctx, `SELECT `+hrTaskColumns+` FROM hr_tasks WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *HRService) notifyTaskAssignee(ctx context.Context, task HRTask) {
	if task.AssignedToUserID == nil {
		return
	}
	s.notifyUser(ctx, *task.AssignedToUserID, "HR task assigned",
		fmt.Sprintf("HR %s task %s has been assigned to you.", task.Kind, task.ID))
}

// CreateTask adds one onboarding/offboarding checklist task (Section 3.4).
func (s *HRService) CreateTask(ctx context.Context, actor HRActor, userID string, input CreateHRTaskInput) (*HRTask, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	input, err := s.normalizeTaskInput(ctx, input)
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
	task, err := s.insertTask(ctx, tx, actor.UserID, userID, input)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyTaskAssignee(ctx, *task)
	return task, nil
}

// ListTasks lists checklist tasks for one employee (employeeUserID) or
// assigned to one person (assigneeUserID).
func (s *HRService) ListTasks(ctx context.Context, employeeUserID, assigneeUserID string, query ListQuery) (ListResult[HRTask], error) {
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "asc", map[string]string{
		"created_at": "created_at",
		"due_date":   "due_date",
		"status":     "status",
	})
	if err != nil {
		return ListResult[HRTask]{}, err
	}
	where, args := []string{}, []any{}
	if employeeUserID != "" {
		if _, err := s.users.Get(ctx, employeeUserID); err != nil {
			return ListResult[HRTask]{}, err
		}
		where = append(where, "user_id = ?")
		args = append(args, employeeUserID)
	}
	if assigneeUserID != "" {
		where = append(where, "assigned_to_user_id = ?")
		args = append(args, assigneeUserID)
	}
	for _, filter := range []string{"kind", "status"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			where = append(where, filter+" = ?")
			args = append(args, value)
		}
	}
	whereSQL := "1=1"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}
	db, err := s.open()
	if err != nil {
		return ListResult[HRTask]{}, err
	}
	defer db.Close()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hr_tasks WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[HRTask]{}, err
	}
	listArgs := append(append([]any{}, args...), query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT `+hrTaskColumns+` FROM hr_tasks WHERE `+whereSQL+`
		ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[HRTask]{}, err
	}
	defer rows.Close()
	items := make([]HRTask, 0, query.PageSize)
	for rows.Next() {
		task, err := scanHRTask(rows)
		if err != nil {
			return ListResult[HRTask]{}, err
		}
		items = append(items, task)
	}
	if err := rows.Err(); err != nil {
		return ListResult[HRTask]{}, err
	}
	return NewListResult(items, query, total), nil
}

// UpdateTask changes a checklist task. With asAssignee, the caller may only
// change the status of a task assigned to them.
func (s *HRService) UpdateTask(ctx context.Context, actor HRActor, id string, input UpdateHRTaskInput, asAssignee bool) (*HRTask, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	task, err := scanHRTask(db.QueryRowContext(ctx, `SELECT `+hrTaskColumns+` FROM hr_tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHRNotFound
	}
	if err != nil {
		return nil, err
	}
	if asAssignee {
		if task.AssignedToUserID == nil || *task.AssignedToUserID != actor.UserID {
			return nil, ErrHRNotFound
		}
		if input.Title != nil || input.AssignedToUserID != nil || input.DueDate != nil {
			return nil, fmt.Errorf("%w: assignees may only change status", ErrHRInvalidInput)
		}
	}
	now := s.timestamp()
	sets, args := []string{"updated_at = ?"}, []any{now}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return nil, fmt.Errorf("%w: task title cannot be empty", ErrHRInvalidInput)
		}
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if input.Status != nil {
		if !validHRTaskStatuses[*input.Status] {
			return nil, fmt.Errorf("%w: unknown task status %q", ErrHRInvalidInput, *input.Status)
		}
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
		if *input.Status == HRTaskDone {
			sets = append(sets, "completed_at = COALESCE(completed_at, ?)")
			args = append(args, now)
		} else {
			sets = append(sets, "completed_at = NULL")
		}
	}
	if input.AssignedToUserID != nil {
		assignee := trimmedOrNil(input.AssignedToUserID)
		if assignee != nil {
			if _, err := s.users.Get(ctx, *assignee); err != nil {
				return nil, err
			}
		}
		sets = append(sets, "assigned_to_user_id = ?")
		args = append(args, assignee)
	}
	if input.DueDate != nil {
		due, err := parseOptionalHRDate("due_date", input.DueDate)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "due_date = ?")
		args = append(args, due)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, `UPDATE hr_tasks SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "hr_task", id, "updated", actor.UserID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	updated, err := scanHRTask(db.QueryRowContext(ctx, `SELECT `+hrTaskColumns+` FROM hr_tasks WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if input.AssignedToUserID != nil && updated.AssignedToUserID != nil &&
		(task.AssignedToUserID == nil || *task.AssignedToUserID != *updated.AssignedToUserID) {
		s.notifyTaskAssignee(ctx, updated)
	}
	return &updated, nil
}
