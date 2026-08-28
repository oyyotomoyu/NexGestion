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

const (
	CheckoutStatusInProgress = "in_progress"
	CheckoutStatusCompleted  = "completed"
	CheckoutStatusVoided     = "voided"
)

var (
	ErrCheckoutNotFound = errors.New("checkout record not found")
	ErrCheckoutInvalid  = errors.New("invalid checkout input")
	ErrCheckoutState    = errors.New("checkout transaction state does not allow this operation")
)

var checkoutDecimalPattern = regexp.MustCompile(`^\d+(\.\d{1,6})?$`)

type CheckoutTransaction struct {
	ID             string             `json:"id"`
	WarehouseID    string             `json:"warehouse_id"`
	CashierUserID  string             `json:"cashier_user_id"`
	CRMCustomerID  *string            `json:"crm_customer_id"`
	Status         string             `json:"status"`
	Currency       string             `json:"currency"`
	SubtotalAmount string             `json:"subtotal_amount"`
	DiscountAmount string             `json:"discount_amount"`
	TotalAmount    string             `json:"total_amount"`
	CompletedAt    *string            `json:"completed_at"`
	CreatedAt      string             `json:"created_at"`
	UpdatedAt      string             `json:"updated_at"`
	Lines          []CheckoutLine     `json:"lines"`
	Discounts      []CheckoutDiscount `json:"discounts"`
	Payments       []CheckoutPayment  `json:"payments"`
}

type CheckoutLine struct {
	ID                    string  `json:"id"`
	CheckoutTransactionID string  `json:"checkout_transaction_id"`
	Description           string  `json:"description"`
	InventoryItemID       *string `json:"inventory_item_id"`
	Quantity              string  `json:"quantity"`
	UnitPrice             string  `json:"unit_price"`
	LineAmount            string  `json:"line_amount"`
	CreatedAt             string  `json:"created_at"`
}

type CheckoutDiscount struct {
	ID                    string `json:"id"`
	CheckoutTransactionID string `json:"checkout_transaction_id"`
	SourceType            string `json:"source_type"`
	SourceReferenceID     string `json:"source_reference_id"`
	Amount                string `json:"amount"`
	CreatedAt             string `json:"created_at"`
}

type CheckoutPayment struct {
	ID                    string  `json:"id"`
	CheckoutTransactionID string  `json:"checkout_transaction_id"`
	Method                string  `json:"method"`
	Amount                string  `json:"amount"`
	Reference             *string `json:"reference"`
	CreatedAt             string  `json:"created_at"`
}

type CheckoutCoupon struct {
	ID            string  `json:"id"`
	Code          string  `json:"code"`
	CouponType    string  `json:"coupon_type"`
	DiscountType  *string `json:"discount_type"`
	DiscountValue *string `json:"discount_value"`
	ValueAmount   *string `json:"value_amount"`
	UsageLimit    *int    `json:"usage_limit"`
	RedeemedCount int     `json:"redeemed_count"`
	StartsAt      *string `json:"starts_at"`
	EndsAt        *string `json:"ends_at"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type CheckoutPromotionRule struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	DiscountType      string  `json:"discount_type"`
	DiscountValue     string  `json:"discount_value"`
	Scope             string  `json:"scope"`
	InventoryItemID   *string `json:"inventory_item_id"`
	MinSubtotalAmount *string `json:"min_subtotal_amount"`
	MembershipTierID  *string `json:"membership_tier_id"`
	StartsAt          *string `json:"starts_at"`
	EndsAt            *string `json:"ends_at"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

type CreateCheckoutTransactionInput struct {
	WarehouseID   string `json:"warehouse_id"`
	CRMCustomerID string `json:"crm_customer_id"`
	Currency      string `json:"currency"`
}

type AddCheckoutLineInput struct {
	Description     string `json:"description"`
	InventoryItemID string `json:"inventory_item_id"`
	Quantity        string `json:"quantity"`
	UnitPrice       string `json:"unit_price"`
}

type AddCheckoutDiscountInput struct {
	SourceType        string `json:"source_type"`
	SourceReferenceID string `json:"source_reference_id"`
	Amount            string `json:"amount"`
}

type AddCheckoutPaymentInput struct {
	Method    string `json:"method"`
	Amount    string `json:"amount"`
	Reference string `json:"reference"`
}

type CreateCheckoutCouponInput struct {
	Code          string `json:"code"`
	CouponType    string `json:"coupon_type"`
	DiscountType  string `json:"discount_type"`
	DiscountValue string `json:"discount_value"`
	ValueAmount   string `json:"value_amount"`
	UsageLimit    *int   `json:"usage_limit"`
	StartsAt      string `json:"starts_at"`
	EndsAt        string `json:"ends_at"`
	Status        string `json:"status"`
}

type CreateCheckoutPromotionRuleInput struct {
	Name              string `json:"name"`
	DiscountType      string `json:"discount_type"`
	DiscountValue     string `json:"discount_value"`
	Scope             string `json:"scope"`
	InventoryItemID   string `json:"inventory_item_id"`
	MinSubtotalAmount string `json:"min_subtotal_amount"`
	MembershipTierID  string `json:"membership_tier_id"`
	StartsAt          string `json:"starts_at"`
	EndsAt            string `json:"ends_at"`
	Status            string `json:"status"`
}

type CheckoutScanResolution struct {
	Type   string          `json:"type"`
	Coupon *CheckoutCoupon `json:"coupon,omitempty"`
}

type CheckoutService struct {
	databasePath string
	users        *UserService
	now          func() time.Time
}

func NewCheckoutService(databaseDirectory string, users *UserService) *CheckoutService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &CheckoutService{
		databasePath: filepath.Join(databaseDirectory, "checkout.db"),
		users:        users,
		now:          time.Now,
	}
}

