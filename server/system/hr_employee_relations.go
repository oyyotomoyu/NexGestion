package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Employee Relations values. See docs/System/hr-system.md Section 5.
const (
	CaseTypeGrievance    = "grievance"
	CaseTypeDisciplinary = "disciplinary"
	CaseTypeMediation    = "mediation"

	CaseStatusOpen          = "open"
	CaseStatusInvestigating = "investigating"
	CaseStatusResolved      = "resolved"
	CaseStatusClosed        = "closed"
	CaseStatusEscalated     = "escalated"

	CaseConfidentialityStandard   = "standard"
	CaseConfidentialityRestricted = "restricted"

	CaseActionInvestigate = "investigate"
	CaseActionResolve     = "resolve"
	CaseActionClose       = "close"
	CaseActionEscalate    = "escalate"
)

var validCaseTypes = map[string]bool{CaseTypeGrievance: true, CaseTypeDisciplinary: true, CaseTypeMediation: true}

type EmployeeRelationsCase struct {
	ID                       string                  `json:"id"`
	CaseType                 string                  `json:"case_type"`
	SubjectUserID            string                  `json:"subject_user_id"`
	ReportedByUserID         *string                 `json:"reported_by_user_id"`
	IsAnonymous              bool                    `json:"is_anonymous"`
	AssignedToUserID         string                  `json:"assigned_to_user_id"`
	Status                   string                  `json:"status"`
	ConfidentialityLevel     string                  `json:"confidentiality_level"`
	Summary                  *string                 `json:"summary"`
	ResolutionNotes          *string                 `json:"resolution_notes"`
	RelatedEmploymentEventID *string                 `json:"related_employment_event_id"`
	CreatedByUserID          string                  `json:"created_by_user_id"`
	CreatedAt                string                  `json:"created_at"`
	UpdatedAt                string                  `json:"updated_at"`
	History                  []EmployeeRelationsNote `json:"history,omitempty"`
}

// EmployeeRelationsNote is one entry of a case's change history, read from
// the HR audit log.
type EmployeeRelationsNote struct {
	Action      string `json:"action"`
	ActorUserID string `json:"actor_user_id"`
	Detail      string `json:"detail"`
	CreatedAt   string `json:"created_at"`
}

// EmployeeRelationsCaseSummary is the read.self view: status only, without
// summary, notes, handler, or reporter (Section 6).
type EmployeeRelationsCaseSummary struct {
	ID        string `json:"id"`
	CaseType  string `json:"case_type"`
	Status    string `json:"status"`
	Relation  string `json:"relation"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateEmployeeRelationsCaseInput struct {
	CaseType             string  `json:"case_type"`
	SubjectUserID        string  `json:"subject_user_id"`
	ReportedByUserID     *string `json:"reported_by_user_id"`
	IsAnonymous          bool    `json:"is_anonymous"`
	AssignedToUserID     *string `json:"assigned_to_user_id"`
	ConfidentialityLevel string  `json:"confidentiality_level"`
	Summary              *string `json:"summary"`
}

type FileGrievanceInput struct {
	// SubjectUserID defaults to the reporter for grievances about a policy
	// or working condition rather than a person.
	SubjectUserID        *string `json:"subject_user_id"`
	IsAnonymous          bool    `json:"is_anonymous"`
	ConfidentialityLevel string  `json:"confidentiality_level"`
	Summary              string  `json:"summary"`
}

type UpdateEmployeeRelationsCaseInput struct {
	Summary              *string `json:"summary"`
	ConfidentialityLevel *string `json:"confidentiality_level"`
}

type TransitionEmployeeRelationsCaseInput struct {
	Action           string  `json:"action"`
	AssignedToUserID *string `json:"assigned_to_user_id"`
	ResolutionNotes  *string `json:"resolution_notes"`
	Note             *string `json:"note"`
}

const employeeRelationsCaseColumns = `id, case_type, subject_user_id, reported_by_user_id, is_anonymous, assigned_to_user_id,
	status, confidentiality_level, summary, resolution_notes, related_employment_event_id, created_by_user_id, created_at, updated_at`

func scanEmployeeRelationsCase(scanner interface{ Scan(...any) error }) (EmployeeRelationsCase, error) {
	var c EmployeeRelationsCase
	var anonymous int
	err := scanner.Scan(&c.ID, &c.CaseType, &c.SubjectUserID, &c.ReportedByUserID, &anonymous, &c.AssignedToUserID,
		&c.Status, &c.ConfidentialityLevel, &c.Summary, &c.ResolutionNotes, &c.RelatedEmploymentEventID,
		&c.CreatedByUserID, &c.CreatedAt, &c.UpdatedAt)
	c.IsAnonymous = anonymous == 1
	return c, err
}

func getEmployeeRelationsCase(ctx context.Context, db *sql.DB, id string) (*EmployeeRelationsCase, error) {
	item, err := scanEmployeeRelationsCase(db.QueryRowContext(ctx, `SELECT `+employeeRelationsCaseColumns+` FROM employee_relations_cases WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHRNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func normalizeConfidentiality(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return CaseConfidentialityStandard, nil
	}
	if value != CaseConfidentialityStandard && value != CaseConfidentialityRestricted {
		return "", fmt.Errorf("%w: confidentiality_level must be standard or restricted", ErrHRInvalidInput)
	}
	return value, nil
}

