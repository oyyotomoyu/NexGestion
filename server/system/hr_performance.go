package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Performance values. See docs/System/hr-system.md Section 4.
const (
	PerformanceCycleDraft  = "draft"
	PerformanceCycleOpen   = "open"
	PerformanceCycleClosed = "closed"

	PerformanceGoalDraft     = "draft"
	PerformanceGoalActive    = "active"
	PerformanceGoalCompleted = "completed"
	PerformanceGoalCancelled = "cancelled"

	PerformanceReviewDraft         = "draft"
	PerformanceReviewSelfReview    = "self_review"
	PerformanceReviewManagerReview = "manager_review"
	PerformanceReviewFinalized     = "finalized"
)

var (
	validPerformanceGoalStatuses = map[string]bool{
		PerformanceGoalDraft: true, PerformanceGoalActive: true, PerformanceGoalCompleted: true, PerformanceGoalCancelled: true,
	}
	goalWeightPattern = regexp.MustCompile(`^\d+(\.\d{1,4})?$`)
)

type PerformanceCycle struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	Status          string `json:"status"`
	CreatedByUserID string `json:"created_by_user_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type CreatePerformanceCycleInput struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type UpdatePerformanceCycleInput struct {
	Name      *string `json:"name"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
	Status    *string `json:"status"`
}

type PerformanceGoal struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	CycleID         string  `json:"cycle_id"`
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	Weight          *string `json:"weight"`
	Status          string  `json:"status"`
	CreatedByUserID string  `json:"created_by_user_id"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type CreatePerformanceGoalInput struct {
	CycleID     string  `json:"cycle_id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Weight      *string `json:"weight"`
	Status      string  `json:"status"`
}

type UpdatePerformanceGoalInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Weight      *string `json:"weight"`
	Status      *string `json:"status"`
}

type PerformanceReviewCorrection struct {
	ID              string  `json:"id"`
	ReviewID        string  `json:"review_id"`
	Rating          *string `json:"rating"`
	ManagerComments *string `json:"manager_comments"`
	Reason          string  `json:"reason"`
	ActorUserID     string  `json:"actor_user_id"`
	CreatedAt       string  `json:"created_at"`
}

type PerformanceReview struct {
	ID              string  `json:"id"`
	UserID          string  `json:"user_id"`
	CycleID         string  `json:"cycle_id"`
	ReviewerUserID  *string `json:"reviewer_user_id"`
	Status          string  `json:"status"`
	Rating          *string `json:"rating"`
	SelfComments    *string `json:"self_comments"`
	ManagerComments *string `json:"manager_comments"`
	FinalizedAt     *string `json:"finalized_at"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	// EffectiveRating/EffectiveManagerComments apply the latest correction
	// on top of the finalized result (Section 4.3).
	EffectiveRating          *string                       `json:"effective_rating"`
	EffectiveManagerComments *string                       `json:"effective_manager_comments"`
	Corrections              []PerformanceReviewCorrection `json:"corrections"`
}

type SubmitSelfReviewInput struct {
	SelfComments string `json:"self_comments"`
}

type SubmitManagerReviewInput struct {
	Rating          string `json:"rating"`
	ManagerComments string `json:"manager_comments"`
}

type CorrectPerformanceReviewInput struct {
	Rating          *string `json:"rating"`
	ManagerComments *string `json:"manager_comments"`
	Reason          string  `json:"reason"`
}

// --- Cycles (Section 4.1) ---

const performanceCycleColumns = `id, name, start_date, end_date, status, created_by_user_id, created_at, updated_at`

func scanPerformanceCycle(scanner interface{ Scan(...any) error }) (PerformanceCycle, error) {
	var c PerformanceCycle
	err := scanner.Scan(&c.ID, &c.Name, &c.StartDate, &c.EndDate, &c.Status, &c.CreatedByUserID, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func getPerformanceCycle(ctx context.Context, db *sql.DB, id string) (*PerformanceCycle, error) {
	cycle, err := scanPerformanceCycle(db.QueryRowContext(ctx, `SELECT `+performanceCycleColumns+` FROM performance_cycles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHRNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cycle, nil
}

