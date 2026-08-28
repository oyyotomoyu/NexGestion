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

var (
	ErrCRMNotFound = errors.New("crm record not found")
	ErrCRMInvalid  = errors.New("invalid crm input")
)

var crmDecimalPattern = regexp.MustCompile(`^\d+(\.\d{1,6})?$`)

type CRMCustomer struct {
	ID            string  `json:"id"`
	PartyType     string  `json:"party_type"`
	Segment       string  `json:"segment"`
	Name          string  `json:"name"`
	ContactEmail  *string `json:"contact_email"`
	ContactPhone  *string `json:"contact_phone"`
	TaxIdentifier *string `json:"tax_identifier"`
	TierID        *string `json:"tier_id"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type CRMCustomerTier struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	DefaultPriceListID *string `json:"default_price_list_id"`
	Status             string  `json:"status"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type CRMMembershipTier struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Description        *string `json:"description"`
	DefaultPriceListID *string `json:"default_price_list_id"`
	Status             string  `json:"status"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type CRMMembership struct {
	ID               string  `json:"id"`
	CustomerID       string  `json:"customer_id"`
	MembershipTierID string  `json:"membership_tier_id"`
	MemberNumber     *string `json:"member_number"`
	JoinedAt         string  `json:"joined_at"`
	ExpiresAt        *string `json:"expires_at"`
	Status           string  `json:"status"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type CRMPriceList struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Currency  string             `json:"currency"`
	Status    string             `json:"status"`
	CreatedAt string             `json:"created_at"`
	UpdatedAt string             `json:"updated_at"`
	Items     []CRMPriceListItem `json:"items"`
}

type CRMPriceListItem struct {
	ID              string  `json:"id"`
	PriceListID     string  `json:"price_list_id"`
	Description     string  `json:"description"`
	InventoryItemID *string `json:"inventory_item_id"`
	UnitPrice       string  `json:"unit_price"`
	CreatedAt       string  `json:"created_at"`
}