// validateHandler checks that a user may handle a case: they hold
// hr.employee_relations.manage and are not the case subject.
func (s *HRService) validateHandler(ctx context.Context, handlerID, subjectID string, requireRead bool) error {
	if handlerID == subjectID {
		return fmt.Errorf("%w: the case subject cannot handle their own case", ErrHRInvalidInput)
	}
	user, err := s.users.Get(ctx, handlerID)
	if err != nil {
		return err
	}
	if user.Status != "active" {
		return fmt.Errorf("%w: handler must be an active user", ErrHRInvalidInput)
	}
	keys, err := s.users.EffectivePermissionKeys(ctx, handlerID)
	if err != nil {
		return err
	}
	actor := NewHRActor(handlerID, keys)
	if !actor.Has(PermissionHREmployeeRelationsManage) || (requireRead && !actor.Has(PermissionHREmployeeRelationsRead)) {
		return fmt.Errorf("%w: handler lacks the required employee relations permission", ErrHRInvalidInput)
	}
	return nil
}

func (s *HRService) insertCase(ctx context.Context, actorUserID string, item EmployeeRelationsCase) (*EmployeeRelationsCase, error) {
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
	now := s.timestamp()
	item.ID, item.Status, item.CreatedByUserID, item.CreatedAt, item.UpdatedAt = uuid.NewString(), CaseStatusOpen, actorUserID, now, now
	if _, err := tx.ExecContext(ctx, `INSERT INTO employee_relations_cases (`+employeeRelationsCaseColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, ?)`,
		item.ID, item.CaseType, item.SubjectUserID, item.ReportedByUserID, boolInt(item.IsAnonymous), item.AssignedToUserID,
		item.Status, item.ConfidentialityLevel, item.Summary, item.CreatedByUserID, now, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO employee_relations_case_handlers (case_id, user_id, assigned_at) VALUES (?, ?, ?)`,
		item.ID, item.AssignedToUserID, now); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "employee_relations_case", item.ID, "created", actorUserID, map[string]string{
		"case_type": item.CaseType, "assigned_to_user_id": item.AssignedToUserID, "confidentiality_level": item.ConfidentialityLevel,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyCaseHandler(ctx, item.ID, item.AssignedToUserID)
	return getEmployeeRelationsCase(ctx, db, item.ID)
}

func (s *HRService) notifyCaseHandler(ctx context.Context, caseID, handlerID string) {
	s.notifyUser(ctx, handlerID, "Employee relations case assigned",
		fmt.Sprintf("Employee relations case %s has been assigned to you.", caseID))
}

// CreateEmployeeRelationsCase opens a case on behalf of HR or a manager
// holding hr.employee_relations.manage. The handler defaults to the creator.
func (s *HRService) CreateEmployeeRelationsCase(ctx context.Context, actor HRActor, input CreateEmployeeRelationsCaseInput) (*EmployeeRelationsCase, error) {
	if !validCaseTypes[input.CaseType] {
		return nil, fmt.Errorf("%w: unknown case_type %q", ErrHRInvalidInput, input.CaseType)
	}
	subjectID := strings.TrimSpace(input.SubjectUserID)
	if _, err := s.users.Get(ctx, subjectID); err != nil {
		return nil, err
	}
	reporter := trimmedOrNil(input.ReportedByUserID)
	if input.IsAnonymous {
		if input.CaseType != CaseTypeGrievance {
			return nil, fmt.Errorf("%w: only grievance cases may be anonymous", ErrHRInvalidInput)
		}
		reporter = nil
	} else if reporter != nil {
		if _, err := s.users.Get(ctx, *reporter); err != nil {
			return nil, err
		}
	}
	handler := actor.UserID
	if assigned := trimmedOrNil(input.AssignedToUserID); assigned != nil {
		handler = *assigned
	}
	if err := s.validateHandler(ctx, handler, subjectID, false); err != nil {
		return nil, err
	}
	confidentiality, err := normalizeConfidentiality(input.ConfidentialityLevel)
	if err != nil {
		return nil, err
	}
	return s.insertCase(ctx, actor.UserID, EmployeeRelationsCase{
		CaseType: input.CaseType, SubjectUserID: subjectID, ReportedByUserID: reporter, IsAnonymous: input.IsAnonymous,
		AssignedToUserID: handler, ConfidentialityLevel: confidentiality, Summary: trimmedOrNil(input.Summary),
	})
}

