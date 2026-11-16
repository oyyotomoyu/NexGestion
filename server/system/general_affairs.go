package system

// General Affairs System (docs/System/general-affairs-system.md): assets and
// custody, office supply requisitions, resource booking, seals, polls and group
// meal ordering. This file holds the shared plumbing; each area lives in its
// own general_affairs_*.go file.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrGANotFound = errors.New("general affairs record not found")
	ErrGAInvalid  = errors.New("invalid general affairs input")
	ErrGAConflict = errors.New("general affairs conflict")
)

func gaInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrGAInvalid, fmt.Sprintf(format, args...))
}

func gaConflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrGAConflict, fmt.Sprintf(format, args...))
}

// GAActor is the caller plus the effective permission keys the API layer
// resolved for them. Several rules in Section 4 are "own record, or anyone's
// with the manage permission", so services need both.
type GAActor struct {
	UserID      string
	Permissions map[string]bool
}

func (a GAActor) Has(key string) bool { return a.Permissions[key] }

// Request types used to find an Approval flow template (approval-system.md
// Section 2.1's dotted module.request convention).
const (
	GARequestTypeRequisition = "general_affairs.supply_requisition"
	GARequestTypeSealUsage   = "general_affairs.seal_usage"
)

type GeneralAffairsService struct {
	databasePath      string
	databaseDirectory string
	users             *UserService
	approvals         *ApprovalService
	notifications     *NotificationService
	now               func() time.Time
}

// NewGeneralAffairsService wires the optional collaborators. approvals and
// notifications may be nil: requests then fall back to manager approval
// (Section 3.1) and notices are simply skipped (Section 3.6).
func NewGeneralAffairsService(databaseDirectory string, users *UserService, approvals *ApprovalService, notifications *NotificationService) *GeneralAffairsService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &GeneralAffairsService{
		databasePath:      filepath.Join(databaseDirectory, "general_affairs.db"),
		databaseDirectory: databaseDirectory,
		users:             users,
		approvals:         approvals,
		notifications:     notifications,
		now:               func() time.Time { return time.Now().UTC() },
	}
}

func (s *GeneralAffairsService) open() (*sql.DB, error) {
	return openSQLite(s.databasePath)
}

func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{`PRAGMA busy_timeout = 5000`, `PRAGMA foreign_keys = ON`} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func (s *GeneralAffairsService) timestamp() string { return gaTime(s.now()) }

func gaTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// parseGATime accepts any RFC3339 time and normalizes it to UTC seconds, the
// storage format the booking overlap triggers compare as strings.
func parseGATime(field, raw string) (string, time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return "", time.Time{}, gaInvalid("%s must be an RFC3339 timestamp", field)
	}
	parsed = parsed.UTC().Truncate(time.Second)
	return gaTime(parsed), parsed, nil
}

func parseGADate(field, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if _, err := time.Parse("2006-01-02", raw); err != nil {
		return "", gaInvalid("%s must be a YYYY-MM-DD date", field)
	}
	return raw, nil
}

// requireText trims and bounds a required free-text field.
func requireText(field, value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", gaInvalid("%s is required", field)
	}
	if len([]rune(value)) > max {
		return "", gaInvalid("%s must be at most %d characters", field, max)
	}
	return value, nil
}

// optionalText trims an optional field; an empty result becomes NULL.
func optionalText(field string, value *string, max int) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if len([]rune(trimmed)) > max {
		return nil, gaInvalid("%s must be at most %d characters", field, max)
	}
	return &trimmed, nil
}

var gaDecimalPattern = regexp.MustCompile(`^\d{1,12}(\.\d+)?$`)

// parseGADecimal validates a non-negative decimal string with at most scale
// fractional digits and returns it normalized (no trailing zeros).
func parseGADecimal(field, raw string, scale int, allowZero bool) (*big.Rat, string, error) {
	raw = strings.TrimSpace(raw)
	if !gaDecimalPattern.MatchString(raw) {
		return nil, "", gaInvalid("%s must be a non-negative decimal number", field)
	}
	if dot := strings.IndexByte(raw, '.'); dot >= 0 && len(raw)-dot-1 > scale {
		return nil, "", gaInvalid("%s allows at most %d decimal places", field, scale)
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, "", gaInvalid("%s must be a decimal number", field)
	}
	if !allowZero && value.Sign() == 0 {
		return nil, "", gaInvalid("%s must be greater than zero", field)
	}
	return value, formatGADecimal(value, scale), nil
}

