package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrFinanceNotFound = errors.New("finance record not found")
	ErrFinanceInvalid  = errors.New("invalid finance input")
	ErrFinanceState    = errors.New("finance record state does not allow this operation")
)

var financeDecimalPattern = regexp.MustCompile(`^\d+(\.\d{1,6})?$`)

type FinanceAccount struct {
	ID              string  `json:"id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	AccountType     string  `json:"account_type"`
	ParentAccountID *string `json:"parent_account_id"`
	Currency        *string `json:"currency"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type AccountingPeriod struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	StartDate string  `json:"start_date"`
	EndDate   string  `json:"end_date"`
	Status    string  `json:"status"`
	ClosedAt  *string `json:"closed_at"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type JournalEntry struct {
	ID                string             `json:"id"`
	EntryDate         string             `json:"entry_date"`
	PeriodID          string             `json:"period_id"`
	SourceModule      string             `json:"source_module"`
	SourceReferenceID *string            `json:"source_reference_id"`
	Description       string             `json:"description"`
	Status            string             `json:"status"`
	CreatedByUserID   string             `json:"created_by_user_id"`
	PostedAt          *string            `json:"posted_at"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
	Lines             []JournalEntryLine `json:"lines"`
}

type JournalEntryLine struct {
	ID           string  `json:"id"`
	EntryID      string  `json:"entry_id"`
	AccountID    string  `json:"account_id"`
	DebitAmount  *string `json:"debit_amount"`
	CreditAmount *string `json:"credit_amount"`
	Currency     string  `json:"currency"`
	Description  string  `json:"description"`
	CreatedAt    string  `json:"created_at"`
}

type FinanceVendor struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	BankDetails   string `json:"bank_details"`
	TaxIdentifier string `json:"tax_identifier"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type APBill struct {
	ID              string  `json:"id"`
	VendorID        string  `json:"vendor_id"`
	BillNumber      string  `json:"bill_number"`
	BillDate        string  `json:"bill_date"`
	DueDate         string  `json:"due_date"`
	Currency        string  `json:"currency"`
	TotalAmount     string  `json:"total_amount"`
	Status          string  `json:"status"`
	JournalEntryID  *string `json:"journal_entry_id"`
	CreatedByUserID string  `json:"created_by_user_id"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type CreateFinanceAccountInput struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	AccountType     string `json:"account_type"`
	ParentAccountID string `json:"parent_account_id"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
}

type CreateAccountingPeriodInput struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type CreateJournalEntryInput struct {
	EntryDate         string                   `json:"entry_date"`
	PeriodID          string                   `json:"period_id"`
	SourceModule      string                   `json:"source_module"`
	SourceReferenceID string                   `json:"source_reference_id"`
	Description       string                   `json:"description"`
	Lines             []CreateJournalLineInput `json:"lines"`
}

type CreateJournalLineInput struct {
	AccountID    string `json:"account_id"`
	DebitAmount  string `json:"debit_amount"`
	CreditAmount string `json:"credit_amount"`
	Currency     string `json:"currency"`
	Description  string `json:"description"`
}

type CreateFinanceVendorInput struct {
	Name          string `json:"name"`
	BankDetails   string `json:"bank_details"`
	TaxIdentifier string `json:"tax_identifier"`
	Status        string `json:"status"`
}

type CreateAPBillInput struct {
	VendorID       string `json:"vendor_id"`
	BillNumber     string `json:"bill_number"`
	BillDate       string `json:"bill_date"`
	DueDate        string `json:"due_date"`
	Currency       string `json:"currency"`
	TotalAmount    string `json:"total_amount"`
	JournalEntryID string `json:"journal_entry_id"`
}

type FinanceService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewFinanceService(databaseDirectory string, users *UserService) *FinanceService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &FinanceService{
		databasePath: filepath.Join(databaseDirectory, "finance.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *FinanceService) open() (*sql.DB, error) {
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

func (s *FinanceService) CreateAccount(ctx context.Context, input CreateFinanceAccountInput) (*FinanceAccount, error) {
	code, name, accountType := strings.TrimSpace(input.Code), strings.TrimSpace(input.Name), strings.TrimSpace(input.AccountType)
	if code == "" || name == "" || !validFinanceAccountType(accountType) {
		return nil, ErrFinanceInvalid
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency != "" && !currencyPattern.MatchString(currency) {
		return nil, ErrFinanceInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO finance_accounts
		(id, code, name, account_type, parent_account_id, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, code, name, accountType, financeNullableTrim(input.ParentAccountID), financeNullableTrim(currency), financeActiveStatus(input.Status), now, now); err != nil {
		return nil, err
	}
	return s.getAccount(ctx, db, id)
}

func (s *FinanceService) ListAccounts(ctx context.Context) ([]FinanceAccount, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, code, name, account_type, parent_account_id, currency, status, created_at, updated_at
		FROM finance_accounts ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []FinanceAccount
	for rows.Next() {
		item, err := scanFinanceAccount(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *FinanceService) CreatePeriod(ctx context.Context, input CreateAccountingPeriodInput) (*AccountingPeriod, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || !validFinanceDate(input.StartDate) || !validFinanceDate(input.EndDate) || input.EndDate < input.StartDate {
		return nil, ErrFinanceInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO accounting_periods
		(id, name, start_date, end_date, status, closed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'open', NULL, ?, ?)`, id, name, input.StartDate, input.EndDate, now, now); err != nil {
		return nil, err
	}
	return s.getPeriod(ctx, db, id)
}

