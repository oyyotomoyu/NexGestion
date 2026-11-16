package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Scheduling implements the v1 scope of docs/System/scheduling-system.md:
// group-scoped schedule periods, shifts, named assignments, the publication
// lifecycle, conflict validation with audited overrides, and self-service
// shift views. Templates, reports, approval routing, and Attendance/Salary
// integration are deferred (Section 14 of the document).

const (
	SchedulingGroupRequired = "required"
	SchedulingGroupAdvisory = "advisory"

	ScheduleDraft     = "draft"
	SchedulePublished = "published"
	ScheduleLocked    = "locked"
	ScheduleArchived  = "archived"
	ScheduleCancelled = "cancelled"

	schedulingLocalLayout  = "2006-01-02T15:04"
	schedulingDateLayout   = "2006-01-02"
	schedulingMaxPeriod    = 366
	schedulingMaxSelfRange = 93
	schedulingMaxShift     = 24 * 60
)

var (
	ErrSchedulingInvalid  = errors.New("invalid scheduling input")
	ErrSchedulingNotFound = errors.New("scheduling record not found")
	ErrSchedulingState    = errors.New("scheduling action conflicts with current status")
)

// SchedulingConflictError carries blocking validation results. The caller may
// retry with an override reason when it holds scheduling.override.
type SchedulingConflictError struct {
	Conflicts []SchedulingConflict
}

func (e *SchedulingConflictError) Error() string {
	return "scheduling validation failed; an override reason is required"
}

type SchedulingConfig struct {
	GroupMode        string `json:"group_mode"`
	AllowOpenShifts  bool   `json:"allow_open_shifts"`
	MaxWeeklyMinutes int    `json:"max_weekly_minutes"`
	MinRestMinutes   int    `json:"min_rest_minutes"`
}

type UpdateSchedulingConfigInput struct {
	GroupMode        *string `json:"group_mode"`
	AllowOpenShifts  *bool   `json:"allow_open_shifts"`
	MaxWeeklyMinutes *int    `json:"max_weekly_minutes"`
	MinRestMinutes   *int    `json:"min_rest_minutes"`
}