func (s *HRService) CreatePerformanceCycle(ctx context.Context, actor HRActor, input CreatePerformanceCycleInput) (*PerformanceCycle, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrHRInvalidInput)
	}
	start, err := parseHRDate("start_date", input.StartDate)
	if err != nil {
		return nil, err
	}
	end, err := parseHRDate("end_date", input.EndDate)
	if err != nil {
		return nil, err
	}
	if end < start {
		return nil, fmt.Errorf("%w: end_date must not be before start_date", ErrHRInvalidInput)
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
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO performance_cycles (`+performanceCycleColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, start, end, PerformanceCycleDraft, actor.UserID, now, now); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_cycle", id, "created", actor.UserID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getPerformanceCycle(ctx, db, id)
}

// UpdatePerformanceCycle edits a cycle. Dates may change only while draft;
// status moves forward only: draft -> open -> closed.
func (s *HRService) UpdatePerformanceCycle(ctx context.Context, actor HRActor, id string, input UpdatePerformanceCycleInput) (*PerformanceCycle, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	cycle, err := getPerformanceCycle(ctx, db, id)
	if err != nil {
		return nil, err
	}
	sets, args := []string{"updated_at = ?"}, []any{s.timestamp()}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", ErrHRInvalidInput)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	start, end := cycle.StartDate, cycle.EndDate
	if input.StartDate != nil || input.EndDate != nil {
		if cycle.Status != PerformanceCycleDraft {
			return nil, fmt.Errorf("%w: cycle dates can only change while draft", ErrHRInvalidTransition)
		}
		if input.StartDate != nil {
			if start, err = parseHRDate("start_date", *input.StartDate); err != nil {
				return nil, err
			}
		}
		if input.EndDate != nil {
			if end, err = parseHRDate("end_date", *input.EndDate); err != nil {
				return nil, err
			}
		}
		if end < start {
			return nil, fmt.Errorf("%w: end_date must not be before start_date", ErrHRInvalidInput)
		}
		sets = append(sets, "start_date = ?", "end_date = ?")
		args = append(args, start, end)
	}
	if input.Status != nil && *input.Status != cycle.Status {
		allowed := (cycle.Status == PerformanceCycleDraft && *input.Status == PerformanceCycleOpen) ||
			(cycle.Status == PerformanceCycleOpen && *input.Status == PerformanceCycleClosed)
		if !allowed {
			return nil, fmt.Errorf("%w: cycle cannot move from %s to %s", ErrHRInvalidTransition, cycle.Status, *input.Status)
		}
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, `UPDATE performance_cycles SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_cycle", id, "updated", actor.UserID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return getPerformanceCycle(ctx, db, id)
}

func (s *HRService) ListPerformanceCycles(ctx context.Context, query ListQuery) (ListResult[PerformanceCycle], error) {
	query, sortExpression, err := NormalizeListQuery(query, "start_date", "desc", map[string]string{
		"start_date": "start_date",
		"name":       "name COLLATE NOCASE",
		"created_at": "created_at",
	})
	if err != nil {
		return ListResult[PerformanceCycle]{}, err
	}
	where, args := "1=1", []any{}
	if status := strings.TrimSpace(query.Filters["status"]); status != "" {
		where, args = "status = ?", append(args, status)
	}
	db, err := s.open()
	if err != nil {
		return ListResult[PerformanceCycle]{}, err
	}
	defer db.Close()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_cycles WHERE `+where, args...).Scan(&total); err != nil {
		return ListResult[PerformanceCycle]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+performanceCycleColumns+` FROM performance_cycles WHERE `+where+`
		ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, append(args, query.PageSize, ListOffset(query))...)
	if err != nil {
		return ListResult[PerformanceCycle]{}, err
	}
	defer rows.Close()
	items := make([]PerformanceCycle, 0, query.PageSize)
	for rows.Next() {
		cycle, err := scanPerformanceCycle(rows)
		if err != nil {
			return ListResult[PerformanceCycle]{}, err
		}
		items = append(items, cycle)
	}
	if err := rows.Err(); err != nil {
		return ListResult[PerformanceCycle]{}, err
	}
	return NewListResult(items, query, total), nil
}