type CRMPointsEarningRule struct {
	ID                    string  `json:"id"`
	MembershipTierID      *string `json:"membership_tier_id"`
	PointsPerCurrencyUnit string  `json:"points_per_currency_unit"`
	Status                string  `json:"status"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

type CRMPointsLedgerEntry struct {
	ID                string  `json:"id"`
	CustomerID        string  `json:"customer_id"`
	PointsDelta       int     `json:"points_delta"`
	EntryType         string  `json:"entry_type"`
	SourceModule      *string `json:"source_module"`
	SourceReferenceID *string `json:"source_reference_id"`
	OccurredAt        string  `json:"occurred_at"`
}

type CRMPointsBalance struct {
	CustomerID string `json:"customer_id"`
	Balance    int    `json:"balance"`
}

type CreateCRMCustomerInput struct {
	PartyType     string `json:"party_type"`
	Segment       string `json:"segment"`
	Name          string `json:"name"`
	ContactEmail  string `json:"contact_email"`
	ContactPhone  string `json:"contact_phone"`
	TaxIdentifier string `json:"tax_identifier"`
	TierID        string `json:"tier_id"`
	Status        string `json:"status"`
}

type UpdateCRMCustomerInput struct {
	Name          *string `json:"name"`
	ContactEmail  *string `json:"contact_email"`
	ContactPhone  *string `json:"contact_phone"`
	TaxIdentifier *string `json:"tax_identifier"`
	TierID        *string `json:"tier_id"`
	Status        *string `json:"status"`
}

type CreateCRMCustomerTierInput struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultPriceListID string `json:"default_price_list_id"`
	Status             string `json:"status"`
}

type UpdateCRMCustomerTierInput struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	DefaultPriceListID *string `json:"default_price_list_id"`
	Status             *string `json:"status"`
}

type CreateCRMMembershipTierInput struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultPriceListID string `json:"default_price_list_id"`
	Status             string `json:"status"`
}

type UpdateCRMMembershipTierInput struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	DefaultPriceListID *string `json:"default_price_list_id"`
	Status             *string `json:"status"`
}

type CreateCRMMembershipInput struct {
	CustomerID       string `json:"customer_id"`
	MembershipTierID string `json:"membership_tier_id"`
	MemberNumber     string `json:"member_number"`
	JoinedAt         string `json:"joined_at"`
	ExpiresAt        string `json:"expires_at"`
	Status           string `json:"status"`
}

type UpdateCRMMembershipInput struct {
	MembershipTierID *string `json:"membership_tier_id"`
	MemberNumber     *string `json:"member_number"`
	ExpiresAt        *string `json:"expires_at"`
	Status           *string `json:"status"`
}

type CreateCRMPriceListInput struct {
	Name     string `json:"name"`
	Currency string `json:"currency"`
	Status   string `json:"status"`
}

type UpdateCRMPriceListInput struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

type AddCRMPriceListItemInput struct {
	Description     string `json:"description"`
	InventoryItemID string `json:"inventory_item_id"`
	UnitPrice       string `json:"unit_price"`
}

type CreateCRMPointsEarningRuleInput struct {
	MembershipTierID      string `json:"membership_tier_id"`
	PointsPerCurrencyUnit string `json:"points_per_currency_unit"`
	Status                string `json:"status"`
}

type UpdateCRMPointsEarningRuleInput struct {
	PointsPerCurrencyUnit *string `json:"points_per_currency_unit"`
	Status                *string `json:"status"`
}

type PostCRMPointsLedgerEntryInput struct {
	CustomerID        string `json:"customer_id"`
	PointsDelta       int    `json:"points_delta"`
	EntryType         string `json:"entry_type"`
	SourceModule      string `json:"source_module"`
	SourceReferenceID string `json:"source_reference_id"`
}

type CRMService struct {
	databasePath string
	now          func() time.Time
}

func NewCRMService(databaseDirectory string) *CRMService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &CRMService{
		databasePath: filepath.Join(databaseDirectory, "crm.db"),
		now:          time.Now,
	}
}

func (s *CRMService) open() (*sql.DB, error) {
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

// --- Customers ---

func (s *CRMService) CreateCustomer(ctx context.Context, input CreateCRMCustomerInput) (*CRMCustomer, error) {
	partyType := strings.TrimSpace(input.PartyType)
	segment := strings.TrimSpace(input.Segment)
	name := strings.TrimSpace(input.Name)
	if (partyType != "individual" && partyType != "organization") || (segment != "b2b" && segment != "b2c") || name == "" {
		return nil, ErrCRMInvalid
	}
	status := normalizedCRMStatus(input.Status)
	if !crmStatusAllowed(status, "active", "inactive") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tierID, err := s.resolveOptionalCustomerTier(ctx, db, input.TierID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_customers
		(id, party_type, segment, name, contact_email, contact_phone, tax_identifier, tier_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, partyType, segment, name, nullableTrim(input.ContactEmail), nullableTrim(input.ContactPhone),
		nullableTrim(input.TaxIdentifier), tierID, status, now, now); err != nil {
		return nil, err
	}
	return s.getCustomer(ctx, db, id)
}

func (s *CRMService) GetCustomer(ctx context.Context, id string) (*CRMCustomer, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getCustomer(ctx, db, id)
}

func (s *CRMService) ListCustomers(ctx context.Context, query ListQuery) (ListResult[CRMCustomer], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{
			"created_at": "created_at",
			"updated_at": "updated_at",
			"name":       "name",
		})
	if err != nil {
		return ListResult[CRMCustomer]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMCustomer]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "(name LIKE ? OR contact_email LIKE ? OR contact_phone LIKE ?)")
		keyword := "%" + normalized.Keyword + "%"
		args = append(args, keyword, keyword, keyword)
	}
	for key, value := range normalized.Filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "status", "segment", "party_type", "tier_id":
			whereParts = append(whereParts, key+" = ?")
			args = append(args, value)
		default:
			return ListResult[CRMCustomer]{}, ErrInvalidListQuery
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_customers"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMCustomer]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, party_type, segment, name, contact_email, contact_phone, tax_identifier, tier_id, status, created_at, updated_at
		FROM crm_customers`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMCustomer]{}, err
	}
	defer rows.Close()
	items := []CRMCustomer{}
	for rows.Next() {
		item, err := scanCRMCustomer(rows)
		if err != nil {
			return ListResult[CRMCustomer]{}, err
		}
		items = append(items, *item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdateCustomer(ctx context.Context, id string, input UpdateCRMCustomerInput) (*CRMCustomer, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_customers WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrCRMInvalid)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if input.ContactEmail != nil {
		sets = append(sets, "contact_email = ?")
		args = append(args, nullableTrim(*input.ContactEmail))
	}
	if input.ContactPhone != nil {
		sets = append(sets, "contact_phone = ?")
		args = append(args, nullableTrim(*input.ContactPhone))
	}
	if input.TaxIdentifier != nil {
		sets = append(sets, "tax_identifier = ?")
		args = append(args, nullableTrim(*input.TaxIdentifier))
	}
	if input.TierID != nil {
		tierID, err := s.resolveOptionalCustomerTier(ctx, db, *input.TierID)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "tier_id = ?")
		args = append(args, tierID)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "inactive") {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getCustomer(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_customers SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return nil, err
	}
	return s.getCustomer(ctx, db, id)
}

func (s *CRMService) resolveOptionalCustomerTier(ctx context.Context, db *sql.DB, rawTierID string) (any, error) {
	tierID := strings.TrimSpace(rawTierID)
	if tierID == "" {
		return nil, nil
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_customer_tiers WHERE id=?`, tierID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, fmt.Errorf("%w: tier_id does not exist", ErrCRMInvalid)
	}
	return tierID, nil
}

func (s *CRMService) getCustomer(ctx context.Context, db *sql.DB, id string) (*CRMCustomer, error) {
	item, err := scanCRMCustomer(db.QueryRowContext(ctx, `SELECT id, party_type, segment, name, contact_email, contact_phone, tax_identifier, tier_id, status, created_at, updated_at
		FROM crm_customers WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	return item, err
}

type crmScanner interface{ Scan(...any) error }

func scanCRMCustomer(row crmScanner) (*CRMCustomer, error) {
	var item CRMCustomer
	if err := row.Scan(&item.ID, &item.PartyType, &item.Segment, &item.Name, &item.ContactEmail, &item.ContactPhone,
		&item.TaxIdentifier, &item.TierID, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

// --- Customer Tiers ---

func (s *CRMService) CreateCustomerTier(ctx context.Context, input CreateCRMCustomerTierInput) (*CRMCustomerTier, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrCRMInvalid
	}
	status := normalizedCRMStatus(input.Status)
	if !crmStatusAllowed(status, "active", "inactive") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	priceListID, err := s.resolveOptionalPriceList(ctx, db, input.DefaultPriceListID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_customer_tiers
		(id, name, description, default_price_list_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, nullableTrim(input.Description), priceListID, status, now, now); err != nil {
		return nil, err
	}
	return s.getCustomerTier(ctx, db, id)
}

func (s *CRMService) GetCustomerTier(ctx context.Context, id string) (*CRMCustomerTier, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getCustomerTier(ctx, db, id)
}

func (s *CRMService) ListCustomerTiers(ctx context.Context, query ListQuery) (ListResult[CRMCustomerTier], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{"created_at": "created_at", "updated_at": "updated_at", "name": "name"})
	if err != nil {
		return ListResult[CRMCustomerTier]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMCustomerTier]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "name LIKE ?")
		args = append(args, "%"+normalized.Keyword+"%")
	}
	if status := strings.TrimSpace(normalized.Filters["status"]); status != "" {
		whereParts = append(whereParts, "status = ?")
		args = append(args, status)
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_customer_tiers"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMCustomerTier]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, description, default_price_list_id, status, created_at, updated_at
		FROM crm_customer_tiers`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMCustomerTier]{}, err
	}
	defer rows.Close()
	items := []CRMCustomerTier{}
	for rows.Next() {
		var item CRMCustomerTier
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.DefaultPriceListID, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ListResult[CRMCustomerTier]{}, err
		}
		items = append(items, item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdateCustomerTier(ctx context.Context, id string, input UpdateCRMCustomerTierInput) (*CRMCustomerTier, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_customer_tiers WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrCRMInvalid)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, nullableTrim(*input.Description))
	}
	if input.DefaultPriceListID != nil {
		priceListID, err := s.resolveOptionalPriceList(ctx, db, *input.DefaultPriceListID)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "default_price_list_id = ?")
		args = append(args, priceListID)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "inactive") {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getCustomerTier(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_customer_tiers SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return nil, err
	}
	return s.getCustomerTier(ctx, db, id)
}

func (s *CRMService) getCustomerTier(ctx context.Context, db *sql.DB, id string) (*CRMCustomerTier, error) {
	var item CRMCustomerTier
	err := db.QueryRowContext(ctx, `SELECT id, name, description, default_price_list_id, status, created_at, updated_at
		FROM crm_customer_tiers WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.Description, &item.DefaultPriceListID, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// --- Membership Tiers ---

func (s *CRMService) CreateMembershipTier(ctx context.Context, input CreateCRMMembershipTierInput) (*CRMMembershipTier, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrCRMInvalid
	}
	status := normalizedCRMStatus(input.Status)
	if !crmStatusAllowed(status, "active", "inactive") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	priceListID, err := s.resolveOptionalPriceList(ctx, db, input.DefaultPriceListID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_membership_tiers
		(id, name, description, default_price_list_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, nullableTrim(input.Description), priceListID, status, now, now); err != nil {
		return nil, err
	}
	return s.getMembershipTier(ctx, db, id)
}

func (s *CRMService) GetMembershipTier(ctx context.Context, id string) (*CRMMembershipTier, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getMembershipTier(ctx, db, id)
}

func (s *CRMService) ListMembershipTiers(ctx context.Context, query ListQuery) (ListResult[CRMMembershipTier], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{"created_at": "created_at", "updated_at": "updated_at", "name": "name"})
	if err != nil {
		return ListResult[CRMMembershipTier]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMMembershipTier]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "name LIKE ?")
		args = append(args, "%"+normalized.Keyword+"%")
	}
	if status := strings.TrimSpace(normalized.Filters["status"]); status != "" {
		whereParts = append(whereParts, "status = ?")
		args = append(args, status)
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_membership_tiers"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMMembershipTier]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, description, default_price_list_id, status, created_at, updated_at
		FROM crm_membership_tiers`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMMembershipTier]{}, err
	}
	defer rows.Close()
	items := []CRMMembershipTier{}
	for rows.Next() {
		var item CRMMembershipTier
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.DefaultPriceListID, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ListResult[CRMMembershipTier]{}, err
		}
		items = append(items, item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdateMembershipTier(ctx context.Context, id string, input UpdateCRMMembershipTierInput) (*CRMMembershipTier, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_membership_tiers WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrCRMInvalid)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if input.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, nullableTrim(*input.Description))
	}
	if input.DefaultPriceListID != nil {
		priceListID, err := s.resolveOptionalPriceList(ctx, db, *input.DefaultPriceListID)
		if err != nil {
			return nil, err
		}
		sets = append(sets, "default_price_list_id = ?")
		args = append(args, priceListID)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "inactive") {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getMembershipTier(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_membership_tiers SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return nil, err
	}
	return s.getMembershipTier(ctx, db, id)
}

func (s *CRMService) getMembershipTier(ctx context.Context, db *sql.DB, id string) (*CRMMembershipTier, error) {
	var item CRMMembershipTier
	err := db.QueryRowContext(ctx, `SELECT id, name, description, default_price_list_id, status, created_at, updated_at
		FROM crm_membership_tiers WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.Description, &item.DefaultPriceListID, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// --- Memberships ---

func (s *CRMService) CreateMembership(ctx context.Context, input CreateCRMMembershipInput) (*CRMMembership, error) {
	customerID := strings.TrimSpace(input.CustomerID)
	membershipTierID := strings.TrimSpace(input.MembershipTierID)
	joinedAt := strings.TrimSpace(input.JoinedAt)
	if customerID == "" || membershipTierID == "" || joinedAt == "" {
		return nil, ErrCRMInvalid
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "active"
	}
	if !crmStatusAllowed(status, "active", "lapsed", "cancelled") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := s.getCustomer(ctx, db, customerID); err != nil {
		return nil, err
	}
	if _, err := s.getMembershipTier(ctx, db, membershipTierID); err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_memberships
		(id, customer_id, membership_tier_id, member_number, joined_at, expires_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, customerID, membershipTierID, nullableTrim(input.MemberNumber), joinedAt, nullableTrim(input.ExpiresAt), status, now, now); err != nil {
		if isCRMUniqueConstraintError(err) {
			return nil, fmt.Errorf("%w: member_number already in use", ErrCRMInvalid)
		}
		return nil, err
	}
	return s.getMembership(ctx, db, id)
}

func (s *CRMService) GetMembership(ctx context.Context, id string) (*CRMMembership, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getMembership(ctx, db, id)
}

func (s *CRMService) ResolveMemberNumber(ctx context.Context, memberNumber string) (*CRMMembership, error) {
	memberNumber = strings.TrimSpace(memberNumber)
	if memberNumber == "" {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	item, err := scanCRMMembership(db.QueryRowContext(ctx, `SELECT id, customer_id, membership_tier_id, member_number, joined_at, expires_at, status, created_at, updated_at
		FROM crm_memberships WHERE member_number=?`, memberNumber))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	return item, err
}

func (s *CRMService) ListMemberships(ctx context.Context, query ListQuery) (ListResult[CRMMembership], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{"created_at": "created_at", "updated_at": "updated_at", "joined_at": "joined_at"})
	if err != nil {
		return ListResult[CRMMembership]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMMembership]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "member_number LIKE ?")
		args = append(args, "%"+normalized.Keyword+"%")
	}
	for key, value := range normalized.Filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "status", "customer_id", "membership_tier_id":
			whereParts = append(whereParts, key+" = ?")
			args = append(args, value)
		default:
			return ListResult[CRMMembership]{}, ErrInvalidListQuery
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_memberships"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMMembership]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, customer_id, membership_tier_id, member_number, joined_at, expires_at, status, created_at, updated_at
		FROM crm_memberships`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMMembership]{}, err
	}
	defer rows.Close()
	items := []CRMMembership{}
	for rows.Next() {
		item, err := scanCRMMembership(rows)
		if err != nil {
			return ListResult[CRMMembership]{}, err
		}
		items = append(items, *item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdateMembership(ctx context.Context, id string, input UpdateCRMMembershipInput) (*CRMMembership, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_memberships WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.MembershipTierID != nil {
		membershipTierID := strings.TrimSpace(*input.MembershipTierID)
		if _, err := s.getMembershipTier(ctx, db, membershipTierID); err != nil {
			return nil, err
		}
		sets = append(sets, "membership_tier_id = ?")
		args = append(args, membershipTierID)
	}
	if input.MemberNumber != nil {
		sets = append(sets, "member_number = ?")
		args = append(args, nullableTrim(*input.MemberNumber))
	}
	if input.ExpiresAt != nil {
		sets = append(sets, "expires_at = ?")
		args = append(args, nullableTrim(*input.ExpiresAt))
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "lapsed", "cancelled") {
			return nil, fmt.Errorf("%w: status must be active, lapsed, or cancelled", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getMembership(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_memberships SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		if isCRMUniqueConstraintError(err) {
			return nil, fmt.Errorf("%w: member_number already in use", ErrCRMInvalid)
		}
		return nil, err
	}
	return s.getMembership(ctx, db, id)
}

func (s *CRMService) getMembership(ctx context.Context, db *sql.DB, id string) (*CRMMembership, error) {
	item, err := scanCRMMembership(db.QueryRowContext(ctx, `SELECT id, customer_id, membership_tier_id, member_number, joined_at, expires_at, status, created_at, updated_at
		FROM crm_memberships WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	return item, err
}