type SchedulePeriod struct {
	ID              string          `json:"id"`
	GroupID         string          `json:"group_id"`
	GroupName       string          `json:"group_name"`
	GroupType       string          `json:"group_type"`
	Name            string          `json:"name"`
	Timezone        string          `json:"timezone"`
	StartDate       string          `json:"start_date"`
	EndDate         string          `json:"end_date"`
	Status          string          `json:"status"`
	CreatedByUserID string          `json:"created_by_user_id"`
	PublishedAt     *string         `json:"published_at"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
	ShiftCount      int             `json:"shift_count"`
	Shifts          []ScheduleShift `json:"shifts,omitempty"`
}

type CreateSchedulePeriodInput struct {
	GroupID   string `json:"group_id"`
	Name      string `json:"name"`
	Timezone  string `json:"timezone"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type UpdateSchedulePeriodInput struct {
	Name      *string `json:"name"`
	Timezone  *string `json:"timezone"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

type ScheduleShift struct {
	ID                string               `json:"id"`
	SchedulePeriodID  string               `json:"schedule_period_id"`
	Title             *string              `json:"title"`
	LocalStartAt      string               `json:"local_start_at"`
	LocalEndAt        string               `json:"local_end_at"`
	StartAtUTC        string               `json:"start_at_utc"`
	EndAtUTC          string               `json:"end_at_utc"`
	PlannedMinutes    int                  `json:"planned_minutes"`
	RequiredHeadcount int                  `json:"required_headcount"`
	AssignmentMode    string               `json:"assignment_mode"`
	Notes             *string              `json:"notes"`
	Status            string               `json:"status"`
	AssignedCount     int                  `json:"assigned_count"`
	OpenSlots         int                  `json:"open_slots"`
	Assignments       []ScheduleAssignment `json:"assignments"`
	CreatedAt         string               `json:"created_at"`
	UpdatedAt         string               `json:"updated_at"`
}

type CreateScheduleShiftInput struct {
	Title             *string `json:"title"`
	LocalStartAt      string  `json:"local_start_at"`
	LocalEndAt        string  `json:"local_end_at"`
	RequiredHeadcount int     `json:"required_headcount"`
	AssignmentMode    string  `json:"assignment_mode"`
	Notes             *string `json:"notes"`
}

type UpdateScheduleShiftInput struct {
	Title             *string `json:"title"`
	LocalStartAt      *string `json:"local_start_at"`
	LocalEndAt        *string `json:"local_end_at"`
	RequiredHeadcount *int    `json:"required_headcount"`
	AssignmentMode    *string `json:"assignment_mode"`
	Notes             *string `json:"notes"`
	OverrideReason    string  `json:"override_reason"`
}

type ScheduleAssignment struct {
	ID               string  `json:"id"`
	ShiftID          string  `json:"shift_id"`
	UserID           string  `json:"user_id"`
	DisplayName      string  `json:"display_name,omitempty"`
	AssignmentStatus string  `json:"assignment_status"`
	Source           string  `json:"source"`
	OverrideReason   *string `json:"override_reason"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type CreateScheduleAssignmentInput struct {
	UserID         string `json:"user_id"`
	OverrideReason string `json:"override_reason"`
}

type SchedulingConflict struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	ShiftID        string `json:"shift_id,omitempty"`
	RelatedShiftID string `json:"related_shift_id,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	Week           string `json:"week,omitempty"`
	// relatedOverridden marks an overlap whose other assignment was already
	// accepted with an audited override.
	relatedOverridden bool
}

type ScheduleCandidate struct {
	UserID          string               `json:"user_id"`
	DisplayName     string               `json:"display_name"`
	Email           string               `json:"email"`
	Title           *string              `json:"title"`
	AlreadyAssigned bool                 `json:"already_assigned"`
	Conflicts       []SchedulingConflict `json:"conflicts"`
}

type MyScheduledShift struct {
	ScheduleShift
	PeriodName       string `json:"period_name"`
	PeriodStatus     string `json:"period_status"`
	GroupID          string `json:"group_id"`
	GroupName        string `json:"group_name"`
	Timezone         string `json:"timezone"`
	AssignmentID     string `json:"assignment_id"`
	AssignmentStatus string `json:"assignment_status"`
}

type SchedulingEvent struct {
	ID               string  `json:"id"`
	SchedulePeriodID *string `json:"schedule_period_id"`
	ShiftID          *string `json:"shift_id"`
	AssignmentID     *string `json:"assignment_id"`
	ActorUserID      string  `json:"actor_user_id"`
	EventType        string  `json:"event_type"`
	Reason           *string `json:"reason"`
	Detail           *string `json:"detail"`
	OccurredAt       string  `json:"occurred_at"`
}

type SchedulingService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewSchedulingService(databaseDirectory string, users *UserService) *SchedulingService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &SchedulingService{
		databasePath: filepath.Join(databaseDirectory, "scheduling.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *SchedulingService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}

type schedulingQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func schedulingInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSchedulingInvalid, fmt.Sprintf(format, args...))
}

func schedulingState(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSchedulingState, fmt.Sprintf(format, args...))
}

func (s *SchedulingService) timestamp() string {
	return s.now().UTC().Format(time.RFC3339)
}

// ---- Configuration ----

func defaultSchedulingConfig() SchedulingConfig {
	return SchedulingConfig{GroupMode: SchedulingGroupAdvisory, AllowOpenShifts: true}
}

func readSchedulingConfig(ctx context.Context, q schedulingQueryer) (SchedulingConfig, error) {
	config := defaultSchedulingConfig()
	rows, err := q.QueryContext(ctx, `SELECT key, value FROM scheduling_settings`)
	if err != nil {
		return config, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return config, err
		}
		switch key {
		case "group_mode":
			config.GroupMode = value
		case "allow_open_shifts":
			config.AllowOpenShifts = value == "true"
		case "max_weekly_minutes":
			config.MaxWeeklyMinutes, _ = strconv.Atoi(value)
		case "min_rest_minutes":
			config.MinRestMinutes, _ = strconv.Atoi(value)
		}
	}
	return config, rows.Err()
}

func (s *SchedulingService) Config(ctx context.Context) (SchedulingConfig, error) {
	db, err := s.open()
	if err != nil {
		return SchedulingConfig{}, err
	}
	defer db.Close()
	return readSchedulingConfig(ctx, db)
}

func (s *SchedulingService) UpdateConfig(ctx context.Context, actorUserID string, input UpdateSchedulingConfigInput) (SchedulingConfig, error) {
	db, err := s.open()
	if err != nil {
		return SchedulingConfig{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SchedulingConfig{}, err
	}
	defer tx.Rollback()
	config, err := readSchedulingConfig(ctx, tx)
	if err != nil {
		return config, err
	}
	if input.GroupMode != nil {
		mode := strings.TrimSpace(*input.GroupMode)
		if mode != SchedulingGroupRequired && mode != SchedulingGroupAdvisory {
			return config, schedulingInvalid("group_mode must be required or advisory")
		}
		config.GroupMode = mode
	}
	if input.AllowOpenShifts != nil {
		config.AllowOpenShifts = *input.AllowOpenShifts
	}
	if input.MaxWeeklyMinutes != nil {
		if *input.MaxWeeklyMinutes < 0 || *input.MaxWeeklyMinutes > 7*24*60 {
			return config, schedulingInvalid("max_weekly_minutes must be between 0 and 10080")
		}
		config.MaxWeeklyMinutes = *input.MaxWeeklyMinutes
	}
	if input.MinRestMinutes != nil {
		if *input.MinRestMinutes < 0 || *input.MinRestMinutes > 24*60 {
			return config, schedulingInvalid("min_rest_minutes must be between 0 and 1440")
		}
		config.MinRestMinutes = *input.MinRestMinutes
	}
	values := map[string]string{
		"group_mode":         config.GroupMode,
		"allow_open_shifts":  strconv.FormatBool(config.AllowOpenShifts),
		"max_weekly_minutes": strconv.Itoa(config.MaxWeeklyMinutes),
		"min_rest_minutes":   strconv.Itoa(config.MinRestMinutes),
	}
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO scheduling_settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return config, err
		}
	}
	detail, _ := json.Marshal(config)
	if err := s.recordEvent(ctx, tx, "", "", "", actorUserID, "config_updated", "", string(detail)); err != nil {
		return config, err
	}
	return config, tx.Commit()
}

// ---- Events ----

func (s *SchedulingService) recordEvent(ctx context.Context, q schedulingQueryer, periodID, shiftID, assignmentID, actorUserID, eventType, reason, detail string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO scheduling_events
		(id, schedule_period_id, shift_id, assignment_id, actor_user_id, event_type, reason, detail, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), nullableTrim(periodID), nullableTrim(shiftID), nullableTrim(assignmentID),
		actorUserID, eventType, nullableTrim(reason), nullableTrim(detail), s.timestamp())
	return err
}

func (s *SchedulingService) PeriodEvents(ctx context.Context, periodID string) ([]SchedulingEvent, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := loadSchedulePeriod(ctx, db, periodID); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, schedule_period_id, shift_id, assignment_id, actor_user_id,
		event_type, reason, detail, occurred_at FROM scheduling_events
		WHERE schedule_period_id=? ORDER BY occurred_at, rowid`, periodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []SchedulingEvent{}
	for rows.Next() {
		var event SchedulingEvent
		if err := rows.Scan(&event.ID, &event.SchedulePeriodID, &event.ShiftID, &event.AssignmentID, &event.ActorUserID,
			&event.EventType, &event.Reason, &event.Detail, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ---- Periods ----

const schedulePeriodColumns = `p.id, p.group_id, p.group_name, p.group_type, p.name, p.timezone, p.start_date, p.end_date,
	p.status, p.created_by_user_id, p.published_at, p.created_at, p.updated_at,
	(SELECT COUNT(*) FROM schedule_shifts s WHERE s.schedule_period_id=p.id AND s.status<>'cancelled')`

type schedulingScanner interface{ Scan(...any) error }

func scanSchedulePeriod(row schedulingScanner) (*SchedulePeriod, error) {
	var period SchedulePeriod
	err := row.Scan(&period.ID, &period.GroupID, &period.GroupName, &period.GroupType, &period.Name, &period.Timezone,
		&period.StartDate, &period.EndDate, &period.Status, &period.CreatedByUserID, &period.PublishedAt,
		&period.CreatedAt, &period.UpdatedAt, &period.ShiftCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchedulingNotFound
	}
	return &period, err
}

func loadSchedulePeriod(ctx context.Context, q schedulingQueryer, id string) (*SchedulePeriod, error) {
	return scanSchedulePeriod(q.QueryRowContext(ctx, `SELECT `+schedulePeriodColumns+` FROM schedule_periods p WHERE p.id=?`, id))
}

func (s *SchedulingService) ListPeriods(ctx context.Context, query ListQuery) (ListResult[SchedulePeriod], error) {
	query, sortExpression, err := NormalizeListQuery(query, "start_date", "desc", map[string]string{
		"name":       "p.name COLLATE NOCASE",
		"group_name": "p.group_name COLLATE NOCASE",
		"start_date": "p.start_date",
		"end_date":   "p.end_date",
		"status":     "p.status",
		"created_at": "p.created_at",
		"updated_at": "p.updated_at",
	})
	if err != nil {
		return ListResult[SchedulePeriod]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[SchedulePeriod]{}, err
	}
	defer db.Close()
	where := []string{"1=1"}
	args := []any{}
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		where = append(where, `(p.name LIKE ? COLLATE NOCASE OR p.group_name LIKE ? COLLATE NOCASE)`)
		args = append(args, pattern, pattern)
	}
	for _, filter := range []string{"group_id", "status"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			where = append(where, "p."+filter+" = ?")
			args = append(args, value)
		}
	}
	if value := strings.TrimSpace(query.Filters["date"]); value != "" {
		where = append(where, "p.start_date <= ? AND p.end_date >= ?")
		args = append(args, value, value)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedule_periods p WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[SchedulePeriod]{}, err
	}
	listArgs := append(append([]any{}, args...), query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT `+schedulePeriodColumns+` FROM schedule_periods p WHERE `+whereSQL+
		` ORDER BY `+sortExpression+` `+query.Order+`, p.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[SchedulePeriod]{}, err
	}
	defer rows.Close()
	periods := []SchedulePeriod{}
	for rows.Next() {
		period, err := scanSchedulePeriod(rows)
		if err != nil {
			return ListResult[SchedulePeriod]{}, err
		}
		periods = append(periods, *period)
	}
	if err := rows.Err(); err != nil {
		return ListResult[SchedulePeriod]{}, err
	}
	return NewListResult(periods, query, total), nil
}

func (s *SchedulingService) GetPeriod(ctx context.Context, id string) (*SchedulePeriod, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	period, err := loadSchedulePeriodDetail(ctx, db, id)
	if err != nil {
		return nil, err
	}
	s.resolveAssigneeNames(ctx, period.Shifts)
	return period, nil
}

// resolveAssigneeNames fills display names at read time; names are never
// stored in scheduling.db (Section 5.2). Missing users keep an empty name.
func (s *SchedulingService) resolveAssigneeNames(ctx context.Context, shifts []ScheduleShift) {
	names := map[string]string{}
	for i := range shifts {
		for j := range shifts[i].Assignments {
			assignment := &shifts[i].Assignments[j]
			name, ok := names[assignment.UserID]
			if !ok {
				if user, err := s.users.Get(ctx, assignment.UserID); err == nil {
					name = user.DisplayName
				}
				names[assignment.UserID] = name
			}
			assignment.DisplayName = name
		}
	}
}

// Groups lists Group System groups a scheduler may target, so schedulers do
// not need groups.read.
func (s *SchedulingService) Groups(ctx context.Context, query ListQuery) (ListResult[Group], error) {
	return s.users.ListGroups(ctx, query)
}

func loadSchedulePeriodDetail(ctx context.Context, q schedulingQueryer, id string) (*SchedulePeriod, error) {
	period, err := loadSchedulePeriod(ctx, q, id)
	if err != nil {
		return nil, err
	}
	shifts, err := loadScheduleShifts(ctx, q, `s.schedule_period_id=?`, id)
	if err != nil {
		return nil, err
	}
	period.Shifts = shifts
	return period, nil
}

func parseSchedulingDate(value, field string) (time.Time, error) {
	date, err := time.Parse(schedulingDateLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, schedulingInvalid("%s must use YYYY-MM-DD", field)
	}
	return date, nil
}

func loadSchedulingLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		return nil, schedulingInvalid("timezone must be an IANA timezone name")
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, schedulingInvalid("timezone must be an IANA timezone name")
	}
	return location, nil
}