func (s *FinanceService) ListPeriods(ctx context.Context) ([]AccountingPeriod, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, name, start_date, end_date, status, closed_at, created_at, updated_at
		FROM accounting_periods ORDER BY start_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AccountingPeriod
	for rows.Next() {
		item, err := scanAccountingPeriod(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *FinanceService) ClosePeriod(ctx context.Context, id string) (*AccountingPeriod, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM accounting_periods WHERE id=?`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	} else if err != nil {
		return nil, err
	}
	if status != "open" {
		return nil, ErrFinanceState
	}
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `UPDATE accounting_periods SET status='closed', closed_at=?, updated_at=? WHERE id=?`, now, now, id); err != nil {
		return nil, err
	}
	return s.getPeriod(ctx, db, id)
}

func (s *FinanceService) CreateJournalEntry(ctx context.Context, actorUserID string, input CreateJournalEntryInput) (*JournalEntry, error) {
	if _, err := s.users.Get(ctx, actorUserID); err != nil {
		return nil, err
	}
	if !validFinanceDate(input.EntryDate) || strings.TrimSpace(input.PeriodID) == "" || strings.TrimSpace(input.SourceModule) == "" || len(input.Lines) < 2 {
		return nil, ErrFinanceInvalid
	}
	if err := validateJournalLines(input.Lines); err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
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
	if err := requireOpenPeriod(ctx, tx, input.PeriodID, input.EntryDate); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal_entries
		(id, entry_date, period_id, source_module, source_reference_id, description, status, created_by_user_id, posted_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'draft', ?, NULL, ?, ?)`,
		id, input.EntryDate, input.PeriodID, strings.TrimSpace(input.SourceModule), financeNullableTrim(input.SourceReferenceID), strings.TrimSpace(input.Description), actorUserID, now, now); err != nil {
		return nil, err
	}
	for _, line := range input.Lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO journal_entry_lines
			(id, entry_id, account_id, debit_amount, credit_amount, currency, description, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), id, strings.TrimSpace(line.AccountID), financeNullableTrim(line.DebitAmount),
			financeNullableTrim(line.CreditAmount), strings.ToUpper(strings.TrimSpace(line.Currency)), strings.TrimSpace(line.Description), now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetJournalEntry(ctx, id)
}

func (s *FinanceService) GetJournalEntry(ctx context.Context, id string) (*JournalEntry, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getJournalEntry(ctx, db, id)
}

func (s *FinanceService) PostJournalEntry(ctx context.Context, id string) (*JournalEntry, error) {
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
	var periodID, entryDate, status string
	if err := tx.QueryRowContext(ctx, `SELECT period_id, entry_date, status FROM journal_entries WHERE id=?`, id).Scan(&periodID, &entryDate, &status); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	} else if err != nil {
		return nil, err
	}
	if status != "draft" {
		return nil, ErrFinanceState
	}
	if err := requireOpenPeriod(ctx, tx, periodID, entryDate); err != nil {
		return nil, err
	}
	if err := requireBalancedEntry(ctx, tx, id); err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET status='posted', posted_at=?, updated_at=? WHERE id=?`, now, now, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetJournalEntry(ctx, id)
}

func (s *FinanceService) CreateVendor(ctx context.Context, input CreateFinanceVendorInput) (*FinanceVendor, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrFinanceInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO finance_vendors
		(id, name, bank_details, tax_identifier, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, strings.TrimSpace(input.BankDetails), strings.TrimSpace(input.TaxIdentifier), financeActiveStatus(input.Status), now, now); err != nil {
		return nil, err
	}
	return s.getVendor(ctx, db, id)
}

func (s *FinanceService) ListVendors(ctx context.Context) ([]FinanceVendor, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, name, bank_details, tax_identifier, status, created_at, updated_at FROM finance_vendors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []FinanceVendor
	for rows.Next() {
		var item FinanceVendor
		if err := rows.Scan(&item.ID, &item.Name, &item.BankDetails, &item.TaxIdentifier, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FinanceService) CreateAPBill(ctx context.Context, actorUserID string, input CreateAPBillInput) (*APBill, error) {
	if _, err := s.users.Get(ctx, actorUserID); err != nil {
		return nil, err
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	amount, amountErr := financeParseAmount(input.TotalAmount)
	if strings.TrimSpace(input.VendorID) == "" || strings.TrimSpace(input.BillNumber) == "" || !validFinanceDate(input.BillDate) || !validFinanceDate(input.DueDate) || !currencyPattern.MatchString(currency) || amountErr != nil || amount.Sign() <= 0 {
		return nil, ErrFinanceInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO ap_bills
		(id, vendor_id, bill_number, bill_date, due_date, currency, total_amount, status, journal_entry_id, created_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'draft', ?, ?, ?, ?)`,
		id, strings.TrimSpace(input.VendorID), strings.TrimSpace(input.BillNumber), input.BillDate, input.DueDate, currency,
		normalizeRat(amount), financeNullableTrim(input.JournalEntryID), actorUserID, now, now); err != nil {
		return nil, err
	}
	return s.getAPBill(ctx, db, id)
}