func (s *CheckoutService) open() (*sql.DB, error) {
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

func (s *CheckoutService) CreateTransaction(ctx context.Context, cashierUserID string, input CreateCheckoutTransactionInput) (*CheckoutTransaction, error) {
	if _, err := s.users.Get(ctx, cashierUserID); err != nil {
		return nil, err
	}
	warehouseID := strings.TrimSpace(input.WarehouseID)
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if warehouseID == "" || !currencyPattern.MatchString(currency) {
		return nil, ErrCheckoutInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	customerID := checkoutNullableTrim(input.CRMCustomerID)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO checkout_transactions
		(id, warehouse_id, cashier_user_id, crm_customer_id, status, currency, subtotal_amount, discount_amount, total_amount, completed_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'in_progress', ?, '0', '0', '0', NULL, ?, ?)`,
		id, warehouseID, cashierUserID, customerID, currency, now, now); err != nil {
		return nil, err
	}
	return s.GetTransaction(ctx, id)
}

func (s *CheckoutService) GetTransaction(ctx context.Context, id string) (*CheckoutTransaction, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getTransaction(ctx, db, id)
}

func (s *CheckoutService) ListTransactions(ctx context.Context, query ListQuery) (ListResult[CheckoutTransaction], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{
			"created_at": "created_at",
			"updated_at": "updated_at",
			"status":     "status",
		})
	if err != nil {
		return ListResult[CheckoutTransaction]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CheckoutTransaction]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "(id LIKE ? OR cashier_user_id LIKE ? OR warehouse_id LIKE ?)")
		keyword := "%" + normalized.Keyword + "%"
		args = append(args, keyword, keyword, keyword)
	}
	for key, value := range normalized.Filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "status", "cashier_user_id":
			whereParts = append(whereParts, key+" = ?")
			args = append(args, value)
		default:
			return ListResult[CheckoutTransaction]{}, ErrInvalidListQuery
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	countQuery := "SELECT COUNT(*) FROM checkout_transactions" + where
	var total int
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return ListResult[CheckoutTransaction]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, warehouse_id, cashier_user_id, crm_customer_id, status, currency,
		subtotal_amount, discount_amount, total_amount, completed_at, created_at, updated_at
		FROM checkout_transactions`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CheckoutTransaction]{}, err
	}
	defer rows.Close()
	items := []CheckoutTransaction{}
	for rows.Next() {
		item, err := scanCheckoutTransaction(rows)
		if err != nil {
			return ListResult[CheckoutTransaction]{}, err
		}
		items = append(items, *item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CheckoutService) AddLine(ctx context.Context, transactionID string, input AddCheckoutLineInput) (*CheckoutTransaction, error) {
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return nil, ErrCheckoutInvalid
	}
	quantity, err := parseCheckoutAmount(input.Quantity)
	if err != nil || quantity.Sign() <= 0 {
		return nil, ErrCheckoutInvalid
	}
	unitPrice, err := parseCheckoutAmount(input.UnitPrice)
	if err != nil {
		return nil, ErrCheckoutInvalid
	}
	lineAmount := new(big.Rat).Mul(quantity, unitPrice)
	return s.withMutableTransaction(ctx, transactionID, func(tx *sql.Tx, now string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO checkout_transaction_lines
			(id, checkout_transaction_id, description, inventory_item_id, quantity, unit_price, line_amount, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), transactionID, description, checkoutNullableTrim(input.InventoryItemID),
			normalizeRat(quantity), normalizeRat(unitPrice), normalizeRat(lineAmount), now)
		return err
	})
}