func validateSchedulingDateRange(startValue, endValue string) error {
	start, err := parseSchedulingDate(startValue, "start_date")
	if err != nil {
		return err
	}
	end, err := parseSchedulingDate(endValue, "end_date")
	if err != nil {
		return err
	}
	if end.Before(start) {
		return schedulingInvalid("end_date must not be before start_date")
	}
	if end.Sub(start).Hours()/24 >= schedulingMaxPeriod {
		return schedulingInvalid("a schedule period may span at most %d days", schedulingMaxPeriod)
	}
	return nil
}

// resolveSchedulingGroup validates the target group against the configured
// integration mode and returns the snapshot stored on the period.
func (s *SchedulingService) resolveSchedulingGroup(ctx context.Context, config SchedulingConfig, groupID string) (*Group, error) {
	group, err := s.users.GetGroup(ctx, strings.TrimSpace(groupID))
	if errors.Is(err, ErrGroupNotFound) {
		return nil, schedulingInvalid("group not found")
	}
	if err != nil {
		return nil, err
	}
	if config.GroupMode == SchedulingGroupRequired && group.Status != "active" {
		return nil, schedulingInvalid("group must be active when group integration is required")
	}
	return group, nil
}

func (s *SchedulingService) CreatePeriod(ctx context.Context, actorUserID string, input CreateSchedulePeriodInput) (*SchedulePeriod, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, schedulingInvalid("name is required")
	}
	if _, err := loadSchedulingLocation(input.Timezone); err != nil {
		return nil, err
	}
	if err := validateSchedulingDateRange(input.StartDate, input.EndDate); err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	config, err := readSchedulingConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	group, err := s.resolveSchedulingGroup(ctx, config, input.GroupID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO schedule_periods
		(id, group_id, group_name, group_type, name, timezone, start_date, end_date, status, created_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?)`,
		id, group.ID, group.Name, group.Type, name, strings.TrimSpace(input.Timezone),
		strings.TrimSpace(input.StartDate), strings.TrimSpace(input.EndDate), actorUserID, now, now); err != nil {
		return nil, err
	}
	if err := s.recordEvent(ctx, tx, id, "", "", actorUserID, "period_created", "", ""); err != nil {
		return nil, err
	}
	period, err := loadSchedulePeriodDetail(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return period, tx.Commit()
}

func scheduleEditable(period *SchedulePeriod) error {
	if period.Status != ScheduleDraft && period.Status != SchedulePublished {
		return schedulingState("schedule period is %s", period.Status)
	}
	return nil
}

func (s *SchedulingService) UpdatePeriod(ctx context.Context, actorUserID, id string, input UpdateSchedulePeriodInput) (*SchedulePeriod, error) {
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
	period, err := loadSchedulePeriod(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := scheduleEditable(period); err != nil {
		return nil, err
	}
	name, timezone, startDate, endDate := period.Name, period.Timezone, period.StartDate, period.EndDate
	if input.Name != nil {
		if name = strings.TrimSpace(*input.Name); name == "" {
			return nil, schedulingInvalid("name is required")
		}
	}
	if input.Timezone != nil {
		timezone = strings.TrimSpace(*input.Timezone)
	}
	if input.StartDate != nil {
		startDate = strings.TrimSpace(*input.StartDate)
	}
	if input.EndDate != nil {
		endDate = strings.TrimSpace(*input.EndDate)
	}
	if timezone != period.Timezone || startDate != period.StartDate || endDate != period.EndDate {
		if period.Status != ScheduleDraft {
			return nil, schedulingState("timezone and dates can only change while the schedule is a draft")
		}
	}
	if timezone != period.Timezone {
		if _, err := loadSchedulingLocation(timezone); err != nil {
			return nil, err
		}
		if period.ShiftCount > 0 {
			return nil, schedulingState("timezone cannot change after shifts are created")
		}
	}
	if err := validateSchedulingDateRange(startDate, endDate); err != nil {
		return nil, err
	}
	var outside int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedule_shifts WHERE schedule_period_id=? AND status<>'cancelled'
		AND (substr(local_start_at,1,10) < ? OR substr(local_start_at,1,10) > ?)`, id, startDate, endDate).Scan(&outside); err != nil {
		return nil, err
	}
	if outside > 0 {
		return nil, schedulingInvalid("%d shift(s) would fall outside the new date range", outside)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_periods SET name=?, timezone=?, start_date=?, end_date=?, updated_at=? WHERE id=?`,
		name, timezone, startDate, endDate, s.timestamp(), id); err != nil {
		return nil, err
	}
	if err := s.recordEvent(ctx, tx, id, "", "", actorUserID, "period_updated", "", ""); err != nil {
		return nil, err
	}
	updated, err := loadSchedulePeriodDetail(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return updated, tx.Commit()
}

// TransitionPeriod applies one lifecycle action (Section 8). Publishing runs
// full conflict validation; blocking conflicts require an override reason
// from a caller holding scheduling.override.
func (s *SchedulingService) TransitionPeriod(ctx context.Context, actorUserID, id, action, reason string, canOverride bool) (*SchedulePeriod, []SchedulingConflict, error) {
	allowed := map[string][]string{
		"publish": {ScheduleDraft},
		"lock":    {SchedulePublished},
		"archive": {ScheduleLocked},
		"cancel":  {ScheduleDraft, SchedulePublished},
	}
	target := map[string]string{"publish": SchedulePublished, "lock": ScheduleLocked, "archive": ScheduleArchived, "cancel": ScheduleCancelled}
	from, ok := allowed[action]
	if !ok {
		return nil, nil, schedulingInvalid("unknown schedule action %q", action)
	}
	reason = strings.TrimSpace(reason)
	db, err := s.open()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	period, err := loadSchedulePeriod(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}
	if !containsString(from, period.Status) {
		return nil, nil, schedulingState("cannot %s a %s schedule period", action, period.Status)
	}
	now := s.timestamp()
	var warnings []SchedulingConflict
	switch action {
	case "publish":
		config, err := readSchedulingConfig(ctx, tx)
		if err != nil {
			return nil, nil, err
		}
		conflicts, err := s.periodConflicts(ctx, tx, period, config)
		if err != nil {
			return nil, nil, err
		}
		if blocking := blockingConflicts(conflicts); len(blocking) > 0 {
			if reason == "" || !canOverride {
				return nil, nil, &SchedulingConflictError{Conflicts: conflicts}
			}
			detail, _ := json.Marshal(blocking)
			if err := s.recordEvent(ctx, tx, id, "", "", actorUserID, "override", reason, string(detail)); err != nil {
				return nil, nil, err
			}
		}
		warnings = conflicts
		groupName, groupType := period.GroupName, period.GroupType
		if group, err := s.users.GetGroup(ctx, period.GroupID); err == nil {
			groupName, groupType = group.Name, group.Type
		} else if !errors.Is(err, ErrGroupNotFound) {
			return nil, nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schedule_periods SET status='published', published_at=?, group_name=?, group_type=?, updated_at=? WHERE id=?`,
			now, groupName, groupType, now, id); err != nil {
			return nil, nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schedule_shifts SET status='published', updated_at=? WHERE schedule_period_id=? AND status='draft'`, now, id); err != nil {
			return nil, nil, err
		}
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE schedule_periods SET status=?, updated_at=? WHERE id=?`, target[action], now, id); err != nil {
			return nil, nil, err
		}
	}
	eventType := map[string]string{"publish": "published", "lock": "locked", "archive": "archived", "cancel": "cancelled"}[action]
	if err := s.recordEvent(ctx, tx, id, "", "", actorUserID, eventType, reason, ""); err != nil {
		return nil, nil, err
	}
	updated, err := loadSchedulePeriodDetail(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}
	if warnings == nil {
		warnings = []SchedulingConflict{}
	}
	return updated, warnings, tx.Commit()
}