func (s *FinanceService) ApproveAPBill(ctx context.Context, id string) (*APBill, error) {
	return s.setAPBillStatus(ctx, id, "draft", "approved")
}

func (s *FinanceService) setAPBillStatus(ctx context.Context, id, from, to string) (*APBill, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := s.now().UTC().Format(time.RFC3339)
	result, err := db.ExecContext(ctx, `UPDATE ap_bills SET status=?, updated_at=? WHERE id=? AND status=?`, to, now, id, from)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ap_bills WHERE id=?`, id).Scan(&exists); err != nil {
			return nil, err
		}
		if exists == 0 {
			return nil, ErrFinanceNotFound
		}
		return nil, ErrFinanceState
	}
	return s.getAPBill(ctx, db, id)
}

func validateJournalLines(lines []CreateJournalLineInput) error {
	debits, credits := new(big.Rat), new(big.Rat)
	for _, line := range lines {
		currency := strings.ToUpper(strings.TrimSpace(line.Currency))
		if strings.TrimSpace(line.AccountID) == "" || !currencyPattern.MatchString(currency) {
			return ErrFinanceInvalid
		}
		hasDebit, hasCredit := strings.TrimSpace(line.DebitAmount) != "", strings.TrimSpace(line.CreditAmount) != ""
		if hasDebit == hasCredit {
			return ErrFinanceInvalid
		}
		if hasDebit {
			amount, err := financeParseAmount(line.DebitAmount)
			if err != nil || amount.Sign() <= 0 {
				return ErrFinanceInvalid
			}
			debits.Add(debits, amount)
		} else {
			amount, err := financeParseAmount(line.CreditAmount)
			if err != nil || amount.Sign() <= 0 {
				return ErrFinanceInvalid
			}
			credits.Add(credits, amount)
		}
	}
	if debits.Cmp(credits) != 0 {
		return fmt.Errorf("%w: debits must equal credits", ErrFinanceInvalid)
	}
	return nil
}

func requireOpenPeriod(ctx context.Context, tx *sql.Tx, periodID, entryDate string) error {
	var startDate, endDate, status string
	if err := tx.QueryRowContext(ctx, `SELECT start_date, end_date, status FROM accounting_periods WHERE id=?`, periodID).Scan(&startDate, &endDate, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrFinanceNotFound
	} else if err != nil {
		return err
	}
	if status != "open" || entryDate < startDate || entryDate > endDate {
		return ErrFinanceState
	}
	return nil
}

func requireBalancedEntry(ctx context.Context, tx *sql.Tx, entryID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT debit_amount, credit_amount FROM journal_entry_lines WHERE entry_id=?`, entryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	debits, credits := new(big.Rat), new(big.Rat)
	lineCount := 0
	for rows.Next() {
		var debit, credit *string
		if err := rows.Scan(&debit, &credit); err != nil {
			return err
		}
		lineCount++
		if debit != nil {
			amount, err := financeParseAmount(*debit)
			if err != nil {
				return err
			}
			debits.Add(debits, amount)
		}
		if credit != nil {
			amount, err := financeParseAmount(*credit)
			if err != nil {
				return err
			}
			credits.Add(credits, amount)
		}
	}
	if lineCount < 2 || debits.Cmp(credits) != 0 {
		return fmt.Errorf("%w: journal entry is not balanced", ErrFinanceInvalid)
	}
	return rows.Err()
}