func scanCRMMembership(row crmScanner) (*CRMMembership, error) {
	var item CRMMembership
	if err := row.Scan(&item.ID, &item.CustomerID, &item.MembershipTierID, &item.MemberNumber, &item.JoinedAt,
		&item.ExpiresAt, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	return &item, nil
}

// --- Price Lists ---

func (s *CRMService) CreatePriceList(ctx context.Context, input CreateCRMPriceListInput) (*CRMPriceList, error) {
	name := strings.TrimSpace(input.Name)
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if name == "" || !currencyPattern.MatchString(currency) {
		return nil, ErrCRMInvalid
	}
	status := normalizedCRMStatus(input.Status)
	if !crmStatusAllowed(status, "active", "inactive") {
		return nil, ErrCRMInvalid
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_price_lists (id, name, currency, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, name, currency, status, now, now); err != nil {
		return nil, err
	}
	return s.getPriceList(ctx, db, id)
}

func (s *CRMService) GetPriceList(ctx context.Context, id string) (*CRMPriceList, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getPriceList(ctx, db, id)
}

func (s *CRMService) ListPriceLists(ctx context.Context, query ListQuery) (ListResult[CRMPriceList], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{"created_at": "created_at", "updated_at": "updated_at", "name": "name"})
	if err != nil {
		return ListResult[CRMPriceList]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMPriceList]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	if normalized.Keyword != "" {
		whereParts = append(whereParts, "name LIKE ?")
		args = append(args, "%"+normalized.Keyword+"%")
	}
	if status := strings.TrimSpace(normalized.Filters["status"]); status != "" {
		whereParts = append(whereParts, "status = ?")
		args = append(args, status)
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_price_lists"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMPriceList]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, currency, status, created_at, updated_at
		FROM crm_price_lists`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMPriceList]{}, err
	}
	defer rows.Close()
	items := []CRMPriceList{}
	for rows.Next() {
		var item CRMPriceList
		if err := rows.Scan(&item.ID, &item.Name, &item.Currency, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ListResult[CRMPriceList]{}, err
		}
		items = append(items, item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdatePriceList(ctx context.Context, id string, input UpdateCRMPriceListInput) (*CRMPriceList, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_price_lists WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrCRMInvalid)
		}
		sets = append(sets, "name = ?")
		args = append(args, name)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "inactive") {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getPriceList(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_price_lists SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return nil, err
	}
	return s.getPriceList(ctx, db, id)
}

func (s *CRMService) AddPriceListItem(ctx context.Context, priceListID string, input AddCRMPriceListItemInput) (*CRMPriceList, error) {
	description := strings.TrimSpace(input.Description)
	unitPrice := strings.TrimSpace(input.UnitPrice)
	if description == "" || !crmDecimalPattern.MatchString(unitPrice) {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_price_lists WHERE id=?`, priceListID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	now := s.now().UTC().Format(time.RFC3339)
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_price_list_items
		(id, price_list_id, description, inventory_item_id, unit_price, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, uuid.NewString(), priceListID, description, nullableTrim(input.InventoryItemID), unitPrice, now); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `UPDATE crm_price_lists SET updated_at=? WHERE id=?`, now, priceListID); err != nil {
		return nil, err
	}
	return s.getPriceList(ctx, db, priceListID)
}

func (s *CRMService) resolveOptionalPriceList(ctx context.Context, db *sql.DB, rawPriceListID string) (any, error) {
	priceListID := strings.TrimSpace(rawPriceListID)
	if priceListID == "" {
		return nil, nil
	}
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_price_lists WHERE id=?`, priceListID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, fmt.Errorf("%w: default_price_list_id does not exist", ErrCRMInvalid)
	}
	return priceListID, nil
}

func (s *CRMService) getPriceList(ctx context.Context, db *sql.DB, id string) (*CRMPriceList, error) {
	var item CRMPriceList
	err := db.QueryRowContext(ctx, `SELECT id, name, currency, status, created_at, updated_at FROM crm_price_lists WHERE id=?`, id).
		Scan(&item.ID, &item.Name, &item.Currency, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	if err != nil {
		return nil, err
	}
	item.Items, err = listCRMPriceListItems(ctx, db, id)
	return &item, err
}

func listCRMPriceListItems(ctx context.Context, db *sql.DB, priceListID string) ([]CRMPriceListItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, price_list_id, description, inventory_item_id, unit_price, created_at
		FROM crm_price_list_items WHERE price_list_id=? ORDER BY created_at, id`, priceListID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []CRMPriceListItem{}
	for rows.Next() {
		var item CRMPriceListItem
		if err := rows.Scan(&item.ID, &item.PriceListID, &item.Description, &item.InventoryItemID, &item.UnitPrice, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// --- Points Earning Rules ---

func (s *CRMService) CreatePointsEarningRule(ctx context.Context, input CreateCRMPointsEarningRuleInput) (*CRMPointsEarningRule, error) {
	rate := strings.TrimSpace(input.PointsPerCurrencyUnit)
	if !crmDecimalPattern.MatchString(rate) {
		return nil, ErrCRMInvalid
	}
	status := normalizedCRMStatus(input.Status)
	if !crmStatusAllowed(status, "active", "inactive") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	membershipTierID := strings.TrimSpace(input.MembershipTierID)
	if membershipTierID != "" {
		if _, err := s.getMembershipTier(ctx, db, membershipTierID); err != nil {
			return nil, err
		}
	} else {
		membershipTierID = ""
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_points_earning_rules
		(id, membership_tier_id, points_per_currency_unit, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, nullableTrim(membershipTierID), rate, status, now, now); err != nil {
		return nil, err
	}
	return s.getPointsEarningRule(ctx, db, id)
}

func (s *CRMService) GetPointsEarningRule(ctx context.Context, id string) (*CRMPointsEarningRule, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return s.getPointsEarningRule(ctx, db, id)
}

func (s *CRMService) ListPointsEarningRules(ctx context.Context, query ListQuery) (ListResult[CRMPointsEarningRule], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "created_at", "desc",
		map[string]string{"created_at": "created_at", "updated_at": "updated_at"})
	if err != nil {
		return ListResult[CRMPointsEarningRule]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMPointsEarningRule]{}, err
	}
	defer db.Close()
	whereParts := []string{}
	args := []any{}
	for key, value := range normalized.Filters {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "status", "membership_tier_id":
			whereParts = append(whereParts, key+" = ?")
			args = append(args, value)
		default:
			return ListResult[CRMPointsEarningRule]{}, ErrInvalidListQuery
		}
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_points_earning_rules"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMPointsEarningRule]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, membership_tier_id, points_per_currency_unit, status, created_at, updated_at
		FROM crm_points_earning_rules`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMPointsEarningRule]{}, err
	}
	defer rows.Close()
	items := []CRMPointsEarningRule{}
	for rows.Next() {
		var item CRMPointsEarningRule
		if err := rows.Scan(&item.ID, &item.MembershipTierID, &item.PointsPerCurrencyUnit, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return ListResult[CRMPointsEarningRule]{}, err
		}
		items = append(items, item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

func (s *CRMService) UpdatePointsEarningRule(ctx context.Context, id string, input UpdateCRMPointsEarningRuleInput) (*CRMPointsEarningRule, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crm_points_earning_rules WHERE id=?`, id).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrCRMNotFound
	}
	sets := []string{}
	args := []any{}
	if input.PointsPerCurrencyUnit != nil {
		rate := strings.TrimSpace(*input.PointsPerCurrencyUnit)
		if !crmDecimalPattern.MatchString(rate) {
			return nil, fmt.Errorf("%w: points_per_currency_unit must be a non-negative decimal", ErrCRMInvalid)
		}
		sets = append(sets, "points_per_currency_unit = ?")
		args = append(args, rate)
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !crmStatusAllowed(status, "active", "inactive") {
			return nil, fmt.Errorf("%w: status must be active or inactive", ErrCRMInvalid)
		}
		sets = append(sets, "status = ?")
		args = append(args, status)
	}
	if len(sets) == 0 {
		return s.getPointsEarningRule(ctx, db, id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, s.now().UTC().Format(time.RFC3339))
	args = append(args, id)
	if _, err := db.ExecContext(ctx, `UPDATE crm_points_earning_rules SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return nil, err
	}
	return s.getPointsEarningRule(ctx, db, id)
}

func (s *CRMService) getPointsEarningRule(ctx context.Context, db *sql.DB, id string) (*CRMPointsEarningRule, error) {
	var item CRMPointsEarningRule
	err := db.QueryRowContext(ctx, `SELECT id, membership_tier_id, points_per_currency_unit, status, created_at, updated_at
		FROM crm_points_earning_rules WHERE id=?`, id).
		Scan(&item.ID, &item.MembershipTierID, &item.PointsPerCurrencyUnit, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCRMNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// --- Loyalty Points Ledger ---

func (s *CRMService) PostPointsLedgerEntry(ctx context.Context, input PostCRMPointsLedgerEntryInput) (*CRMPointsLedgerEntry, error) {
	customerID := strings.TrimSpace(input.CustomerID)
	entryType := strings.TrimSpace(input.EntryType)
	if customerID == "" || input.PointsDelta == 0 || !crmStatusAllowed(entryType, "earned", "redeemed", "expired", "adjustment") {
		return nil, ErrCRMInvalid
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := s.getCustomer(ctx, db, customerID); err != nil {
		return nil, err
	}
	now := s.now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO crm_points_ledger
		(id, customer_id, points_delta, entry_type, source_module, source_reference_id, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, customerID, input.PointsDelta, entryType, nullableTrim(input.SourceModule), nullableTrim(input.SourceReferenceID), now); err != nil {
		return nil, err
	}
	var item CRMPointsLedgerEntry
	err = db.QueryRowContext(ctx, `SELECT id, customer_id, points_delta, entry_type, source_module, source_reference_id, occurred_at
		FROM crm_points_ledger WHERE id=?`, id).
		Scan(&item.ID, &item.CustomerID, &item.PointsDelta, &item.EntryType, &item.SourceModule, &item.SourceReferenceID, &item.OccurredAt)
	return &item, err
}

func (s *CRMService) GetPointsBalance(ctx context.Context, customerID string) (*CRMPointsBalance, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := s.getCustomer(ctx, db, customerID); err != nil {
		return nil, err
	}
	var balance int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(points_delta), 0) FROM crm_points_ledger WHERE customer_id=?`, customerID).Scan(&balance); err != nil {
		return nil, err
	}
	return &CRMPointsBalance{CustomerID: customerID, Balance: balance}, nil
}

func (s *CRMService) ListPointsLedger(ctx context.Context, customerID string, query ListQuery) (ListResult[CRMPointsLedgerEntry], error) {
	normalized, sortExpression, err := NormalizeListQuery(query, "occurred_at", "desc",
		map[string]string{"occurred_at": "occurred_at"})
	if err != nil {
		return ListResult[CRMPointsLedgerEntry]{}, err
	}
	db, err := s.open()
	if err != nil {
		return ListResult[CRMPointsLedgerEntry]{}, err
	}
	defer db.Close()
	if _, err := s.getCustomer(ctx, db, customerID); err != nil {
		return ListResult[CRMPointsLedgerEntry]{}, err
	}
	whereParts := []string{"customer_id = ?"}
	args := []any{customerID}
	if entryType := strings.TrimSpace(normalized.Filters["entry_type"]); entryType != "" {
		whereParts = append(whereParts, "entry_type = ?")
		args = append(args, entryType)
	}
	where := " WHERE " + strings.Join(whereParts, " AND ")
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crm_points_ledger"+where, args...).Scan(&total); err != nil {
		return ListResult[CRMPointsLedgerEntry]{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, customer_id, points_delta, entry_type, source_module, source_reference_id, occurred_at
		FROM crm_points_ledger`+where+` ORDER BY `+sortExpression+` `+normalized.Order+` LIMIT ? OFFSET ?`,
		append(args, normalized.PageSize, ListOffset(normalized))...)
	if err != nil {
		return ListResult[CRMPointsLedgerEntry]{}, err
	}
	defer rows.Close()
	items := []CRMPointsLedgerEntry{}
	for rows.Next() {
		var item CRMPointsLedgerEntry
		if err := rows.Scan(&item.ID, &item.CustomerID, &item.PointsDelta, &item.EntryType, &item.SourceModule, &item.SourceReferenceID, &item.OccurredAt); err != nil {
			return ListResult[CRMPointsLedgerEntry]{}, err
		}
		items = append(items, item)
	}
	return NewListResult(items, normalized, total), rows.Err()
}

// --- Shared helpers ---

func normalizedCRMStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "active"
	}
	return value
}

func crmStatusAllowed(status string, allowed ...string) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func isCRMUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