// ---- Shifts ----

const scheduleShiftColumns = `s.id, s.schedule_period_id, s.title, s.local_start_at, s.local_end_at, s.start_at_utc, s.end_at_utc,
	s.planned_minutes, s.required_headcount, s.assignment_mode, s.notes, s.status, s.created_at, s.updated_at`

func scanScheduleShift(row schedulingScanner, extra ...any) (*ScheduleShift, error) {
	var shift ScheduleShift
	destinations := []any{&shift.ID, &shift.SchedulePeriodID, &shift.Title, &shift.LocalStartAt, &shift.LocalEndAt,
		&shift.StartAtUTC, &shift.EndAtUTC, &shift.PlannedMinutes, &shift.RequiredHeadcount, &shift.AssignmentMode,
		&shift.Notes, &shift.Status, &shift.CreatedAt, &shift.UpdatedAt}
	err := row.Scan(append(destinations, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchedulingNotFound
	}
	shift.Assignments = []ScheduleAssignment{}
	return &shift, err
}

func activeScheduleAssignment(status string) bool {
	return status == "planned" || status == "confirmed"
}

func loadScheduleShifts(ctx context.Context, q schedulingQueryer, where string, args ...any) ([]ScheduleShift, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+scheduleShiftColumns+` FROM schedule_shifts s WHERE `+where+` ORDER BY s.start_at_utc, s.id`, args...)
	if err != nil {
		return nil, err
	}
	shifts := []ScheduleShift{}
	index := map[string]int{}
	for rows.Next() {
		shift, err := scanScheduleShift(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[shift.ID] = len(shifts)
		shifts = append(shifts, *shift)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(shifts) == 0 {
		return shifts, nil
	}
	rows, err = q.QueryContext(ctx, `SELECT a.id, a.shift_id, a.user_id, a.assignment_status, a.source, a.override_reason, a.created_at, a.updated_at
		FROM schedule_assignments a JOIN schedule_shifts s ON s.id=a.shift_id WHERE `+where+` ORDER BY a.created_at, a.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var assignment ScheduleAssignment
		if err := rows.Scan(&assignment.ID, &assignment.ShiftID, &assignment.UserID, &assignment.AssignmentStatus,
			&assignment.Source, &assignment.OverrideReason, &assignment.CreatedAt, &assignment.UpdatedAt); err != nil {
			return nil, err
		}
		shift := &shifts[index[assignment.ShiftID]]
		shift.Assignments = append(shift.Assignments, assignment)
		if activeScheduleAssignment(assignment.AssignmentStatus) {
			shift.AssignedCount++
		}
	}
	for i := range shifts {
		if shifts[i].Status != "cancelled" && shifts[i].AssignedCount < shifts[i].RequiredHeadcount {
			shifts[i].OpenSlots = shifts[i].RequiredHeadcount - shifts[i].AssignedCount
		}
	}
	return shifts, rows.Err()
}

func loadScheduleShift(ctx context.Context, q schedulingQueryer, id string) (*ScheduleShift, error) {
	shifts, err := loadScheduleShifts(ctx, q, `s.id=?`, id)
	if err != nil {
		return nil, err
	}
	if len(shifts) == 0 {
		return nil, ErrSchedulingNotFound
	}
	return &shifts[0], nil
}

type scheduleInterval struct {
	shiftID    string
	start      time.Time
	end        time.Time
	minutes    int
	overridden bool
}

// resolveShiftInterval converts local wall-clock times in the period timezone
// into UTC instants. Duration comes from the instants so DST transitions
// produce the real elapsed time (Section 4.2).
func resolveShiftInterval(period *SchedulePeriod, localStart, localEnd string) (scheduleInterval, string, string, error) {
	location, err := loadSchedulingLocation(period.Timezone)
	if err != nil {
		return scheduleInterval{}, "", "", err
	}
	start, err := time.ParseInLocation(schedulingLocalLayout, strings.TrimSpace(localStart), location)
	if err != nil {
		return scheduleInterval{}, "", "", schedulingInvalid("local_start_at must use YYYY-MM-DDTHH:MM")
	}
	end, err := time.ParseInLocation(schedulingLocalLayout, strings.TrimSpace(localEnd), location)
	if err != nil {
		return scheduleInterval{}, "", "", schedulingInvalid("local_end_at must use YYYY-MM-DDTHH:MM")
	}
	if !end.After(start) {
		return scheduleInterval{}, "", "", schedulingInvalid("shift end must be after its start")
	}
	minutes := int(end.Sub(start).Minutes())
	if minutes > schedulingMaxShift {
		return scheduleInterval{}, "", "", schedulingInvalid("a shift may last at most 24 hours")
	}
	startDate := start.Format(schedulingDateLayout)
	if startDate < period.StartDate || startDate > period.EndDate {
		return scheduleInterval{}, "", "", schedulingInvalid("shift must start within the schedule period")
	}
	return scheduleInterval{start: start.UTC(), end: end.UTC(), minutes: minutes},
		start.Format(schedulingLocalLayout), end.Format(schedulingLocalLayout), nil
}

func validShiftAssignmentMode(mode string) bool {
	return mode == "named_users" || mode == "open_shift" || mode == "requirement_only"
}

func (s *SchedulingService) CreateShift(ctx context.Context, actorUserID, periodID string, input CreateScheduleShiftInput) (*ScheduleShift, error) {
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
	period, err := loadSchedulePeriod(ctx, tx, periodID)
	if err != nil {
		return nil, err
	}
	if err := scheduleEditable(period); err != nil {
		return nil, err
	}
	interval, localStart, localEnd, err := resolveShiftInterval(period, input.LocalStartAt, input.LocalEndAt)
	if err != nil {
		return nil, err
	}
	headcount := input.RequiredHeadcount
	if headcount == 0 {
		headcount = 1
	}
	if headcount < 1 || headcount > 1000 {
		return nil, schedulingInvalid("required_headcount must be between 1 and 1000")
	}
	mode := strings.TrimSpace(input.AssignmentMode)
	if mode == "" {
		mode = "named_users"
	}
	if !validShiftAssignmentMode(mode) {
		return nil, schedulingInvalid("assignment_mode must be named_users, open_shift, or requirement_only")
	}
	status := ScheduleDraft
	if period.Status == SchedulePublished {
		status = SchedulePublished
	}
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO schedule_shifts
		(id, schedule_period_id, title, local_start_at, local_end_at, start_at_utc, end_at_utc, planned_minutes,
		required_headcount, assignment_mode, notes, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, periodID, nullableTrim(optionalString(input.Title)), localStart, localEnd,
		interval.start.Format(time.RFC3339), interval.end.Format(time.RFC3339), interval.minutes,
		headcount, mode, nullableTrim(optionalString(input.Notes)), status, now, now); err != nil {
		return nil, err
	}
	if err := s.touchPeriod(ctx, tx, periodID); err != nil {
		return nil, err
	}
	if err := s.recordEvent(ctx, tx, periodID, id, "", actorUserID, "shift_created", "", ""); err != nil {
		return nil, err
	}
	shift, err := loadScheduleShift(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	return shift, tx.Commit()
}

func (s *SchedulingService) touchPeriod(ctx context.Context, q schedulingQueryer, periodID string) error {
	_, err := q.ExecContext(ctx, `UPDATE schedule_periods SET updated_at=? WHERE id=?`, s.timestamp(), periodID)
	return err
}

func (s *SchedulingService) loadEditableShift(ctx context.Context, q schedulingQueryer, shiftID string) (*ScheduleShift, *SchedulePeriod, error) {
	shift, err := loadScheduleShift(ctx, q, shiftID)
	if err != nil {
		return nil, nil, err
	}
	period, err := loadSchedulePeriod(ctx, q, shift.SchedulePeriodID)
	if err != nil {
		return nil, nil, err
	}
	if err := scheduleEditable(period); err != nil {
		return nil, nil, err
	}
	if shift.Status == "cancelled" || shift.Status == "completed" {
		return nil, nil, schedulingState("shift is %s", shift.Status)
	}
	return shift, period, nil
}

func (s *SchedulingService) UpdateShift(ctx context.Context, actorUserID, shiftID string, input UpdateScheduleShiftInput, canOverride bool) (*ScheduleShift, []SchedulingConflict, error) {
	db, err := s.open()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	shift, period, err := s.loadEditableShift(ctx, tx, shiftID)
	if err != nil {
		return nil, nil, err
	}
	localStart, localEnd := shift.LocalStartAt, shift.LocalEndAt
	if input.LocalStartAt != nil {
		localStart = *input.LocalStartAt
	}
	if input.LocalEndAt != nil {
		localEnd = *input.LocalEndAt
	}
	interval, localStart, localEnd, err := resolveShiftInterval(period, localStart, localEnd)
	if err != nil {
		return nil, nil, err
	}
	interval.shiftID = shift.ID
	headcount := shift.RequiredHeadcount
	if input.RequiredHeadcount != nil {
		headcount = *input.RequiredHeadcount
		if headcount < 1 || headcount > 1000 {
			return nil, nil, schedulingInvalid("required_headcount must be between 1 and 1000")
		}
		if headcount < shift.AssignedCount {
			return nil, nil, schedulingState("required_headcount cannot be below the %d active assignment(s)", shift.AssignedCount)
		}
	}
	mode := shift.AssignmentMode
	if input.AssignmentMode != nil {
		if mode = strings.TrimSpace(*input.AssignmentMode); !validShiftAssignmentMode(mode) {
			return nil, nil, schedulingInvalid("assignment_mode must be named_users, open_shift, or requirement_only")
		}
	}
	title, notes := shift.Title, shift.Notes
	if input.Title != nil {
		title = input.Title
	}
	if input.Notes != nil {
		notes = input.Notes
	}
	warnings := []SchedulingConflict{}
	timeChanged := localStart != shift.LocalStartAt || localEnd != shift.LocalEndAt
	if timeChanged {
		config, err := readSchedulingConfig(ctx, tx)
		if err != nil {
			return nil, nil, err
		}
		for _, assignment := range shift.Assignments {
			if !activeScheduleAssignment(assignment.AssignmentStatus) {
				continue
			}
			conflicts, err := userIntervalConflicts(ctx, tx, assignment.UserID, interval, config, period.Timezone)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, conflicts...)
		}
	}
	reason := strings.TrimSpace(input.OverrideReason)
	if blocking := blockingConflicts(warnings); len(blocking) > 0 {
		if reason == "" || !canOverride {
			return nil, nil, &SchedulingConflictError{Conflicts: warnings}
		}
		detail, _ := json.Marshal(blocking)
		if err := s.recordEvent(ctx, tx, period.ID, shift.ID, "", actorUserID, "override", reason, string(detail)); err != nil {
			return nil, nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_shifts SET title=?, local_start_at=?, local_end_at=?, start_at_utc=?, end_at_utc=?,
		planned_minutes=?, required_headcount=?, assignment_mode=?, notes=?, updated_at=? WHERE id=?`,
		nullableTrim(optionalString(title)), localStart, localEnd, interval.start.Format(time.RFC3339), interval.end.Format(time.RFC3339),
		interval.minutes, headcount, mode, nullableTrim(optionalString(notes)), s.timestamp(), shift.ID); err != nil {
		return nil, nil, err
	}
	if err := s.touchPeriod(ctx, tx, period.ID); err != nil {
		return nil, nil, err
	}
	detail := ""
	if timeChanged {
		detail = fmt.Sprintf("time changed from %s–%s to %s–%s", shift.LocalStartAt, shift.LocalEndAt, localStart, localEnd)
	}
	if err := s.recordEvent(ctx, tx, period.ID, shift.ID, "", actorUserID, "shift_updated", reason, detail); err != nil {
		return nil, nil, err
	}
	updated, err := loadScheduleShift(ctx, tx, shift.ID)
	if err != nil {
		return nil, nil, err
	}
	return updated, warnings, tx.Commit()
}

func (s *SchedulingService) CancelShift(ctx context.Context, actorUserID, shiftID, reason string) (*ScheduleShift, error) {
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
	shift, period, err := s.loadEditableShift(ctx, tx, shiftID)
	if err != nil {
		return nil, err
	}
	now := s.timestamp()
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_shifts SET status='cancelled', updated_at=? WHERE id=?`, now, shift.ID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_assignments SET assignment_status='cancelled', updated_at=?
		WHERE shift_id=? AND assignment_status IN ('planned','confirmed')`, now, shift.ID); err != nil {
		return nil, err
	}
	if err := s.touchPeriod(ctx, tx, period.ID); err != nil {
		return nil, err
	}
	if err := s.recordEvent(ctx, tx, period.ID, shift.ID, "", actorUserID, "shift_cancelled", reason, ""); err != nil {
		return nil, err
	}
	updated, err := loadScheduleShift(ctx, tx, shift.ID)
	if err != nil {
		return nil, err
	}
	return updated, tx.Commit()
}

// ---- Assignments ----

func (s *SchedulingService) AssignUser(ctx context.Context, actorUserID, shiftID string, input CreateScheduleAssignmentInput, canOverride bool) (*ScheduleAssignment, []SchedulingConflict, error) {
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		return nil, nil, schedulingInvalid("user_id is required")
	}
	db, err := s.open()
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	shift, period, err := s.loadEditableShift(ctx, tx, shiftID)
	if err != nil {
		return nil, nil, err
	}
	for _, assignment := range shift.Assignments {
		if assignment.UserID == userID && activeScheduleAssignment(assignment.AssignmentStatus) {
			return nil, nil, schedulingState("user is already assigned to this shift")
		}
	}
	if shift.AssignedCount >= shift.RequiredHeadcount {
		return nil, nil, schedulingState("shift is fully staffed")
	}
	config, err := readSchedulingConfig(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	checker := newSchedulingUserChecker(s.users, config, period.GroupID)
	conflicts, err := checker.check(ctx, userID, shift.ID, false)
	if errors.Is(err, ErrUserNotFound) {
		return nil, nil, schedulingInvalid("user not found")
	}
	if err != nil {
		return nil, nil, err
	}
	interval, err := shiftInterval(shift)
	if err != nil {
		return nil, nil, err
	}
	overlaps, err := userIntervalConflicts(ctx, tx, userID, interval, config, period.Timezone)
	if err != nil {
		return nil, nil, err
	}
	conflicts = append(conflicts, overlaps...)
	reason := strings.TrimSpace(input.OverrideReason)
	blocking := blockingConflicts(conflicts)
	if len(blocking) > 0 && (reason == "" || !canOverride) {
		return nil, nil, &SchedulingConflictError{Conflicts: conflicts}
	}
	if len(blocking) == 0 {
		reason = ""
	}
	id, now := uuid.NewString(), s.timestamp()
	if _, err := tx.ExecContext(ctx, `INSERT INTO schedule_assignments
		(id, shift_id, user_id, assignment_status, source, override_reason, created_at, updated_at)
		VALUES (?, ?, ?, 'planned', 'manual', ?, ?, ?)`, id, shift.ID, userID, nullableTrim(reason), now, now); err != nil {
		return nil, nil, err
	}
	if len(blocking) > 0 {
		detail, _ := json.Marshal(blocking)
		if err := s.recordEvent(ctx, tx, period.ID, shift.ID, id, actorUserID, "override", reason, string(detail)); err != nil {
			return nil, nil, err
		}
	}
	if err := s.touchPeriod(ctx, tx, period.ID); err != nil {
		return nil, nil, err
	}
	if err := s.recordEvent(ctx, tx, period.ID, shift.ID, id, actorUserID, "assignment_created", "", "user "+userID); err != nil {
		return nil, nil, err
	}
	assignment, err := loadScheduleAssignment(ctx, tx, id)
	if err != nil {
		return nil, nil, err
	}
	return assignment, conflicts, tx.Commit()
}

func loadScheduleAssignment(ctx context.Context, q schedulingQueryer, id string) (*ScheduleAssignment, error) {
	var assignment ScheduleAssignment
	err := q.QueryRowContext(ctx, `SELECT id, shift_id, user_id, assignment_status, source, override_reason, created_at, updated_at
		FROM schedule_assignments WHERE id=?`, id).Scan(&assignment.ID, &assignment.ShiftID, &assignment.UserID,
		&assignment.AssignmentStatus, &assignment.Source, &assignment.OverrideReason, &assignment.CreatedAt, &assignment.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSchedulingNotFound
	}
	return &assignment, err
}

func (s *SchedulingService) CancelAssignment(ctx context.Context, actorUserID, assignmentID string) error {
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
	assignment, err := loadScheduleAssignment(ctx, tx, assignmentID)
	if err != nil {
		return err
	}
	_, period, err := s.loadEditableShift(ctx, tx, assignment.ShiftID)
	if err != nil {
		return err
	}
	if !activeScheduleAssignment(assignment.AssignmentStatus) {
		return schedulingState("assignment is %s", assignment.AssignmentStatus)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_assignments SET assignment_status='cancelled', updated_at=? WHERE id=?`,
		s.timestamp(), assignment.ID); err != nil {
		return err
	}
	if err := s.touchPeriod(ctx, tx, period.ID); err != nil {
		return err
	}
	if err := s.recordEvent(ctx, tx, period.ID, assignment.ShiftID, assignment.ID, actorUserID, "assignment_cancelled", "", "user "+assignment.UserID); err != nil {
		return err
	}
	return tx.Commit()
}

// RespondToAssignment lets the assigned user confirm or decline a published
// shift. Other users' assignments are reported as not found.
func (s *SchedulingService) RespondToAssignment(ctx context.Context, userID, assignmentID, decision, note string) (*ScheduleAssignment, error) {
	if decision != "confirmed" && decision != "declined" {
		return nil, schedulingInvalid("assignment_status must be confirmed or declined")
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
	assignment, err := loadScheduleAssignment(ctx, tx, assignmentID)
	if err != nil {
		return nil, err
	}
	if assignment.UserID != userID {
		return nil, ErrSchedulingNotFound
	}
	shift, err := loadScheduleShift(ctx, tx, assignment.ShiftID)
	if err != nil {
		return nil, err
	}
	period, err := loadSchedulePeriod(ctx, tx, shift.SchedulePeriodID)
	if err != nil {
		return nil, err
	}
	if period.Status != SchedulePublished || shift.Status != SchedulePublished {
		return nil, schedulingState("only shifts in a published schedule can be confirmed or declined")
	}
	if !activeScheduleAssignment(assignment.AssignmentStatus) {
		return nil, schedulingState("assignment is %s", assignment.AssignmentStatus)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE schedule_assignments SET assignment_status=?, updated_at=? WHERE id=?`,
		decision, s.timestamp(), assignment.ID); err != nil {
		return nil, err
	}
	if err := s.recordEvent(ctx, tx, period.ID, shift.ID, assignment.ID, userID, "assignment_"+decision, note, ""); err != nil {
		return nil, err
	}
	updated, err := loadScheduleAssignment(ctx, tx, assignment.ID)
	if err != nil {
		return nil, err
	}
	return updated, tx.Commit()
}

// ---- Self-service and lookup ----

func (s *SchedulingService) MyShifts(ctx context.Context, userID, fromValue, toValue string) ([]MyScheduledShift, error) {
	from, err := parseSchedulingDate(fromValue, "from")
	if err != nil {
		return nil, err
	}
	to, err := parseSchedulingDate(toValue, "to")
	if err != nil {
		return nil, err
	}
	if to.Before(from) || to.Sub(from).Hours()/24 >= schedulingMaxSelfRange {
		return nil, schedulingInvalid("the date range must be ordered and span at most %d days", schedulingMaxSelfRange)
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT `+scheduleShiftColumns+`, p.name, p.status, p.group_id, p.group_name, p.timezone, a.id, a.assignment_status
		FROM schedule_assignments a
		JOIN schedule_shifts s ON s.id=a.shift_id
		JOIN schedule_periods p ON p.id=s.schedule_period_id
		WHERE a.user_id=? AND a.assignment_status IN ('planned','confirmed','completed')
			AND s.status<>'cancelled' AND p.status IN ('published','locked','archived')
			AND substr(s.local_start_at,1,10) BETWEEN ? AND ?
		ORDER BY s.start_at_utc, s.id`, userID, from.Format(schedulingDateLayout), to.Format(schedulingDateLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shifts := []MyScheduledShift{}
	for rows.Next() {
		var item MyScheduledShift
		shift, err := scanScheduleShift(rows, &item.PeriodName, &item.PeriodStatus, &item.GroupID, &item.GroupName, &item.Timezone, &item.AssignmentID, &item.AssignmentStatus)
		if err != nil {
			return nil, err
		}
		item.ScheduleShift = *shift
		shifts = append(shifts, item)
	}
	return shifts, rows.Err()
}

func (s *SchedulingService) Candidates(ctx context.Context, shiftID string) ([]ScheduleCandidate, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	shift, err := loadScheduleShift(ctx, db, shiftID)
	if err != nil {
		return nil, err
	}
	period, err := loadSchedulePeriod(ctx, db, shift.SchedulePeriodID)
	if err != nil {
		return nil, err
	}
	config, err := readSchedulingConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	members, err := s.users.ActiveGroupMembers(ctx, period.GroupID)
	if err != nil {
		return nil, err
	}
	interval, err := shiftInterval(shift)
	if err != nil {
		return nil, err
	}
	assigned := map[string]bool{}
	for _, assignment := range shift.Assignments {
		if activeScheduleAssignment(assignment.AssignmentStatus) {
			assigned[assignment.UserID] = true
		}
	}
	candidates := make([]ScheduleCandidate, 0, len(members))
	for _, member := range members {
		candidate := ScheduleCandidate{UserID: member.UserID, DisplayName: member.DisplayName, Email: member.Email,
			Title: member.Title, AlreadyAssigned: assigned[member.UserID], Conflicts: []SchedulingConflict{}}
		if !candidate.AlreadyAssigned {
			conflicts, err := userIntervalConflicts(ctx, db, member.UserID, interval, config, period.Timezone)
			if err != nil {
				return nil, err
			}
			candidate.Conflicts = conflicts
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func (s *SchedulingService) Conflicts(ctx context.Context, periodID string) ([]SchedulingConflict, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	period, err := loadSchedulePeriod(ctx, db, periodID)
	if err != nil {
		return nil, err
	}
	config, err := readSchedulingConfig(ctx, db)
	if err != nil {
		return nil, err
	}
	return s.periodConflicts(ctx, db, period, config)
}

// ---- Validation (Section 7) ----

func blockingConflicts(conflicts []SchedulingConflict) []SchedulingConflict {
	var blocking []SchedulingConflict
	for _, conflict := range conflicts {
		if conflict.Severity == "error" {
			blocking = append(blocking, conflict)
		}
	}
	return blocking
}

func shiftInterval(shift *ScheduleShift) (scheduleInterval, error) {
	start, err := time.Parse(time.RFC3339, shift.StartAtUTC)
	if err != nil {
		return scheduleInterval{}, err
	}
	end, err := time.Parse(time.RFC3339, shift.EndAtUTC)
	if err != nil {
		return scheduleInterval{}, err
	}
	return scheduleInterval{shiftID: shift.ID, start: start, end: end, minutes: shift.PlannedMinutes}, nil
}

// userIntervalConflicts compares one candidate shift with every other active
// assignment the user holds across all non-cancelled schedules.
func userIntervalConflicts(ctx context.Context, q schedulingQueryer, userID string, candidate scheduleInterval, config SchedulingConfig, timezone string) ([]SchedulingConflict, error) {
	window := 8 * 24 * time.Hour
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.start_at_utc, s.end_at_utc, s.planned_minutes, a.override_reason IS NOT NULL
		FROM schedule_assignments a
		JOIN schedule_shifts s ON s.id=a.shift_id
		JOIN schedule_periods p ON p.id=s.schedule_period_id
		WHERE a.user_id=? AND a.assignment_status IN ('planned','confirmed','completed')
			AND s.status<>'cancelled' AND p.status<>'cancelled' AND s.id<>?
			AND s.end_at_utc > ? AND s.start_at_utc < ?`,
		userID, candidate.shiftID, candidate.start.Add(-window).Format(time.RFC3339), candidate.end.Add(window).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	var others []scheduleInterval
	for rows.Next() {
		var other scheduleInterval
		var start, end string
		if err := rows.Scan(&other.shiftID, &start, &end, &other.minutes, &other.overridden); err != nil {
			rows.Close()
			return nil, err
		}
		if other.start, err = time.Parse(time.RFC3339, start); err != nil {
			rows.Close()
			return nil, err
		}
		if other.end, err = time.Parse(time.RFC3339, end); err != nil {
			rows.Close()
			return nil, err
		}
		others = append(others, other)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	conflicts := []SchedulingConflict{}
	rest := time.Duration(config.MinRestMinutes) * time.Minute
	for _, other := range others {
		if candidate.start.Before(other.end) && candidate.end.After(other.start) {
			conflicts = append(conflicts, SchedulingConflict{Code: "user_overlap", Severity: "error", UserID: userID,
				ShiftID: candidate.shiftID, RelatedShiftID: other.shiftID, Message: "user is already assigned to an overlapping shift",
				relatedOverridden: other.overridden})
			continue
		}
		if rest > 0 {
			gap := candidate.start.Sub(other.end)
			if other.start.After(candidate.start) {
				gap = other.start.Sub(candidate.end)
			}
			if gap < rest {
				conflicts = append(conflicts, SchedulingConflict{Code: "rest_time", Severity: "warning", UserID: userID,
					ShiftID: candidate.shiftID, RelatedShiftID: other.shiftID,
					Message: fmt.Sprintf("only %d minute(s) of rest between shifts; policy requires %d", int(gap.Minutes()), config.MinRestMinutes)})
			}
		}
	}
	if config.MaxWeeklyMinutes > 0 {
		location, err := loadSchedulingLocation(timezone)
		if err != nil {
			return nil, err
		}
		weekStart := scheduleWeekStart(candidate.start, location)
		total := candidate.minutes
		for _, other := range others {
			if scheduleWeekStart(other.start, location).Equal(weekStart) {
				total += other.minutes
			}
		}
		if total > config.MaxWeeklyMinutes {
			conflicts = append(conflicts, SchedulingConflict{Code: "max_weekly_hours", Severity: "warning", UserID: userID,
				ShiftID: candidate.shiftID, Week: weekStart.Format(schedulingDateLayout),
				Message: fmt.Sprintf("%d planned minute(s) in the week exceed the %d minute limit", total, config.MaxWeeklyMinutes)})
		}
	}
	return conflicts, nil
}

// scheduleWeekStart returns local Monday 00:00 of the week containing instant.
func scheduleWeekStart(instant time.Time, location *time.Location) time.Time {
	local := instant.In(location)
	offset := (int(local.Weekday()) + 6) % 7
	day := local.AddDate(0, 0, -offset)
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
}

// schedulingUserChecker caches user and membership lookups while validating
// many assignments.
type schedulingUserChecker struct {
	users   *UserService
	config  SchedulingConfig
	groupID string
	cache   map[string]*User
	members map[string]bool
}

func newSchedulingUserChecker(users *UserService, config SchedulingConfig, groupID string) *schedulingUserChecker {
	return &schedulingUserChecker{users: users, config: config, groupID: groupID, cache: map[string]*User{}}
}

// check reports user-level conflicts. Errors on an assignment that was
// already accepted with an override are downgraded to warnings.
func (c *schedulingUserChecker) check(ctx context.Context, userID, shiftID string, overridden bool) ([]SchedulingConflict, error) {
	user, ok := c.cache[userID]
	if !ok {
		var err error
		if user, err = c.users.Get(ctx, userID); err != nil {
			return nil, err
		}
		c.cache[userID] = user
	}
	if c.members == nil {
		members, err := c.users.ActiveGroupMembers(ctx, c.groupID)
		if err != nil {
			return nil, err
		}
		c.members = map[string]bool{}
		for _, member := range members {
			c.members[member.UserID] = true
		}
	}
	errorSeverity := "error"
	if overridden {
		errorSeverity = "warning"
	}
	conflicts := []SchedulingConflict{}
	if user.Status != "active" || user.DeletedAt != nil {
		conflicts = append(conflicts, SchedulingConflict{Code: "user_inactive", Severity: errorSeverity, UserID: userID,
			ShiftID: shiftID, Message: "user is not active"})
	}
	if !c.members[userID] {
		severity := "warning"
		if c.config.GroupMode == SchedulingGroupRequired {
			severity = errorSeverity
		}
		conflicts = append(conflicts, SchedulingConflict{Code: "not_group_member", Severity: severity, UserID: userID,
			ShiftID: shiftID, Message: "user is not an active member of the scheduled group"})
	}
	return conflicts, nil
}

func (s *SchedulingService) periodConflicts(ctx context.Context, q schedulingQueryer, period *SchedulePeriod, config SchedulingConfig) ([]SchedulingConflict, error) {
	conflicts := []SchedulingConflict{}
	group, err := s.users.GetGroup(ctx, period.GroupID)
	switch {
	case errors.Is(err, ErrGroupNotFound):
		severity := "warning"
		if config.GroupMode == SchedulingGroupRequired {
			severity = "error"
		}
		conflicts = append(conflicts, SchedulingConflict{Code: "group_missing", Severity: severity, Message: "the scheduled group no longer exists"})
	case err != nil:
		return nil, err
	case group.Status != "active" && config.GroupMode == SchedulingGroupRequired:
		conflicts = append(conflicts, SchedulingConflict{Code: "group_inactive", Severity: "error", Message: "the scheduled group is inactive"})
	}
	shifts, err := loadScheduleShifts(ctx, q, `s.schedule_period_id=? AND s.status<>'cancelled'`, period.ID)
	if err != nil {
		return nil, err
	}
	checker := newSchedulingUserChecker(s.users, config, period.GroupID)
	seen := map[string]bool{}
	add := func(conflict SchedulingConflict) {
		first, second := conflict.ShiftID, conflict.RelatedShiftID
		if second != "" && second < first {
			first, second = second, first
		}
		key := conflict.Code + "|" + conflict.UserID + "|" + first + "|" + second
		if conflict.Week != "" {
			key = conflict.Code + "|" + conflict.UserID + "|" + conflict.Week
		}
		if !seen[key] {
			seen[key] = true
			conflicts = append(conflicts, conflict)
		}
	}
	for i := range shifts {
		shift := &shifts[i]
		if shift.OpenSlots > 0 {
			switch shift.AssignmentMode {
			case "named_users":
				severity := "error"
				if config.AllowOpenShifts {
					severity = "warning"
				}
				add(SchedulingConflict{Code: "understaffed", Severity: severity, ShiftID: shift.ID,
					Message: fmt.Sprintf("%d of %d required people assigned", shift.AssignedCount, shift.RequiredHeadcount)})
			case "open_shift":
				add(SchedulingConflict{Code: "open_slots", Severity: "warning", ShiftID: shift.ID,
					Message: fmt.Sprintf("%d open slot(s) remain", shift.OpenSlots)})
			}
		}
		interval, err := shiftInterval(shift)
		if err != nil {
			return nil, err
		}
		for _, assignment := range shift.Assignments {
			if !activeScheduleAssignment(assignment.AssignmentStatus) {
				continue
			}
			overridden := assignment.OverrideReason != nil
			userConflicts, err := checker.check(ctx, assignment.UserID, shift.ID, overridden)
			if errors.Is(err, ErrUserNotFound) {
				add(SchedulingConflict{Code: "user_missing", Severity: "error", UserID: assignment.UserID, ShiftID: shift.ID,
					Message: "assigned user no longer exists"})
				continue
			}
			if err != nil {
				return nil, err
			}
			intervalConflicts, err := userIntervalConflicts(ctx, q, assignment.UserID, interval, config, period.Timezone)
			if err != nil {
				return nil, err
			}
			for _, conflict := range append(userConflicts, intervalConflicts...) {
				if (overridden || conflict.relatedOverridden) && conflict.Severity == "error" {
					conflict.Severity = "warning"
				}
				add(conflict)
			}
		}
	}
	sort.SliceStable(conflicts, func(i, j int) bool {
		return conflicts[i].Severity == "error" && conflicts[j].Severity != "error"
	})
	return conflicts, nil
}