func (s *FinanceService) getAccount(ctx context.Context, db *sql.DB, id string) (*FinanceAccount, error) {
	account, err := scanFinanceAccount(db.QueryRowContext(ctx, `SELECT id, code, name, account_type, parent_account_id, currency, status, created_at, updated_at FROM finance_accounts WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	}
	return account, err
}

func scanFinanceAccount(row interface{ Scan(...any) error }) (*FinanceAccount, error) {
	var item FinanceAccount
	if err := row.Scan(&item.ID, &item.Code, &item.Name, &item.AccountType, &item.ParentAccountID, &item.Currency, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *FinanceService) getPeriod(ctx context.Context, db *sql.DB, id string) (*AccountingPeriod, error) {
	period, err := scanAccountingPeriod(db.QueryRowContext(ctx, `SELECT id, name, start_date, end_date, status, closed_at, created_at, updated_at FROM accounting_periods WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	}
	return period, err
}

func scanAccountingPeriod(row interface{ Scan(...any) error }) (*AccountingPeriod, error) {
	var item AccountingPeriod
	if err := row.Scan(&item.ID, &item.Name, &item.StartDate, &item.EndDate, &item.Status, &item.ClosedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *FinanceService) getJournalEntry(ctx context.Context, db *sql.DB, id string) (*JournalEntry, error) {
	var item JournalEntry
	err := db.QueryRowContext(ctx, `SELECT id, entry_date, period_id, source_module, source_reference_id, description, status, created_by_user_id, posted_at, created_at, updated_at FROM journal_entries WHERE id=?`, id).
		Scan(&item.ID, &item.EntryDate, &item.PeriodID, &item.SourceModule, &item.SourceReferenceID, &item.Description, &item.Status, &item.CreatedByUserID, &item.PostedAt, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, entry_id, account_id, debit_amount, credit_amount, currency, description, created_at FROM journal_entry_lines WHERE entry_id=? ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var line JournalEntryLine
		if err := rows.Scan(&line.ID, &line.EntryID, &line.AccountID, &line.DebitAmount, &line.CreditAmount, &line.Currency, &line.Description, &line.CreatedAt); err != nil {
			return nil, err
		}
		item.Lines = append(item.Lines, line)
	}
	return &item, rows.Err()
}

func (s *FinanceService) getVendor(ctx context.Context, db *sql.DB, id string) (*FinanceVendor, error) {
	var item FinanceVendor
	err := db.QueryRowContext(ctx, `SELECT id, name, bank_details, tax_identifier, status, created_at, updated_at FROM finance_vendors WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.BankDetails, &item.TaxIdentifier, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	}
	return &item, err
}

func (s *FinanceService) getAPBill(ctx context.Context, db *sql.DB, id string) (*APBill, error) {
	var item APBill
	err := db.QueryRowContext(ctx, `SELECT id, vendor_id, bill_number, bill_date, due_date, currency, total_amount, status, journal_entry_id, created_by_user_id, created_at, updated_at FROM ap_bills WHERE id=?`, id).
		Scan(&item.ID, &item.VendorID, &item.BillNumber, &item.BillDate, &item.DueDate, &item.Currency, &item.TotalAmount, &item.Status, &item.JournalEntryID, &item.CreatedByUserID, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrFinanceNotFound
	}
	return &item, err
}

func financeParseAmount(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if !financeDecimalPattern.MatchString(value) {
		return nil, ErrFinanceInvalid
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, ErrFinanceInvalid
	}
	return amount, nil
}

func financeNullableTrim(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func financeActiveStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "active"
	}
	return value
}

func validFinanceDate(value string) bool {
	_, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	return err == nil
}

func validFinanceAccountType(value string) bool {
	switch value {
	case "asset", "liability", "equity", "revenue", "expense":
		return true
	default:
		return false
	}
}