// FileGrievance lets an employee raise a grievance themselves. The case is
// routed to an available HR handler, never to anyone involved in it.
func (s *HRService) FileGrievance(ctx context.Context, actor HRActor, input FileGrievanceInput) (*EmployeeRelationsCaseSummary, error) {
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		return nil, fmt.Errorf("%w: summary is required", ErrHRInvalidInput)
	}
	subjectID := actor.UserID
	if subject := trimmedOrNil(input.SubjectUserID); subject != nil {
		subjectID = *subject
	}
	if _, err := s.users.Get(ctx, subjectID); err != nil {
		return nil, err
	}
	confidentiality, err := normalizeConfidentiality(input.ConfidentialityLevel)
	if err != nil {
		return nil, err
	}
	candidates, err := s.users.UsersWithPermission(ctx, PermissionHREmployeeRelationsManage)
	if err != nil {
		return nil, err
	}
	handler := ""
	for _, candidate := range candidates {
		if candidate.ID != subjectID && candidate.ID != actor.UserID && candidate.Status == "active" {
			handler = candidate.ID
			break
		}
	}
	if handler == "" {
		return nil, fmt.Errorf("%w: no employee relations handler is available", ErrHRInvalidInput)
	}
	var reporter *string
	if !input.IsAnonymous {
		reporter = &actor.UserID
	}
	item, err := s.insertCase(ctx, actor.UserID, EmployeeRelationsCase{
		CaseType: CaseTypeGrievance, SubjectUserID: subjectID, ReportedByUserID: reporter, IsAnonymous: input.IsAnonymous,
		AssignedToUserID: handler, ConfidentialityLevel: confidentiality, Summary: &summary,
	})
	if err != nil {
		return nil, err
	}
	relation := "reporter"
	if input.IsAnonymous {
		relation = "anonymous_reporter"
	}
	return &EmployeeRelationsCaseSummary{ID: item.ID, CaseType: item.CaseType, Status: item.Status, Relation: relation,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}, nil
}

// canViewCase applies Section 5.4. Visibility is never inferred from the
// Group/Role hierarchy, and the subject never gets the full case even when
// they hold a broad HR permission.
func canViewCase(ctx context.Context, db *sql.DB, actor HRActor, item *EmployeeRelationsCase) (bool, error) {
	if actor.UserID == item.SubjectUserID {
		return false, nil
	}
	if actor.UserID == item.AssignedToUserID {
		return true, nil
	}
	if item.ConfidentialityLevel == CaseConfidentialityStandard {
		return actor.Has(PermissionHREmployeeRelationsRead), nil
	}
	if !actor.Has(PermissionHREmployeeRelationsManage) {
		return false, nil
	}
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM employee_relations_case_handlers WHERE case_id = ? AND user_id = ?`,
		item.ID, actor.UserID).Scan(&count)
	return count > 0, err
}

func (s *HRService) GetEmployeeRelationsCase(ctx context.Context, actor HRActor, id string) (*EmployeeRelationsCase, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	item, err := getEmployeeRelationsCase(ctx, db, id)
	if err != nil {
		return nil, err
	}
	visible, err := canViewCase(ctx, db, actor, item)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, ErrHRNotFound
	}
	rows, err := db.QueryContext(ctx, `SELECT action, actor_user_id, detail, created_at FROM hr_audit_log
		WHERE entity_type = 'employee_relations_case' AND entity_id = ? ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	item.History = []EmployeeRelationsNote{}
	for rows.Next() {
		var note EmployeeRelationsNote
		if err := rows.Scan(&note.Action, &note.ActorUserID, &note.Detail, &note.CreatedAt); err != nil {
			return nil, err
		}
		item.History = append(item.History, note)
	}
	return item, rows.Err()
}