// --- Visibility (Section 4.5) ---

// canViewPerformance: self always; hr.performance.read within the reporting
// line; hr.performance.read together with hr.performance.cycles.manage is
// the broader HR-administrator view across reporting lines.
func (s *HRService) canViewPerformance(ctx context.Context, actor HRActor, userID string) (bool, error) {
	if actor.UserID == userID {
		return true, nil
	}
	if !actor.Has(PermissionHRPerformanceRead) {
		return false, nil
	}
	if actor.Has(PermissionHRPerformanceCyclesManage) {
		return true, nil
	}
	return s.users.ManagesUser(ctx, actor.UserID, userID)
}

// canManagePerformance: a manager with hr.performance.review over a direct
// or indirect report. Never oneself.
func (s *HRService) canManagePerformance(ctx context.Context, actor HRActor, userID string) (bool, error) {
	if actor.UserID == userID || !actor.Has(PermissionHRPerformanceReview) {
		return false, nil
	}
	return s.users.ManagesUser(ctx, actor.UserID, userID)
}

// --- Goals (Section 4.2) ---

const performanceGoalColumns = `id, user_id, cycle_id, title, description, weight, status, created_by_user_id, created_at, updated_at`

func scanPerformanceGoal(scanner interface{ Scan(...any) error }) (PerformanceGoal, error) {
	var g PerformanceGoal
	err := scanner.Scan(&g.ID, &g.UserID, &g.CycleID, &g.Title, &g.Description, &g.Weight, &g.Status,
		&g.CreatedByUserID, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

func normalizeGoalWeight(value *string) (*string, error) {
	weight := trimmedOrNil(value)
	if weight != nil && !goalWeightPattern.MatchString(*weight) {
		return nil, fmt.Errorf("%w: weight must be a non-negative decimal number", ErrHRInvalidInput)
	}
	return weight, nil
}

// goalsEditable reports whether goals in the cycle may still change: the
// cycle is not closed and the employee's review is not finalized.
func goalsEditable(ctx context.Context, db *sql.DB, userID, cycleID string) error {
	cycle, err := getPerformanceCycle(ctx, db, cycleID)
	if err != nil {
		return err
	}
	if cycle.Status == PerformanceCycleClosed {
		return fmt.Errorf("%w: cycle is closed", ErrHRInvalidTransition)
	}
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM performance_reviews WHERE user_id = ? AND cycle_id = ?`, userID, cycleID).Scan(&status)
	if err == nil && status == PerformanceReviewFinalized {
		return fmt.Errorf("%w: review is finalized", ErrHRInvalidTransition)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

// CreatePerformanceGoal creates a goal owned by userID. Employees set their
// own goals; managers set goals for their reporting line.
func (s *HRService) CreatePerformanceGoal(ctx context.Context, actor HRActor, userID string, input CreatePerformanceGoalInput) (*PerformanceGoal, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	if actor.UserID != userID {
		allowed, err := s.canManagePerformance(ctx, actor, userID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrHRForbidden
		}
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("%w: title is required", ErrHRInvalidInput)
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = PerformanceGoalDraft
	}
	if !validPerformanceGoalStatuses[status] {
		return nil, fmt.Errorf("%w: unknown goal status %q", ErrHRInvalidInput, status)
	}
	weight, err := normalizeGoalWeight(input.Weight)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := goalsEditable(ctx, db, userID, strings.TrimSpace(input.CycleID)); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO performance_goals (`+performanceGoalColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, strings.TrimSpace(input.CycleID), title, trimmedOrNil(input.Description), weight, status, actor.UserID, now, now); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_goal", id, "created", actor.UserID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	goal, err := scanPerformanceGoal(db.QueryRowContext(ctx, `SELECT `+performanceGoalColumns+` FROM performance_goals WHERE id = ?`, id))
	return &goal, err
}

func (s *HRService) UpdatePerformanceGoal(ctx context.Context, actor HRActor, id string, input UpdatePerformanceGoalInput) (*PerformanceGoal, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	goal, err := scanPerformanceGoal(db.QueryRowContext(ctx, `SELECT `+performanceGoalColumns+` FROM performance_goals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHRNotFound
	}
	if err != nil {
		return nil, err
	}
	if actor.UserID != goal.UserID {
		allowed, err := s.canManagePerformance(ctx, actor, goal.UserID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrHRNotFound
		}
	}
	if err := goalsEditable(ctx, db, goal.UserID, goal.CycleID); err != nil {
		return nil, err
	}
	sets, args := []string{"updated_at = ?"}, []any{s.timestamp()}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return nil, fmt.Errorf("%w: title cannot be empty", ErrHRInvalidInput)
		}
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, trimmedOrNil(input.Description))
	}
	if input.Weight != nil {
		weight, err := normalizeGoalWeight(input.Weight)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "weight = ?")
		args = append(args, weight)
	}
	if input.Status != nil {
		if !validPerformanceGoalStatuses[*input.Status] {
			return nil, fmt.Errorf("%w: unknown goal status %q", ErrHRInvalidInput, *input.Status)
		}
		sets = append(sets, "status = ?")
		args = append(args, *input.Status)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	args = append(args, id)
	if _, err := tx.ExecContext(ctx, `UPDATE performance_goals SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_goal", id, "updated", actor.UserID, input); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	updated, err := scanPerformanceGoal(db.QueryRowContext(ctx, `SELECT `+performanceGoalColumns+` FROM performance_goals WHERE id = ?`, id))
	return &updated, err
}

func (s *HRService) ListPerformanceGoals(ctx context.Context, actor HRActor, userID string, query ListQuery) (ListResult[PerformanceGoal], error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	allowed, err := s.canViewPerformance(ctx, actor, userID)
	if err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	if !allowed {
		return ListResult[PerformanceGoal]{}, ErrHRForbidden
	}
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "asc", map[string]string{
		"created_at": "created_at",
		"title":      "title COLLATE NOCASE",
		"status":     "status",
	})
	if err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	where, args := []string{"user_id = ?"}, []any{userID}
	for _, filter := range []string{"cycle_id", "status"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			where = append(where, filter+" = ?")
			args = append(args, value)
		}
	}
	whereSQL := strings.Join(where, " AND ")
	db, err := s.open()
	if err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	defer db.Close()
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_goals WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+performanceGoalColumns+` FROM performance_goals WHERE `+whereSQL+`
		ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, append(args, query.PageSize, ListOffset(query))...)
	if err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	defer rows.Close()
	items := make([]PerformanceGoal, 0, query.PageSize)
	for rows.Next() {
		goal, err := scanPerformanceGoal(rows)
		if err != nil {
			return ListResult[PerformanceGoal]{}, err
		}
		items = append(items, goal)
	}
	if err := rows.Err(); err != nil {
		return ListResult[PerformanceGoal]{}, err
	}
	return NewListResult(items, query, total), nil
}

// --- Reviews (Section 4.3) ---

const performanceReviewColumns = `id, user_id, cycle_id, reviewer_user_id, status, rating, self_comments, manager_comments,
	finalized_at, created_at, updated_at`

func scanPerformanceReview(scanner interface{ Scan(...any) error }) (PerformanceReview, error) {
	var r PerformanceReview
	err := scanner.Scan(&r.ID, &r.UserID, &r.CycleID, &r.ReviewerUserID, &r.Status, &r.Rating, &r.SelfComments,
		&r.ManagerComments, &r.FinalizedAt, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *HRService) loadReviewCorrections(ctx context.Context, db *sql.DB, review *PerformanceReview) error {
	review.Corrections = []PerformanceReviewCorrection{}
	review.EffectiveRating, review.EffectiveManagerComments = review.Rating, review.ManagerComments
	rows, err := db.QueryContext(ctx, `SELECT id, review_id, rating, manager_comments, reason, actor_user_id, created_at
		FROM performance_review_corrections WHERE review_id = ? ORDER BY created_at, id`, review.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var c PerformanceReviewCorrection
		if err := rows.Scan(&c.ID, &c.ReviewID, &c.Rating, &c.ManagerComments, &c.Reason, &c.ActorUserID, &c.CreatedAt); err != nil {
			return err
		}
		if c.Rating != nil {
			review.EffectiveRating = c.Rating
		}
		if c.ManagerComments != nil {
			review.EffectiveManagerComments = c.ManagerComments
		}
		review.Corrections = append(review.Corrections, c)
	}
	return rows.Err()
}

// redactForEmployee hides the manager's assessment from the employee until
// the review is finalized (Section 4.5).
func redactForEmployee(review *PerformanceReview) {
	if review.Status == PerformanceReviewFinalized {
		return
	}
	review.Rating, review.ManagerComments = nil, nil
	review.EffectiveRating, review.EffectiveManagerComments = nil, nil
}

func (s *HRService) getPerformanceReview(ctx context.Context, db *sql.DB, query string, args ...any) (*PerformanceReview, error) {
	review, err := scanPerformanceReview(db.QueryRowContext(ctx, `SELECT `+performanceReviewColumns+` FROM performance_reviews WHERE `+query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHRNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.loadReviewCorrections(ctx, db, &review); err != nil {
		return nil, err
	}
	return &review, nil
}

// SubmitSelfReview records the employee's self-assessment in an open cycle,
// moving the review from draft to self_review.
func (s *HRService) SubmitSelfReview(ctx context.Context, actor HRActor, cycleID string, input SubmitSelfReviewInput) (*PerformanceReview, error) {
	comments := strings.TrimSpace(input.SelfComments)
	if comments == "" {
		return nil, fmt.Errorf("%w: self_comments is required", ErrHRInvalidInput)
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	cycle, err := getPerformanceCycle(ctx, db, cycleID)
	if err != nil {
		return nil, err
	}
	if cycle.Status != PerformanceCycleOpen {
		return nil, fmt.Errorf("%w: cycle is not open", ErrHRInvalidTransition)
	}
	existing, err := s.getPerformanceReview(ctx, db, `user_id = ? AND cycle_id = ?`, actor.UserID, cycleID)
	if err != nil && !errors.Is(err, ErrHRNotFound) {
		return nil, err
	}
	if existing != nil && existing.Status != PerformanceReviewDraft && existing.Status != PerformanceReviewSelfReview {
		return nil, fmt.Errorf("%w: self review can no longer be changed", ErrHRInvalidTransition)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	id := uuid.NewString()
	if existing == nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO performance_reviews (`+performanceReviewColumns+`)
			VALUES (?, ?, ?, NULL, ?, NULL, ?, NULL, NULL, ?, ?)`,
			id, actor.UserID, cycleID, PerformanceReviewSelfReview, comments, now, now); err != nil {
			return nil, err
		}
	} else {
		id = existing.ID
		if _, err := tx.ExecContext(ctx, `UPDATE performance_reviews SET status = ?, self_comments = ?, updated_at = ? WHERE id = ?`,
			PerformanceReviewSelfReview, comments, now, id); err != nil {
			return nil, err
		}
	}
	if err := s.audit(ctx, tx, "performance_review", id, "self_review_submitted", actor.UserID, map[string]string{"cycle_id": cycleID}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	review, err := s.getPerformanceReview(ctx, db, `id = ?`, id)
	if err != nil {
		return nil, err
	}
	redactForEmployee(review)
	return review, nil
}

