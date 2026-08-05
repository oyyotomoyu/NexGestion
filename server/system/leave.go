package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	LeaveDurationHourly   = "hourly"
	LeaveDurationFullDay  = "full_day"
	fullWorkingDayMinutes = 8 * 60
)

var (
	ErrInvalidLeaveRequest = errors.New("invalid leave request")
	ErrLeaveDecision       = errors.New("leave request cannot be decided")
)

type LeaveType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type leaveTypeCatalog struct {
	LeaveTypes []LeaveType `json:"leave_types"`
}

type LeaveRequest struct {
	ID               string  `json:"id"`
	UserID           string  `json:"user_id"`
	LeaveType        string  `json:"leave_type"`
	LeaveDate        string  `json:"leave_date"`
	DurationType     string  `json:"duration_type"`
	StartTime        *string `json:"start_time"`
	EndTime          *string `json:"end_time"`
	RequestedMinutes int     `json:"requested_minutes"`
	Reason           string  `json:"reason"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type ApplyLeaveInput struct {
	LeaveType    string  `json:"leave_type"`
	LeaveDate    string  `json:"leave_date"`
	DurationType string  `json:"duration_type"`
	StartTime    *string `json:"start_time"`
	EndTime      *string `json:"end_time"`
	Reason       string  `json:"reason"`
}

type LeaveApprovalRequest struct {
	LeaveRequest
	RequesterName       string  `json:"requester_name"`
	OrganizationGroupID *string `json:"organization_group_id"`
	AdministratorRoute  bool    `json:"administrator_route"`
}

type DecideLeaveInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

func LoadLeaveTypes() ([]LeaveType, error) {
	path, err := leaveTypesPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read leave type catalog: %w", err)
	}
	var catalog leaveTypeCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("decode leave type catalog: %w", err)
	}
	seen := make(map[string]struct{}, len(catalog.LeaveTypes))
	for index := range catalog.LeaveTypes {
		item := &catalog.LeaveTypes[index]
		item.Key, item.Label = strings.TrimSpace(item.Key), strings.TrimSpace(item.Label)
		if item.Key == "" || item.Label == "" {
			return nil, fmt.Errorf("leave type entry %d requires key and label", index)
		}
		if _, exists := seen[item.Key]; exists {
			return nil, fmt.Errorf("duplicate leave type %q", item.Key)
		}
		seen[item.Key] = struct{}{}
	}
	if len(catalog.LeaveTypes) == 0 {
		return nil, errors.New("leave type catalog must not be empty")
	}
	return catalog.LeaveTypes, nil
}

func leaveTypesPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("NEXGESTION_LEAVE_TYPES_CONFIG")); configured != "" {
		return configured, nil
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	for _, candidate := range []string{
		filepath.Join("config", "leave-types.json"),
		filepath.Join("..", "config", "leave-types.json"),
		filepath.Join(filepath.Dir(sourceFile), "..", "..", "config", "leave-types.json"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("config/leave-types.json not found")
}

func (s *AttendanceService) ApplyLeave(ctx context.Context, userID string, input ApplyLeaveInput) (*LeaveRequest, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	types, err := LoadLeaveTypes()
	if err != nil {
		return nil, err
	}
	input.LeaveType = strings.TrimSpace(input.LeaveType)
	input.LeaveDate = strings.TrimSpace(input.LeaveDate)
	input.DurationType = strings.TrimSpace(input.DurationType)
	input.Reason = strings.TrimSpace(input.Reason)
	validType := false
	for _, leaveType := range types {
		if leaveType.Key == input.LeaveType {
			validType = true
			break
		}
	}
	if !validType {
		return nil, fmt.Errorf("%w: unknown leave type", ErrInvalidLeaveRequest)
	}
	if _, err := time.Parse("2006-01-02", input.LeaveDate); err != nil {
		return nil, fmt.Errorf("%w: leave_date must use YYYY-MM-DD", ErrInvalidLeaveRequest)
	}

	minutes := fullWorkingDayMinutes
	var startTime, endTime any
	switch input.DurationType {
	case LeaveDurationFullDay:
		input.StartTime, input.EndTime = nil, nil
	case LeaveDurationHourly:
		if input.StartTime == nil || input.EndTime == nil {
			return nil, fmt.Errorf("%w: hourly leave requires start_time and end_time", ErrInvalidLeaveRequest)
		}
		start, startErr := time.Parse("15:04", strings.TrimSpace(*input.StartTime))
		end, endErr := time.Parse("15:04", strings.TrimSpace(*input.EndTime))
		if startErr != nil || endErr != nil || !end.After(start) {
			return nil, fmt.Errorf("%w: hourly leave requires a valid time range", ErrInvalidLeaveRequest)
		}
		minutes = int(end.Sub(start) / time.Minute)
		if minutes < 60 {
			return nil, fmt.Errorf("%w: hourly leave must be at least one hour", ErrInvalidLeaveRequest)
		}
		normalizedStart, normalizedEnd := start.Format("15:04"), end.Format("15:04")
		input.StartTime, input.EndTime = &normalizedStart, &normalizedEnd
		startTime, endTime = normalizedStart, normalizedEnd
	default:
		return nil, fmt.Errorf("%w: duration_type must be hourly or full_day", ErrInvalidLeaveRequest)
	}

	route, err := s.users.ResolveLeaveApproval(ctx, userID)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := formatAttendanceTime(attendanceMinute(s.now()))
	id := uuid.NewString()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var overlaps int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM leave_requests
		WHERE user_id=? AND leave_date=? AND status IN ('pending','approved') AND (
			duration_type='full_day' OR ?='full_day' OR
			(start_time<? AND end_time>?)
		)`, userID, input.LeaveDate, input.DurationType, endTime, startTime).Scan(&overlaps); err != nil {
		return nil, err
	}
	if overlaps > 0 {
		return nil, fmt.Errorf("%w: leave request overlaps an existing pending or approved request", ErrInvalidLeaveRequest)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO leave_requests
		(id,user_id,leave_type,leave_date,duration_type,start_time,end_time,requested_minutes,reason,status,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?)`,
		id, userID, input.LeaveType, input.LeaveDate, input.DurationType, startTime, endTime, minutes, input.Reason, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("%w: an identical leave request already exists", ErrInvalidLeaveRequest)
		}
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO leave_request_events
		(id,leave_request_id,actor_user_id,event_type,note,occurred_at)
		VALUES(?,?,?,'submitted','',?)`, uuid.NewString(), id, userID, now); err != nil {
		return nil, err
	}
	for _, approverID := range route.ApproverUserIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO leave_request_assignments
			(leave_request_id,approver_user_id,organization_group_id,is_administrator_route,assigned_at)
			VALUES(?,?,?,?,?)`, id, approverID, route.GroupID, route.AdministratorRoute, now); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO leave_request_events
		(id,leave_request_id,actor_user_id,event_type,note,occurred_at)
		VALUES(?,?,?,'assigned',?,?)`, uuid.NewString(), id, userID,
		leaveAssignmentNote(route), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getLeaveRequest(ctx, db, id)
}