func (s *CheckoutService) AddDiscount(ctx context.Context, transactionID string, input AddCheckoutDiscountInput) (*CheckoutTransaction, error) {
	sourceType := strings.TrimSpace(input.SourceType)
	sourceRef := strings.TrimSpace(input.SourceReferenceID)
	amount, err := parseCheckoutAmount(input.Amount)
	if sourceRef == "" || err != nil || amount.Sign() <= 0 || (sourceType != "promotion" && sourceType != "coupon" && sourceType != "points") {
		return nil, ErrCheckoutInvalid
	}
	return s.withMutableTransaction(ctx, transactionID, func(tx *sql.Tx, now string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO checkout_discounts
			(id, checkout_transaction_id, source_type, source_reference_id, amount, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), transactionID, sourceType, sourceRef, normalizeRat(amount), now)
		return err
	})
}

func (s *CheckoutService) AddPayment(ctx context.Context, transactionID string, input AddCheckoutPaymentInput) (*CheckoutTransaction, error) {
	method := strings.TrimSpace(input.Method)
	amount, err := parseCheckoutAmount(input.Amount)
	if err != nil || amount.Sign() <= 0 || !validCheckoutPaymentMethod(method) {
		return nil, ErrCheckoutInvalid
	}
	return s.withMutableTransaction(ctx, transactionID, func(tx *sql.Tx, now string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO checkout_payments
			(id, checkout_transaction_id, method, amount, reference, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), transactionID, method, normalizeRat(amount), checkoutNullableTrim(input.Reference), now)
		return err
	})
}