func (s *HRService) validRating(ctx context.Context, db *sql.DB, rating string) error {
	settings, err := readHRSettings(ctx, db)
	if err != nil {
		return err
	}
	for _, value := range settings.PerformanceRatingScale {
		if value == rating {
			return nil
		}
	}
	return fmt.Errorf("%w: rating must be one of %v", ErrHRInvalidInput, settings.PerformanceRatingScale)
}

// SubmitManagerReview records the manager's assessment, moving the review
// from self_review to manager_review. The manager may revise it until it is
// finalized.
func (s *HRService) SubmitManagerReview(ctx context.Context, actor HRActor, userID, cycleID string, input SubmitManagerReviewInput) (*PerformanceReview, error) {
	allowed, err := s.canManagePerformance(ctx, actor, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrHRForbidden
	}
	rating := strings.TrimSpace(input.Rating)
	comments := strings.TrimSpace(input.ManagerComments)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := s.validRating(ctx, db, rating); err != nil {
		return nil, err
	}
	review, err := s.getPerformanceReview(ctx, db, `user_id = ? AND cycle_id = ?`, userID, cycleID)
	if err != nil {
		return nil, err
	}
	if review.Status != PerformanceReviewSelfReview && review.Status != PerformanceReviewManagerReview {
		return nil, fmt.Errorf("%w: review must have a submitted self review", ErrHRInvalidTransition)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE performance_reviews SET status = ?, reviewer_user_id = ?, rating = ?,
		manager_comments = ?, updated_at = ? WHERE id = ?`,
		PerformanceReviewManagerReview, actor.UserID, rating, nullableTrim(comments), s.timestamp(), review.ID); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_review", review.ID, "manager_review_submitted", actor.UserID, map[string]string{"rating": rating}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getPerformanceReview(ctx, db, `id = ?`, review.ID)
}

// FinalizePerformanceReview closes the review and makes it visible to the
// employee.
func (s *HRService) FinalizePerformanceReview(ctx context.Context, actor HRActor, userID, cycleID string) (*PerformanceReview, error) {
	allowed, err := s.canManagePerformance(ctx, actor, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrHRForbidden
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	review, err := s.getPerformanceReview(ctx, db, `user_id = ? AND cycle_id = ?`, userID, cycleID)
	if err != nil {
		return nil, err
	}
	if review.Status != PerformanceReviewManagerReview {
		return nil, fmt.Errorf("%w: review must have a manager review before finalizing", ErrHRInvalidTransition)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE performance_reviews SET status = ?, finalized_at = ?, updated_at = ? WHERE id = ?`,
		PerformanceReviewFinalized, now, now, review.ID); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_review", review.ID, "finalized", actor.UserID, map[string]string{}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notifyUser(ctx, userID, "Performance review finalized", fmt.Sprintf("Performance review %s is now available.", review.ID))
	return s.getPerformanceReview(ctx, db, `id = ?`, review.ID)
}

