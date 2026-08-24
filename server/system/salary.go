package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Compensation basis values. See docs/System/salary-system.md Section 2.1.
const (
	CompensationBasisHourly       = "hourly"
	CompensationBasisDaily        = "daily"
	CompensationBasisWeekly       = "weekly"
	CompensationBasisMonthly      = "monthly"
	CompensationBasisAnnual       = "annual"
	CompensationBasisPieceRate    = "piece_rate"
	CompensationBasisProjectBased = "project_based"
	CompensationBasisContract     = "contract"
)

var validCompensationBases = map[string]bool{
	CompensationBasisHourly:       true,
	CompensationBasisDaily:        true,
	CompensationBasisWeekly:       true,
	CompensationBasisMonthly:      true,
	CompensationBasisAnnual:       true,
	CompensationBasisPieceRate:    true,
	CompensationBasisProjectBased: true,
	CompensationBasisContract:     true,
}

var (
	ErrSalaryNotFound     = errors.New("compensation record not found")
	ErrSalaryInvalidInput = errors.New("invalid compensation record input")
	ErrSalaryOverlap      = errors.New("compensation record overlaps the currently active record")
)

var (
	rateAmountPattern = regexp.MustCompile(`^\d+(\.\d{1,6})?$`)
	currencyPattern   = regexp.MustCompile(`^[A-Z]{3}$`)
)

const compensationDateLayout = "2006-01-02"

// CompensationRecord is one effective-dated compensation basis and rate
// assignment for an employee. See docs/System/salary-system.md Section 2.3.
type CompensationRecord struct {
	ID                 string  `json:"id"`
	UserID             string  `json:"user_id"`
	CompensationBasis  string  `json:"compensation_basis"`
	RateAmount         string  `json:"rate_amount"`
	Currency           string  `json:"currency"`
	JurisdictionID     string  `json:"jurisdiction_id"`
	EffectiveStartDate string  `json:"effective_start_date"`
	EffectiveEndDate   *string `json:"effective_end_date"`
	Note               string  `json:"note"`
	CreatedByUserID    string  `json:"created_by_user_id"`
	CreatedAt          string  `json:"created_at"`
}

type CreateCompensationRecordInput struct {
	CompensationBasis  string `json:"compensation_basis"`
	RateAmount         string `json:"rate_amount"`
	Currency           string `json:"currency"`
	JurisdictionID     string `json:"jurisdiction_id"`
	EffectiveStartDate string `json:"effective_start_date"`
	Note               string `json:"note"`
}