func leaveAssignmentNote(route *LeaveApprovalRoute) string {
	if route.RequiresAssignment {
		return "requires administrator assignment"
	}
	if route.AdministratorRoute {
		return "assigned to administrator"
	}
	if route.GroupID != nil {
		return "assigned through organization group " + *route.GroupID
	}
	return "assigned"
}

func (s *AttendanceService) ListLeaveRequests(ctx context.Context, userID string, query ListQuery) (ListResult[LeaveRequest], error) {
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{
		"leave_date":        "leave_date",
		"created_at":        "created_at",
		"updated_at":        "updated_at",
		"status":            "status",
		"leave_type":        "leave_type",
		"requested_minutes": "requested_minutes",
	})
	if err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	if _, err := s.users.Get(ctx, userID); err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	defer db.Close()
	where := []string{"user_id=?"}
	args := []any{userID}
	applyLeaveListFilters(&where, &args, query)
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM leave_requests WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id,user_id,leave_type,leave_date,duration_type,start_time,end_time,
		requested_minutes,reason,status,created_at,updated_at
		FROM leave_requests WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	defer rows.Close()
	requests := make([]LeaveRequest, 0)
	for rows.Next() {
		var request LeaveRequest
		if err := scanLeaveRequest(rows, &request); err != nil {
			return ListResult[LeaveRequest]{}, err
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return ListResult[LeaveRequest]{}, err
	}
	return NewListResult(requests, query, total), nil
}