func (s *CheckoutService) CompleteTransaction(ctx context.Context, transactionID string) (*CheckoutTransaction, error) {
	return s.withMutableTransaction(ctx, transactionID, func(tx *sql.Tx, now string) error {
		var total, paid string
		if err := tx.QueryRowContext(ctx, `SELECT total_amount FROM checkout_transactions WHERE id=?`, transactionID).Scan(&total); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CAST(amount AS REAL)),0) FROM checkout_payments WHERE checkout_transaction_id=?`, transactionID).Scan(&paid); err != nil {
			return err
		}
		totalRat, totalErr := parseCheckoutAmount(total)
		paidRat, paidErr := parseCheckoutAmount(paid)
		if totalErr != nil || paidErr != nil || totalRat.Cmp(paidRat) != 0 {
			return fmt.Errorf("%w: payments must equal total amount", ErrCheckoutInvalid)
		}
		completedAt := now
		_, err := tx.ExecContext(ctx, `UPDATE checkout_transactions SET status='completed', completed_at=?, updated_at=? WHERE id=?`, completedAt, now, transactionID)
		return err
	})
}

func (s *CheckoutService) VoidTransaction(ctx context.Context, transactionID string) (*CheckoutTransaction, error) {
	return s.withMutableTransaction(ctx, transactionID, func(tx *sql.Tx, now string) error {
		_, err := tx.ExecContext(ctx, `UPDATE checkout_transactions SET status='voided', updated_at=? WHERE id=?`, now, transactionID)
		return err
	})
}

func (s *CheckoutService) CreateCoupon(ctx context.Context, input CreateCheckoutCouponInput) (*CheckoutCoupon, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	couponType := strings.TrimSpace(input.CouponType)
	status := normalizedCheckoutStatus(input.Status)
	if code == "" || (couponType != "discount" && couponType != "voucher") {
		return nil, ErrCheckoutInvalid
	}
	var discountType, discountValue, valueAmount any
	if couponType == "discount" {
		if input.DiscountType != "percentage" && input.DiscountType != "fixed_amount" {
			return nil, ErrCheckoutInvalid
		}
		if _, err := parseCheckoutAmount(input.DiscountValue); err != nil {
			return nil, ErrCheckoutInvalid
		}
		discountType, discountValue = input.DiscountType, input.DiscountValue
	} else {
		if amount, err := parseCheckoutAmount(input.ValueAmount); err != nil || amount.Sign() <= 0 {
			return nil, ErrCheckoutInvalid
		}
		valueAmount = input.ValueAmount
	}
	if input.UsageLimit != nil && *input.UsageLimit <= 0 {
		return nil, ErrCheckoutInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO checkout_coupons
		(id, code, coupon_type, discount_type, discount_value, value_amount, usage_limit, starts_at, ends_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, code, couponType, discountType, discountValue, valueAmount, input.UsageLimit, checkoutNullableTrim(input.StartsAt), checkoutNullableTrim(input.EndsAt), status, now, now); err != nil {
		return nil, err
	}
	return s.getCoupon(ctx, db, id)
}

func (s *CheckoutService) CreatePromotionRule(ctx context.Context, input CreateCheckoutPromotionRuleInput) (*CheckoutPromotionRule, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || (input.DiscountType != "percentage" && input.DiscountType != "fixed_amount") || (input.Scope != "transaction" && input.Scope != "item") {
		return nil, ErrCheckoutInvalid
	}
	if _, err := parseCheckoutAmount(input.DiscountValue); err != nil {
		return nil, ErrCheckoutInvalid
	}
	if strings.TrimSpace(input.MinSubtotalAmount) != "" {
		if _, err := parseCheckoutAmount(input.MinSubtotalAmount); err != nil {
			return nil, ErrCheckoutInvalid
		}
	}
	now := s.now().UTC().Format(time.RFC3339)
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO checkout_promotion_rules
		(id, name, discount_type, discount_value, scope, inventory_item_id, min_subtotal_amount, membership_tier_id, starts_at, ends_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, name, input.DiscountType, input.DiscountValue, input.Scope, checkoutNullableTrim(input.InventoryItemID),
		checkoutNullableTrim(input.MinSubtotalAmount), checkoutNullableTrim(input.MembershipTierID), checkoutNullableTrim(input.StartsAt), checkoutNullableTrim(input.EndsAt), normalizedCheckoutStatus(input.Status), now, now); err != nil {
		return nil, err
	}
	return s.getPromotionRule(ctx, db, id)
}

func (s *CheckoutService) ResolveScanCode(ctx context.Context, code string) (*CheckoutScanResolution, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	coupon, err := scanCheckoutCoupon(db.QueryRowContext(ctx, `SELECT id, code, coupon_type, discount_type, discount_value, value_amount,
		usage_limit, redeemed_count, starts_at, ends_at, status, created_at, updated_at FROM checkout_coupons WHERE code=?`, strings.ToUpper(strings.TrimSpace(code))))
	if errors.Is(err, sql.ErrNoRows) {
		return &CheckoutScanResolution{Type: "not_recognized"}, nil
	}
	if err != nil {
		return nil, err
	}
	return &CheckoutScanResolution{Type: "coupon", Coupon: coupon}, nil
}

func (s *CheckoutService) withMutableTransaction(ctx context.Context, transactionID string, fn func(*sql.Tx, string) error) (*CheckoutTransaction, error) {
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
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM checkout_transactions WHERE id=?`, transactionID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCheckoutNotFound
	} else if err != nil {
		return nil, err
	}
	if status != CheckoutStatusInProgress {
		return nil, ErrCheckoutState
	}
	now := s.now().UTC().Format(time.RFC3339)
	if err := fn(tx, now); err != nil {
		return nil, err
	}
	if err := recalculateCheckoutTotals(ctx, tx, transactionID, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.getTransaction(ctx, db, transactionID)
}

func recalculateCheckoutTotals(ctx context.Context, tx *sql.Tx, transactionID, now string) error {
	var subtotal, discount string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CAST(line_amount AS REAL)),0) FROM checkout_transaction_lines WHERE checkout_transaction_id=?`, transactionID).Scan(&subtotal); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(CAST(amount AS REAL)),0) FROM checkout_discounts WHERE checkout_transaction_id=?`, transactionID).Scan(&discount); err != nil {
		return err
	}
	subtotalRat, err := parseCheckoutAmount(subtotal)
	if err != nil {
		return err
	}
	discountRat, err := parseCheckoutAmount(discount)
	if err != nil {
		return err
	}
	total := new(big.Rat).Sub(subtotalRat, discountRat)
	if total.Sign() < 0 {
		total.SetInt64(0)
	}
	_, err = tx.ExecContext(ctx, `UPDATE checkout_transactions
		SET subtotal_amount=?, discount_amount=?, total_amount=?, updated_at=? WHERE id=?`,
		normalizeRat(subtotalRat), normalizeRat(discountRat), normalizeRat(total), now, transactionID)
	return err
}