// ListEmployeeRelationsCases returns every case the actor may see.
func (s *HRService) ListEmployeeRelationsCases(ctx context.Context, actor HRActor, query ListQuery) (ListResult[EmployeeRelationsCase], error) {
	if !actor.Has(PermissionHREmployeeRelationsManage) && !actor.Has(PermissionHREmployeeRelationsRead) {
		return ListResult[EmployeeRelationsCase]{}, ErrHRForbidden
	}
	query, sortExpression, err := NormalizeListQuery(query, "updated_at", "desc", map[string]string{
		"created_at": "c.created_at",
		"updated_at": "c.updated_at",
		"status":     "c.status",
	})
	if err != nil {
		return ListResult[EmployeeRelationsCase]{}, err
	}
	visibility := `(c.assigned_to_user_id = ?`
	args := []any{actor.UserID}
	if actor.Has(PermissionHREmployeeRelationsRead) {
		visibility += ` OR c.confidentiality_level = 'standard'`
	}
	if actor.Has(PermissionHREmployeeRelationsManage) {
		visibility += ` OR (c.confidentiality_level = 'restricted' AND EXISTS (
			SELECT 1 FROM employee_relations_case_handlers h WHERE h.case_id = c.id AND h.user_id = ?))`
		args = append(args, actor.UserID)
	}
	where := []string{visibility + `)`, `c.subject_user_id <> ?`}
	args = append(args, actor.UserID)
	for _, filter := range []string{"status", "case_type", "subject_user_id", "assigned_to_user_id"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			where = append(where, "c."+filter+" = ?")
			args = append(args, value)
		}
	}
	whereSQL := strings.Join(where, " AND ")
	db, err := s.open()
	if err != nil {
		return ListResult[EmployeeRelationsCase]{}, err
	}
	defer db.Close()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM employee_relations_cases c WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[EmployeeRelationsCase]{}, err
	}
	columns := "c." + strings.ReplaceAll(strings.Join(strings.Fields(employeeRelationsCaseColumns), " "), ", ", ", c.")
	rows, err := db.QueryContext(ctx, `SELECT `+columns+` FROM employee_relations_cases c WHERE `+whereSQL+`
		ORDER BY `+sortExpression+` `+query.Order+`, c.id ASC LIMIT ? OFFSET ?`, append(args, query.PageSize, ListOffset(query))...)
	if err != nil {
		return ListResult[EmployeeRelationsCase]{}, err
	}
	defer rows.Close()
	items := make([]EmployeeRelationsCase, 0, query.PageSize)
	for rows.Next() {
		item, err := scanEmployeeRelationsCase(rows)
		if err != nil {
			return ListResult[EmployeeRelationsCase]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult[EmployeeRelationsCase]{}, err
	}
	return NewListResult(items, query, total), nil
}

// ListMyEmployeeRelationsCases returns the read.self view of cases the actor
// reported (non-anonymously) or is the subject of.
func (s *HRService) ListMyEmployeeRelationsCases(ctx context.Context, actor HRActor) ([]EmployeeRelationsCaseSummary, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, case_type, status,
		CASE WHEN subject_user_id = ? THEN 'subject' ELSE 'reporter' END, created_at, updated_at
		FROM employee_relations_cases WHERE subject_user_id = ? OR reported_by_user_id = ?
		ORDER BY updated_at DESC, id`, actor.UserID, actor.UserID, actor.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []EmployeeRelationsCaseSummary{}
	for rows.Next() {
		var item EmployeeRelationsCaseSummary
		if err := rows.Scan(&item.ID, &item.CaseType, &item.Status, &item.Relation, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// handlerCase loads a case the actor currently handles; anyone else gets
// not-found so case existence is not disclosed.
func (s *HRService) handlerCase(ctx context.Context, db *sql.DB, actor HRActor, id string) (*EmployeeRelationsCase, error) {
	item, err := getEmployeeRelationsCase(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if item.AssignedToUserID != actor.UserID || !actor.Has(PermissionHREmployeeRelationsManage) {
		return nil, ErrHRNotFound
	}
	return item, nil
}

func (s *HRService) UpdateEmployeeRelationsCase(ctx context.Context, actor HRActor, id string, input UpdateEmployeeRelationsCaseInput) (*EmployeeRelationsCase, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	item, err := s.handlerCase(ctx, db, actor, id)
	if err != nil {
		return nil, err
	}
	if item.Status == CaseStatusClosed {
		return nil, fmt.Errorf("%w: closed cases cannot change", ErrHRInvalidTransition)
	}
	sets, args := []string{"updated_at = ?"}, []any{s.timestamp()}
	detail := map[string]string{}
	if input.Summary != nil {
		sets = append(sets, "summary = ?")
		args = append(args, trimmedOrNil(input.Summary))
		detail["summary"] = "changed"
	}
	if input.ConfidentialityLevel != nil {
		level, err := normalizeConfidentiality(*input.ConfidentialityLevel)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "confidentiality_level = ?")
		args = append(args, level)
		detail["confidentiality_level"] = level
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, `UPDATE employee_relations_cases SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "employee_relations_case", id, "updated", actor.UserID, detail); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetEmployeeRelationsCase(ctx, actor, id)
}