func formatGADecimal(value *big.Rat, scale int) string {
	text := value.FloatString(scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text
}

func formatGAAmount(value *big.Rat) string { return value.FloatString(2) }

func optionalDecimal(field string, raw *string, scale int) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	_, normalized, err := parseGADecimal(field, *raw, scale, false)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

// requireActiveUser rejects unknown, deleted or disabled users being named as
// custodians, invitees, bookers and the like.
func (s *GeneralAffairsService) requireActiveUser(ctx context.Context, field, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return gaInvalid("%s is required", field)
	}
	user, err := s.users.Get(ctx, userID)
	if errors.Is(err, ErrUserNotFound) {
		return gaInvalid("%s %q does not exist", field, userID)
	}
	if err != nil {
		return err
	}
	if user.DeletedAt != nil || user.Status != "active" {
		return gaInvalid("%s %q is not an active user", field, userID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Approval routing (Section 3.1)
// ---------------------------------------------------------------------------

// routeApproval submits subjectID to the Approval engine when an active flow
// template exists for requestType. Otherwise it resolves the requester's
// manager chain (group-system.md Section 5, the same resolution leave uses)
// and records them as the fallback deciders.
func (s *GeneralAffairsService) routeApproval(ctx context.Context, db *sql.DB, requestType, subjectType, subjectID, requesterID string, amount *string) (*string, error) {
	if s.approvals != nil {
		template, err := s.approvals.ActiveFlowTemplateByRequestType(ctx, requestType)
		switch {
		case err == nil:
			request, err := s.approvals.SubmitRequest(ctx, requesterID, SubmitApprovalRequestInput{
				FlowTemplateID: template.ID, SourceModule: "general_affairs", SourceReferenceID: subjectID, Amount: amount,
			})
			if err != nil {
				return nil, err
			}
			return &request.ID, nil
		case !errors.Is(err, ErrApprovalNotFound):
			return nil, err
		}
	}
	route, err := s.users.ResolveLeaveApproval(ctx, requesterID)
	if err != nil {
		return nil, err
	}
	for _, approverID := range route.ApproverUserIDs {
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO ga_fallback_approvers (subject_type, subject_id, user_id)
			VALUES (?, ?, ?)`, subjectType, subjectID, approverID); err != nil {
			return nil, err
		}
	}
	s.notifyUsers(ctx, route.ApproverUserIDs, "important", "General Affairs approval needed",
		fmt.Sprintf("A %s request is waiting for your decision.", strings.ReplaceAll(subjectType, "_", " ")))
	return nil, nil
}

func (s *GeneralAffairsService) isFallbackApprover(ctx context.Context, db *sql.DB, subjectType, subjectID, userID string) (bool, error) {
	var found int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ga_fallback_approvers WHERE subject_type = ? AND subject_id = ? AND user_id = ?`,
		subjectType, subjectID, userID).Scan(&found)
	return found > 0, err
}

func (s *GeneralAffairsService) fallbackApprovers(ctx context.Context, db *sql.DB, subjectType, subjectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT user_id FROM ga_fallback_approvers WHERE subject_type = ? AND subject_id = ? ORDER BY user_id`, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// approvalOutcome reads the linked Approval request's terminal status, the
// read-derived sync used instead of the Approval engine writing back into this
// module (approval-system.md Section 2.5: it only ever notifies). It returns
// "" while the request is still undecided.
func (s *GeneralAffairsService) approvalOutcome(ctx context.Context, approvalRequestID string) (string, error) {
	if s.approvals == nil {
		return "", nil
	}
	request, err := s.approvals.GetRequest(ctx, approvalRequestID)
	if errors.Is(err, ErrApprovalNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	switch request.Status {
	case ApprovalRequestApproved:
		return "approved", nil
	case ApprovalRequestRejected:
		return "rejected", nil
	case "cancelled":
		return "cancelled", nil
	}
	return "", nil
}

// ---------------------------------------------------------------------------
// Notifications (Section 3.6). Best effort: a notice that cannot be delivered
// never undoes the business action that triggered it.
// ---------------------------------------------------------------------------

func (s *GeneralAffairsService) notify(ctx context.Context, typeCode, title, message string, audiences []NotificationAudienceInput) {
	if s.notifications == nil || len(audiences) == 0 {
		return
	}
	if _, err := s.notifications.Create(ctx, adminUserID, CreateNotificationInput{
		Title: truncate(title, 100), Message: truncate(message, 1000), TypeCode: typeCode, DurationCode: "week", Audiences: audiences,
	}); err != nil {
		log.Printf("general affairs notification %q failed: %v", title, err)
	}
}

func (s *GeneralAffairsService) notifyUsers(ctx context.Context, userIDs []string, typeCode, title, message string) {
	seen := map[string]bool{}
	audiences := []NotificationAudienceInput{}
	for _, id := range userIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			audiences = append(audiences, NotificationAudienceInput{Scope: "user", TargetUserID: id})
		}
	}
	s.notify(ctx, typeCode, title, message, audiences)
}

// ---------------------------------------------------------------------------
// Optional cross-module reads (Sections 3.4 and 3.7). Procurement and
// Inventory are read straight from their own databases when present; a
// missing database means the module is not installed, never an error.
// ---------------------------------------------------------------------------

var errGAModuleAbsent = errors.New("module not installed")

func (s *GeneralAffairsService) readForeignStatus(ctx context.Context, databaseName, query, id string) (string, error) {
	path := filepath.Join(s.databaseDirectory, databaseName)
	if _, err := os.Stat(path); err != nil {
		return "", errGAModuleAbsent
	}
	db, err := openSQLite(path)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var status string
	err = db.QueryRowContext(ctx, query, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrGANotFound
	}
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return "", errGAModuleAbsent
	}
	return status, err
}

func (s *GeneralAffairsService) purchaseOrderStatus(ctx context.Context, id string) (string, error) {
	return s.readForeignStatus(ctx, "procurement.db", `SELECT status FROM operation_documents WHERE id = ? AND kind = 'purchase_order'`, id)
}

func (s *GeneralAffairsService) outboundStatus(ctx context.Context, id string) (string, error) {
	return s.readForeignStatus(ctx, "inventory.db", `SELECT status FROM inventory_records WHERE id = ? AND kind = 'outbound'`, id)
}

// ---------------------------------------------------------------------------
// Listing
// ---------------------------------------------------------------------------

type gaListSpec struct {
	from         string // FROM ... clause including joins
	where        []string
	args         []any
	keywordWhere string // e.g. "(name LIKE ? OR category LIKE ?)", one ? per keywordArgs
	keywordArgs  int
	sorts        map[string]string
	defaultSort  string
	defaultOrder string
	columns      string
}

func gaList[T any](ctx context.Context, db *sql.DB, query ListQuery, spec gaListSpec, scan func(rowScanner) (T, error)) (ListResult[T], error) {
	query, sortExpression, err := NormalizeListQuery(query, spec.defaultSort, spec.defaultOrder, spec.sorts)
	if err != nil {
		return ListResult[T]{}, err
	}
	where, args := append([]string{"1 = 1"}, spec.where...), append([]any{}, spec.args...)
	if query.Keyword != "" && spec.keywordWhere != "" {
		where = append(where, spec.keywordWhere)
		for i := 0; i < spec.keywordArgs; i++ {
			args = append(args, "%"+query.Keyword+"%")
		}
	}
	clause := " FROM " + spec.from + " WHERE " + strings.Join(where, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*)"+clause, args...).Scan(&total); err != nil {
		return ListResult[T]{}, err
	}
	rows, err := db.QueryContext(ctx, "SELECT "+spec.columns+clause+" ORDER BY "+sortExpression+" "+strings.ToUpper(query.Order)+
		" LIMIT ? OFFSET ?", append(args, query.PageSize, ListOffset(query))...)
	if err != nil {
		return ListResult[T]{}, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return ListResult[T]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult[T]{}, err
	}
	return NewListResult(items, query, total), nil
}

func filterValue(query ListQuery, key string) string { return strings.TrimSpace(query.Filters[key]) }

// ---------------------------------------------------------------------------
// Maintenance: overdue borrowed items, poll deadlines.
// ---------------------------------------------------------------------------

// RunMaintenance closes polls past closes_at and reminds bookers of overdue
// borrowed items once each (Section 3.6). Intended to run hourly.
func (s *GeneralAffairsService) RunMaintenance(ctx context.Context) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	now := s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE ga_polls SET status = 'closed', closed_at = ?, updated_at = ?
		WHERE status = 'open' AND closes_at IS NOT NULL AND closes_at <= ?`, now, now, now); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, `SELECT b.id, b.booked_by_user_id, r.name, b.ends_at FROM ga_bookings b
		JOIN ga_resources r ON r.id = b.resource_id
		WHERE r.requires_return = 1 AND b.status = 'confirmed' AND b.returned_at IS NULL
			AND b.ends_at <= ? AND b.overdue_notified_at IS NULL`, now)
	if err != nil {
		return err
	}
	type overdue struct{ id, booker, resource, endsAt string }
	var items []overdue
	for rows.Next() {
		var item overdue
		if err := rows.Scan(&item.id, &item.booker, &item.resource, &item.endsAt); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	for _, item := range items {
		s.notifyUsers(ctx, []string{item.booker}, "warning", "Borrowed item overdue",
			fmt.Sprintf("%s was due back at %s. Please return it to General Affairs.", item.resource, item.endsAt))
		if _, err := db.ExecContext(ctx, `UPDATE ga_bookings SET overdue_notified_at = ? WHERE id = ?`, now, item.id); err != nil {
			return err
		}
	}
	return nil
}