type SalaryService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewSalaryService(databaseDirectory string, users *UserService) *SalaryService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &SalaryService{
		databasePath: filepath.Join(databaseDirectory, "salary.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *SalaryService) open() (*sql.DB, error) {
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

// CreateCompensationRecord assigns a new compensation basis/rate to an
// employee, closing the previously active record rather than overwriting it.
// See docs/System/salary-system.md Section 2.3.
func (s *SalaryService) CreateCompensationRecord(ctx context.Context, actorUserID, userID string, input CreateCompensationRecordInput) (*CompensationRecord, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	if !validCompensationBases[input.CompensationBasis] {
		return nil, fmt.Errorf("%w: unknown compensation basis %q", ErrSalaryInvalidInput, input.CompensationBasis)
	}
	if !rateAmountPattern.MatchString(input.RateAmount) {
		return nil, fmt.Errorf("%w: rate_amount must be a non-negative decimal number", ErrSalaryInvalidInput)
	}
	if input.RateAmount == "0" {
		return nil, fmt.Errorf("%w: rate_amount must be greater than zero", ErrSalaryInvalidInput)
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if !currencyPattern.MatchString(currency) {
		return nil, fmt.Errorf("%w: currency must be a 3-letter ISO 4217 code", ErrSalaryInvalidInput)
	}
	jurisdictionID := strings.TrimSpace(input.JurisdictionID)
	if jurisdictionID == "" {
		return nil, fmt.Errorf("%w: jurisdiction_id is required", ErrSalaryInvalidInput)
	}
	startDate, err := time.Parse(compensationDateLayout, strings.TrimSpace(input.EffectiveStartDate))
	if err != nil {
		return nil, fmt.Errorf("%w: effective_start_date must be formatted as YYYY-MM-DD", ErrSalaryInvalidInput)
	}
	note := strings.TrimSpace(input.Note)

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

	var activeID, activeStart string
	err = tx.QueryRowContext(ctx, `SELECT id, effective_start_date FROM compensation_records
		WHERE user_id=? AND effective_end_date IS NULL`, userID).Scan(&activeID, &activeStart)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No active record yet; nothing to close.
	case err != nil:
		return nil, err
	default:
		if input.EffectiveStartDate <= activeStart {
			return nil, fmt.Errorf("%w: effective_start_date must be after the current record's start date (%s)", ErrSalaryOverlap, activeStart)
		}
		closeDate := startDate.AddDate(0, 0, -1).Format(compensationDateLayout)
		if _, err := tx.ExecContext(ctx, `UPDATE compensation_records SET effective_end_date=? WHERE id=?`, closeDate, activeID); err != nil {
			return nil, err
		}
	}

	id := uuid.NewString()
	createdAt := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO compensation_records
		(id, user_id, compensation_basis, rate_amount, currency, jurisdiction_id, effective_start_date, effective_end_date, note, created_by_user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?, ?)`,
		id, userID, input.CompensationBasis, input.RateAmount, currency, jurisdictionID,
		input.EffectiveStartDate, note, actorUserID, createdAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getCompensationRecord(ctx, db, id)
}

func (s *SalaryService) getCompensationRecord(ctx context.Context, db *sql.DB, id string) (*CompensationRecord, error) {
	record, err := scanCompensationRecord(db.QueryRowContext(ctx, `SELECT id, user_id, compensation_basis, rate_amount, currency,
		jurisdiction_id, effective_start_date, effective_end_date, note, created_by_user_id, created_at
		FROM compensation_records WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSalaryNotFound
	}
	return record, err
}

func scanCompensationRecord(row *sql.Row) (*CompensationRecord, error) {
	var r CompensationRecord
	if err := row.Scan(&r.ID, &r.UserID, &r.CompensationBasis, &r.RateAmount, &r.Currency,
		&r.JurisdictionID, &r.EffectiveStartDate, &r.EffectiveEndDate, &r.Note, &r.CreatedByUserID, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// CurrentCompensationRecord returns the employee's currently active
// compensation basis and rate (Section 2: exactly one active record at a
// time).
func (s *SalaryService) CurrentCompensationRecord(ctx context.Context, userID string) (*CompensationRecord, error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	record, err := scanCompensationRecord(db.QueryRowContext(ctx, `SELECT id, user_id, compensation_basis, rate_amount, currency,
		jurisdiction_id, effective_start_date, effective_end_date, note, created_by_user_id, created_at
		FROM compensation_records WHERE user_id=? AND effective_end_date IS NULL`, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSalaryNotFound
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

// ListCompensationRecords returns the effective-dated rate history for an
// employee (Section 2.3), newest first by default.
func (s *SalaryService) ListCompensationRecords(ctx context.Context, userID string, query ListQuery) (ListResult[CompensationRecord], error) {
	if _, err := s.users.Get(ctx, userID); err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	query, sortExpression, err := NormalizeListQuery(query, "effective_start_date", "desc", map[string]string{
		"effective_start_date": "effective_start_date",
		"created_at":           "created_at",
	})
	if err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	defer db.Close()

	where := []string{"user_id=?"}
	args := []any{userID}
	if basis := strings.TrimSpace(query.Filters["compensation_basis"]); basis != "" {
		where = append(where, "compensation_basis = ?")
		args = append(args, basis)
	}
	switch strings.TrimSpace(query.Filters["active"]) {
	case "true":
		where = append(where, "effective_end_date IS NULL")
	case "false":
		where = append(where, "effective_end_date IS NOT NULL")
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM compensation_records WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.PageSize, ListOffset(query))
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, compensation_basis, rate_amount, currency,
		jurisdiction_id, effective_start_date, effective_end_date, note, created_by_user_id, created_at
		FROM compensation_records WHERE `+whereSQL+` ORDER BY `+sortExpression+` `+query.Order+`, id ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	defer rows.Close()
	items := make([]CompensationRecord, 0, query.PageSize)
	for rows.Next() {
		var r CompensationRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.CompensationBasis, &r.RateAmount, &r.Currency,
			&r.JurisdictionID, &r.EffectiveStartDate, &r.EffectiveEndDate, &r.Note, &r.CreatedByUserID, &r.CreatedAt); err != nil {
			return ListResult[CompensationRecord]{}, err
		}
		items = append(items, r)
	}
	if err := rows.Err(); err != nil {
		return ListResult[CompensationRecord]{}, err
	}
	return NewListResult(items, query, total), nil
}