// TransitionEmployeeRelationsCase moves a case through the Section 5.3
// workflow. Only the current handler may act.
//
//	open/escalated --investigate--> investigating --resolve--> resolved --close--> closed
//	open/investigating --escalate--> escalated (reassigned to a higher HR role)
func (s *HRService) TransitionEmployeeRelationsCase(ctx context.Context, actor HRActor, id string, input TransitionEmployeeRelationsCaseInput) (*EmployeeRelationsCase, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	item, err := s.handlerCase(ctx, db, actor, id)
	if err != nil {
		return nil, err
	}
	now := s.timestamp()
	sets, args := []string{"updated_at = ?"}, []any{now}
	detail := map[string]string{"from": item.Status}
	if note := trimmedOrNil(input.Note); note != nil {
		detail["note"] = *note
	}
	next, newHandler := "", ""
	switch input.Action {
	case CaseActionInvestigate:
		if item.Status != CaseStatusOpen && item.Status != CaseStatusEscalated {
			return nil, fmt.Errorf("%w: case is %s", ErrHRInvalidTransition, item.Status)
		}
		next = CaseStatusInvestigating
	case CaseActionResolve:
		if item.Status != CaseStatusInvestigating {
			return nil, fmt.Errorf("%w: only an investigating case can be resolved", ErrHRInvalidTransition)
		}
		notes := trimmedOrNil(input.ResolutionNotes)
		if notes == nil {
			return nil, fmt.Errorf("%w: resolution_notes is required", ErrHRInvalidInput)
		}
		next = CaseStatusResolved
		sets = append(sets, "resolution_notes = ?")
		args = append(args, *notes)
	case CaseActionClose:
		if item.Status != CaseStatusResolved {
			return nil, fmt.Errorf("%w: only a resolved case can be closed", ErrHRInvalidTransition)
		}
		next = CaseStatusClosed
		if notes := trimmedOrNil(input.ResolutionNotes); notes != nil {
			sets = append(sets, "resolution_notes = ?")
			args = append(args, *notes)
		}
	case CaseActionEscalate:
		if item.Status != CaseStatusOpen && item.Status != CaseStatusInvestigating {
			return nil, fmt.Errorf("%w: case is %s", ErrHRInvalidTransition, item.Status)
		}
		target := trimmedOrNil(input.AssignedToUserID)
		if target == nil || *target == item.AssignedToUserID {
			return nil, fmt.Errorf("%w: escalation requires a different assigned_to_user_id", ErrHRInvalidInput)
		}
		if err := s.validateHandler(ctx, *target, item.SubjectUserID, true); err != nil {
			return nil, err
		}
		next, newHandler = CaseStatusEscalated, *target
		sets = append(sets, "assigned_to_user_id = ?")
		args = append(args, newHandler)
		detail["assigned_to_user_id"] = newHandler
	default:
		return nil, fmt.Errorf("%w: unknown action %q", ErrHRInvalidInput, input.Action)
	}
	detail["to"] = next
	sets = append(sets, "status = ?")
	args = append(args, next, id)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE employee_relations_cases SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if newHandler != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO employee_relations_case_handlers (case_id, user_id, assigned_at) VALUES (?, ?, ?)`,
			id, newHandler, now); err != nil {
			return nil, err
		}
	}
	if err := s.audit(ctx, tx, "employee_relations_case", id, input.Action, actor.UserID, detail); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if newHandler != "" {
		s.notifyCaseHandler(ctx, id, newHandler)
	}
	if (next == CaseStatusResolved || next == CaseStatusClosed) && item.ReportedByUserID != nil {
		s.notifyUser(ctx, *item.ReportedByUserID, "Employee relations case updated",
			fmt.Sprintf("Employee relations case %s is now %s.", id, next))
	}
	if newHandler != "" {
		// The escalating handler keeps visibility through the handler
		// history but no longer acts on the case.
		item, err := getEmployeeRelationsCase(ctx, db, id)
		return item, err
	}
	return s.GetEmployeeRelationsCase(ctx, actor, id)
}