// CorrectPerformanceReview appends a correction to a finalized review
// instead of overwriting it (Section 4.3, Section 8).
func (s *HRService) CorrectPerformanceReview(ctx context.Context, actor HRActor, reviewID string, input CorrectPerformanceReviewInput) (*PerformanceReview, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	review, err := s.getPerformanceReview(ctx, db, `id = ?`, reviewID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.canManagePerformance(ctx, actor, review.UserID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrHRNotFound
	}
	if review.Status != PerformanceReviewFinalized {
		return nil, fmt.Errorf("%w: only a finalized review is corrected; revise the manager review instead", ErrHRInvalidTransition)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrHRInvalidInput)
	}
	rating := trimmedOrNil(input.Rating)
	comments := trimmedOrNil(input.ManagerComments)
	if rating == nil && comments == nil {
		return nil, fmt.Errorf("%w: rating or manager_comments is required", ErrHRInvalidInput)
	}
	if rating != nil {
		if err := s.validRating(ctx, db, *rating); err != nil {
			return nil, err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO performance_review_corrections
		(id, review_id, rating, manager_comments, reason, actor_user_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, reviewID, rating, comments, reason, actor.UserID, s.timestamp()); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, "performance_review", reviewID, "corrected", actor.UserID, map[string]string{"correction_id": id, "reason": reason}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getPerformanceReview(ctx, db, `id = ?`, reviewID)
}

// ListPerformanceReviews lists one employee's reviews. When the caller is
// the employee, unfinalized manager assessments are redacted.
func (s *HRService) ListPerformanceReviews(ctx context.Context, actor HRActor, userID string) ([]PerformanceReview, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	allowed, err := s.canViewPerformance(ctx, actor, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrHRForbidden
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT `+performanceReviewColumns+` FROM performance_reviews WHERE user_id = ? ORDER BY created_at DESC, id`, userID)
	if err != nil {
		return nil, err
	}
	items := []PerformanceReview{}
	for rows.Next() {
		review, err := scanPerformanceReview(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, review)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range items {
		if err := s.loadReviewCorrections(ctx, db, &items[index]); err != nil {
			return nil, err
		}
		if actor.UserID == userID {
			redactForEmployee(&items[index])
		}
	}
	return items, nil
}