func (s *AttendanceService) getLeaveRequest(ctx context.Context, db *sql.DB, id string) (*LeaveRequest, error) {
	var request LeaveRequest
	err := scanLeaveRequest(db.QueryRowContext(ctx, `SELECT id,user_id,leave_type,leave_date,duration_type,start_time,end_time,
		requested_minutes,reason,status,created_at,updated_at FROM leave_requests WHERE id=?`, id), &request)
	return &request, err
}

type leaveScanner interface{ Scan(...any) error }

func scanLeaveRequest(scanner leaveScanner, request *LeaveRequest) error {
	return scanner.Scan(&request.ID, &request.UserID, &request.LeaveType, &request.LeaveDate,
		&request.DurationType, &request.StartTime, &request.EndTime, &request.RequestedMinutes,
		&request.Reason, &request.Status, &request.CreatedAt, &request.UpdatedAt)
}

func (s *AttendanceService) ListLeaveApprovals(ctx context.Context, approverID string, query ListQuery) (ListResult[LeaveApprovalRequest], error) {
	query, sortExpression, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{
		"leave_date":        "lr.leave_date",
		"created_at":        "lr.created_at",
		"updated_at":        "lr.updated_at",
		"status":            "lr.status",
		"leave_type":        "lr.leave_type",
		"requested_minutes": "lr.requested_minutes",
	})
	if err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	if _, err := s.users.Get(ctx, approverID); err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	if err := s.backfillLeaveAssignments(ctx); err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	defer db.Close()
	where := []string{"a.approver_user_id=?"}
	args := []any{approverID}
	applyLeaveListFilters(&where, &args, query)
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM leave_requests lr JOIN leave_request_assignments a ON a.leave_request_id=lr.id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT lr.id,lr.user_id,lr.leave_type,lr.leave_date,lr.duration_type,
		lr.start_time,lr.end_time,lr.requested_minutes,lr.reason,lr.status,lr.created_at,lr.updated_at,
		a.organization_group_id,a.is_administrator_route
		FROM leave_requests lr JOIN leave_request_assignments a ON a.leave_request_id=lr.id
		WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, lr.id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	defer rows.Close()
	var requests []LeaveApprovalRequest
	for rows.Next() {
		var item LeaveApprovalRequest
		if err := rows.Scan(&item.ID, &item.UserID, &item.LeaveType, &item.LeaveDate,
			&item.DurationType, &item.StartTime, &item.EndTime, &item.RequestedMinutes,
			&item.Reason, &item.Status, &item.CreatedAt, &item.UpdatedAt,
			&item.OrganizationGroupID, &item.AdministratorRoute); err != nil {
			return ListResult[LeaveApprovalRequest]{}, err
		}
		user, err := s.users.Get(ctx, item.UserID)
		if err != nil {
			return ListResult[LeaveApprovalRequest]{}, err
		}
		item.RequesterName = user.DisplayName
		requests = append(requests, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult[LeaveApprovalRequest]{}, err
	}
	return NewListResult(requests, query, total), nil
}

func applyLeaveListFilters(where *[]string, args *[]any, query ListQuery) {
	if query.Keyword != "" {
		pattern := "%" + query.Keyword + "%"
		*where = append(*where, `(leave_type LIKE ? COLLATE NOCASE OR leave_date LIKE ? COLLATE NOCASE OR reason LIKE ? COLLATE NOCASE OR status LIKE ? COLLATE NOCASE)`)
		*args = append(*args, pattern, pattern, pattern, pattern)
	}
	for _, filter := range []string{"status", "leave_type", "duration_type", "leave_date"} {
		if value := strings.TrimSpace(query.Filters[filter]); value != "" {
			*where = append(*where, filter+" = ?")
			*args = append(*args, value)
		}
	}
	if from := strings.TrimSpace(query.Filters["from"]); from != "" {
		*where = append(*where, "leave_date >= ?")
		*args = append(*args, from)
	}
	if to := strings.TrimSpace(query.Filters["to"]); to != "" {
		*where = append(*where, "leave_date <= ?")
		*args = append(*args, to)
	}
}