func (s *CheckoutService) getTransaction(ctx context.Context, db *sql.DB, id string) (*CheckoutTransaction, error) {
	tx, err := scanCheckoutTransaction(db.QueryRowContext(ctx, `SELECT id, warehouse_id, cashier_user_id, crm_customer_id, status, currency,
		subtotal_amount, discount_amount, total_amount, completed_at, created_at, updated_at FROM checkout_transactions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCheckoutNotFound
	}
	if err != nil {
		return nil, err
	}
	tx.Lines, err = listCheckoutLines(ctx, db, id)
	if err != nil {
		return nil, err
	}
	tx.Discounts, err = listCheckoutDiscounts(ctx, db, id)
	if err != nil {
		return nil, err
	}
	tx.Payments, err = listCheckoutPayments(ctx, db, id)
	return tx, err
}

type checkoutScanner interface{ Scan(...any) error }

func scanCheckoutTransaction(row checkoutScanner) (*CheckoutTransaction, error) {
	var item CheckoutTransaction
	if err := row.Scan(&item.ID, &item.WarehouseID, &item.CashierUserID, &item.CRMCustomerID, &item.Status, &item.Currency,
		&item.SubtotalAmount, &item.DiscountAmount, &item.TotalAmount, &item.CompletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func listCheckoutLines(ctx context.Context, db *sql.DB, transactionID string) ([]CheckoutLine, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, checkout_transaction_id, description, inventory_item_id, quantity, unit_price, line_amount, created_at
		FROM checkout_transaction_lines WHERE checkout_transaction_id=? ORDER BY created_at, id`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CheckoutLine
	for rows.Next() {
		var item CheckoutLine
		if err := rows.Scan(&item.ID, &item.CheckoutTransactionID, &item.Description, &item.InventoryItemID, &item.Quantity, &item.UnitPrice, &item.LineAmount, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func listCheckoutDiscounts(ctx context.Context, db *sql.DB, transactionID string) ([]CheckoutDiscount, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, checkout_transaction_id, source_type, source_reference_id, amount, created_at
		FROM checkout_discounts WHERE checkout_transaction_id=? ORDER BY created_at, id`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CheckoutDiscount
	for rows.Next() {
		var item CheckoutDiscount
		if err := rows.Scan(&item.ID, &item.CheckoutTransactionID, &item.SourceType, &item.SourceReferenceID, &item.Amount, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func listCheckoutPayments(ctx context.Context, db *sql.DB, transactionID string) ([]CheckoutPayment, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, checkout_transaction_id, method, amount, reference, created_at
		FROM checkout_payments WHERE checkout_transaction_id=? ORDER BY created_at, id`, transactionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []CheckoutPayment
	for rows.Next() {
		var item CheckoutPayment
		if err := rows.Scan(&item.ID, &item.CheckoutTransactionID, &item.Method, &item.Amount, &item.Reference, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *CheckoutService) getCoupon(ctx context.Context, db *sql.DB, id string) (*CheckoutCoupon, error) {
	return scanCheckoutCoupon(db.QueryRowContext(ctx, `SELECT id, code, coupon_type, discount_type, discount_value, value_amount,
		usage_limit, redeemed_count, starts_at, ends_at, status, created_at, updated_at FROM checkout_coupons WHERE id=?`, id))
}

func scanCheckoutCoupon(row *sql.Row) (*CheckoutCoupon, error) {
	var item CheckoutCoupon
	if err := row.Scan(&item.ID, &item.Code, &item.CouponType, &item.DiscountType, &item.DiscountValue, &item.ValueAmount,
		&item.UsageLimit, &item.RedeemedCount, &item.StartsAt, &item.EndsAt, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *CheckoutService) getPromotionRule(ctx context.Context, db *sql.DB, id string) (*CheckoutPromotionRule, error) {
	var item CheckoutPromotionRule
	err := db.QueryRowContext(ctx, `SELECT id, name, discount_type, discount_value, scope, inventory_item_id, min_subtotal_amount,
		membership_tier_id, starts_at, ends_at, status, created_at, updated_at FROM checkout_promotion_rules WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.DiscountType, &item.DiscountValue, &item.Scope, &item.InventoryItemID, &item.MinSubtotalAmount,
			&item.MembershipTierID, &item.StartsAt, &item.EndsAt, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	return &item, err
}

func parseCheckoutAmount(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if !checkoutDecimalPattern.MatchString(value) {
		return nil, ErrCheckoutInvalid
	}
	amount, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, ErrCheckoutInvalid
	}
	return amount, nil
}

func normalizeRat(value *big.Rat) string {
	return strings.TrimRight(strings.TrimRight(value.FloatString(6), "0"), ".")
}

func checkoutNullableTrim(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func normalizedCheckoutStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "active"
	}
	return value
}

func validCheckoutPaymentMethod(method string) bool {
	switch method {
	case "cash", "card", "mobile_payment", "voucher", "crypto":
		return true
	default:
		return false
	}
}