func (s *AttendanceService) backfillLeaveAssignments(ctx context.Context) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT lr.id,lr.user_id,lr.created_at FROM leave_requests lr
		WHERE lr.status='pending' AND NOT EXISTS (
			SELECT 1 FROM leave_request_assignments a WHERE a.leave_request_id=lr.id
		) AND NOT EXISTS (
			SELECT 1 FROM leave_request_events e WHERE e.leave_request_id=lr.id AND e.event_type='assigned'
		)`)
	if err != nil {
		db.Close()
		return err
	}
	type unassignedRequest struct{ id, userID, createdAt string }
	var pending []unassignedRequest
	for rows.Next() {
		var item unassignedRequest
		if err := rows.Scan(&item.id, &item.userID, &item.createdAt); err != nil {
			rows.Close()
			db.Close()
			return err
		}
		pending = append(pending, item)
	}
	if err := rows.Close(); err != nil {
		db.Close()
		return err
	}
	defer db.Close()
	for _, item := range pending {
		route, err := s.users.ResolveLeaveApproval(ctx, item.userID)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, approverID := range route.ApproverUserIDs {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO leave_request_assignments
				(leave_request_id,approver_user_id,organization_group_id,is_administrator_route,assigned_at)
				VALUES(?,?,?,?,?)`, item.id, approverID, route.GroupID, route.AdministratorRoute, item.createdAt); err != nil {
				tx.Rollback()
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO leave_request_events
			(id,leave_request_id,actor_user_id,event_type,note,occurred_at)
			VALUES(?,?,?,'assigned',?,?)`, uuid.NewString(), item.id, item.userID,
			"backfilled: "+leaveAssignmentNote(route), formatAttendanceTime(attendanceMinute(s.now()))); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *AttendanceService) DecideLeave(ctx context.Context, approverID, requestID string, input DecideLeaveInput) (*LeaveApprovalRequest, error) {
	input.Decision, input.Note = strings.TrimSpace(input.Decision), strings.TrimSpace(input.Note)
	if input.Decision != "approved" && input.Decision != "rejected" {
		return nil, fmt.Errorf("%w: decision must be approved or rejected", ErrLeaveDecision)
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
	var requesterID string
	var groupID *string
	var administratorRoute bool
	err = tx.QueryRowContext(ctx, `SELECT lr.user_id,a.organization_group_id,a.is_administrator_route
		FROM leave_requests lr JOIN leave_request_assignments a ON a.leave_request_id=lr.id
		WHERE lr.id=? AND a.approver_user_id=? AND lr.status='pending'`,
		requestID, approverID).Scan(&requesterID, &groupID, &administratorRoute)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLeaveDecision
	}
	if err != nil {
		return nil, err
	}
	if requesterID == approverID {
		return nil, ErrLeaveDecision
	}
	now := formatAttendanceTime(attendanceMinute(s.now()))
	result, err := tx.ExecContext(ctx, `UPDATE leave_requests SET status=?,updated_at=?
		WHERE id=? AND status='pending'`, input.Decision, now, requestID)
	if err != nil {
		return nil, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return nil, ErrLeaveDecision
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO leave_request_events
		(id,leave_request_id,actor_user_id,event_type,note,occurred_at)
		VALUES(?,?,?,?,?,?)`, uuid.NewString(), requestID, approverID, input.Decision, input.Note, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	request, err := s.getLeaveRequest(ctx, db, requestID)
	if err != nil {
		return nil, err
	}
	user, err := s.users.Get(ctx, requesterID)
	if err != nil {
		return nil, err
	}
	return &LeaveApprovalRequest{
		LeaveRequest: *request, RequesterName: user.DisplayName,
		OrganizationGroupID: groupID, AdministratorRoute: administratorRoute,
	}, nil
}
