package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrOperationInvalid = errors.New("invalid operations input")
var ErrOperationNotFound = errors.New("operations record not found")
var ErrOperationStale = errors.New("document changed; reload before retrying")
var ErrOperationState = errors.New("invalid operations state or duplicate request")

type OperationLine struct {
	FulfilledQuantity   string `json:"fulfilled_quantity,omitempty"`
	RemainingQuantity   string `json:"remaining_quantity,omitempty"`
	ID                  string `json:"id"`
	Description         string `json:"description,omitempty"`
	InventoryItemID     string `json:"inventory_item_id,omitempty"`
	LocationID          string `json:"location_id,omitempty"`
	Quantity            string `json:"quantity,omitempty"`
	UnitPrice           string `json:"unit_price,omitempty"`
	QuantityUsed        string `json:"quantity_used,omitempty"`
	OrderLineID         string `json:"order_line_id,omitempty"`
	PurchaseOrderLineID string `json:"purchase_order_line_id,omitempty"`
	QuantityReceived    string `json:"quantity_received,omitempty"`
	QuantityAccepted    string `json:"quantity_accepted,omitempty"`
	QuantityRejected    string `json:"quantity_rejected,omitempty"`
	RejectionReason     string `json:"rejection_reason,omitempty"`
}

// OperationDocument is the stored snapshot. Kind-specific validation owns every transition.
type OperationDocument struct {
	ExpectedUpdatedAt     string          `json:"expected_updated_at,omitempty"`
	ID                    string          `json:"id"`
	Kind                  string          `json:"kind"`
	Status                string          `json:"status"`
	Name                  string          `json:"name,omitempty"`
	ContactEmail          string          `json:"contact_email,omitempty"`
	ContactPhone          string          `json:"contact_phone,omitempty"`
	PaymentTerms          string          `json:"payment_terms,omitempty"`
	TaxIdentifier         string          `json:"tax_identifier,omitempty"`
	CustomerID            string          `json:"customer_id,omitempty"`
	CustomerName          string          `json:"customer_name,omitempty"`
	VendorID              string          `json:"vendor_id,omitempty"`
	VendorName            string          `json:"vendor_name,omitempty"`
	QuoteDate             string          `json:"quote_date,omitempty"`
	ValidUntil            string          `json:"valid_until,omitempty"`
	OrderDate             string          `json:"order_date,omitempty"`
	Currency              string          `json:"currency,omitempty"`
	TotalAmount           string          `json:"total_amount,omitempty"`
	SourceQuoteID         string          `json:"source_quote_id,omitempty"`
	ResultingOrderID      string          `json:"resulting_order_id,omitempty"`
	PurchaseOrderID       string          `json:"purchase_order_id,omitempty"`
	OrderID               string          `json:"order_id,omitempty"`
	ReceivedDate          string          `json:"received_date,omitempty"`
	ReceivedByUserID      string          `json:"received_by_user_id,omitempty"`
	InspectedByUserID     string          `json:"inspected_by_user_id,omitempty"`
	InspectedAt           string          `json:"inspected_at,omitempty"`
	InvoiceDate           string          `json:"invoice_date,omitempty"`
	InvoiceAmount         string          `json:"invoice_amount,omitempty"`
	VendorInvoiceNumber   string          `json:"vendor_invoice_number,omitempty"`
	MatchStatus           string          `json:"match_status,omitempty"`
	FullyReceived         bool            `json:"fully_received,omitempty"`
	WarehouseID           string          `json:"warehouse_id,omitempty"`
	LocationID            string          `json:"location_id,omitempty"`
	ShippedDate           string          `json:"shipped_date,omitempty"`
	TrackingReference     string          `json:"tracking_reference,omitempty"`
	Note                  string          `json:"note,omitempty"`
	OutputDescription     string          `json:"output_description,omitempty"`
	OutputInventoryItemID string          `json:"output_inventory_item_id,omitempty"`
	OutputQuantity        string          `json:"output_quantity,omitempty"`
	PlannedDate           string          `json:"planned_date,omitempty"`
	CompletedAt           string          `json:"completed_at,omitempty"`
	Materials             []OperationLine `json:"materials,omitempty"`
	Lines                 []OperationLine `json:"lines,omitempty"`
	InventoryRecordID     string          `json:"inventory_record_id,omitempty"`
	RequestKey            string          `json:"request_key,omitempty"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
	CreatedBy             string          `json:"created_by"`
}

type OperationsService struct {
	directory string
	module    string
	now       func() time.Time
}

func NewOrderService(directory string) *OperationsService {
	return newOperationsService(directory, "order")
}
func NewProcurementService(directory string) *OperationsService {
	return newOperationsService(directory, "procurement")
}
func NewProductionService(directory string) *OperationsService {
	return newOperationsService(directory, "production")
}
func newOperationsService(directory, module string) *OperationsService {
	if directory == "" {
		directory = defaultDatabaseDirectory
	}
	return &OperationsService{directory, module, time.Now}
}

// ATTACH keeps module and inventory writes in one SQLite transaction. A missing
// optional module is never created by opening another module.
func openOperationsDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func attachInventory(ctx context.Context, db *sql.DB, directory string) (bool, error) {
	path := filepath.Join(directory, "inventory.db")
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = db.ExecContext(ctx, `ATTACH DATABASE ? AS inventory`, path)
	return err == nil, err
}
func (s *OperationsService) open(ctx context.Context) (*sql.DB, bool, error) {
	db, err := openOperationsDB(filepath.Join(s.directory, s.module+".db"))
	if err != nil {
		return nil, false, err
	}
	available, err := attachInventory(ctx, db, s.directory)
	if err != nil {
		db.Close()
		return nil, false, err
	}
	return db, available, nil
}
func (s *OperationsService) allowed(kind string) bool {
	switch s.module {
	case "order":
		return kind == "quote" || kind == "order" || kind == "shipment"
	case "procurement":
		return kind == "vendor" || kind == "purchase_order" || kind == "goods_receipt" || kind == "vendor_invoice"
	case "production":
		return kind == "production_order"
	}
	return false
}
func operationDate(value string, optional bool) error {
	if optional && value == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return ErrOperationInvalid
	}
	return nil
}
func operationDecimal(value string, positive bool) (*big.Rat, error) {
	if len(value) > 40 {
		return nil, ErrOperationInvalid
	}
	r, err := parseCheckoutAmount(value)
	if err != nil || positive && r.Sign() <= 0 || !positive && r.Sign() < 0 {
		return nil, ErrOperationInvalid
	}
	return r, nil
}
func operationQuantity(value string, positive bool) (*big.Rat, error) {
	q, err := inventoryQuantity(value)
	if err != nil || q < 0 || positive && q == 0 {
		return nil, ErrOperationInvalid
	}
	return big.NewRat(q, 1000000), nil
}
func operationMoney(r *big.Rat) string { return normalizeRat(r) }
func normalizeOperationLines(lines []OperationLine) ([]OperationLine, string, error) {
	if len(lines) == 0 || len(lines) > 1000 {
		return nil, "", ErrOperationInvalid
	}
	total := new(big.Rat)
	for i := range lines {
		line := &lines[i]
		line.ID = uuid.NewString()
		line.FulfilledQuantity = ""
		line.RemainingQuantity = ""
		line.Description = strings.TrimSpace(line.Description)
		if line.Description == "" {
			return nil, "", ErrOperationInvalid
		}
		q, err := operationQuantity(line.Quantity, true)
		if err != nil {
			return nil, "", err
		}
		p, err := operationDecimal(line.UnitPrice, false)
		if err != nil {
			return nil, "", err
		}
		line.Quantity = operationMoney(q)
		line.UnitPrice = operationMoney(p)
		total.Add(total, new(big.Rat).Mul(q, p))
	}
	// Persist totals at the platform's six-place precision.
	return lines, operationMoney(total), nil
}
func readOperation(ctx context.Context, tx *sql.Tx, kind, id string) (OperationDocument, error) {
	var d OperationDocument
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT payload FROM operation_documents WHERE kind=? AND id=?`, kind, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrOperationNotFound
	}
	if err != nil {
		return d, err
	}
	err = json.Unmarshal([]byte(raw), &d)
	return d, err
}
func operationChildren(ctx context.Context, tx *sql.Tx, kind, parent string) ([]OperationDocument, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payload FROM operation_documents WHERE kind=? AND parent_id=? ORDER BY created_at,id`, kind, parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []OperationDocument{}
	for rows.Next() {
		var raw string
		var d OperationDocument
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &d); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
func persistOperation(ctx context.Context, tx *sql.Tx, d OperationDocument, actor, action string) error {
	if len(d.RequestKey) > 200 {
		return ErrOperationInvalid
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	parent := d.PurchaseOrderID
	if d.Kind == "shipment" {
		parent = d.OrderID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_documents(id,kind,status,parent_id,request_key,payload,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,payload=excluded.payload,updated_at=excluded.updated_at`, d.ID, d.Kind, d.Status, checkoutNullableTrim(parent), checkoutNullableTrim(d.RequestKey), string(raw), d.CreatedAt, d.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return ErrOperationState
		}
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_events VALUES(?,?,?,?,?)`, uuid.NewString(), d.ID, actor, action, d.UpdatedAt)
	return err
}
func (s *OperationsService) expire(d OperationDocument) OperationDocument {
	if d.Kind == "quote" && (d.Status == "draft" || d.Status == "sent") && d.ValidUntil != "" && d.ValidUntil < s.now().UTC().Format("2006-01-02") {
		d.Status = "expired"
	}
	return d
}
func (s *OperationsService) Get(ctx context.Context, kind, id string) (OperationDocument, error) {
	if !s.allowed(kind) {
		return OperationDocument{}, ErrOperationInvalid
	}
	db, _, err := s.open(ctx)
	if err != nil {
		return OperationDocument{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return OperationDocument{}, err
	}
	defer tx.Rollback()
	d, err := readOperation(ctx, tx, kind, id)
	if err == nil && kind == "purchase_order" {
		d.FullyReceived, err = operationFullyReceived(ctx, tx, d)
	}
	if err == nil && kind == "order" {
		err = operationFulfillment(ctx, tx, &d)
	}
	return s.expire(d), err
}
func (s *OperationsService) List(ctx context.Context, kind string, query ListQuery) (ListResult[OperationDocument], error) {
	var zero ListResult[OperationDocument]
	if !s.allowed(kind) {
		return zero, ErrOperationInvalid
	}
	query, _, err := NormalizeListQuery(query, "created_at", "desc", map[string]string{"created_at": "created_at", "updated_at": "updated_at", "status": "status", "name": "name", "total_amount": "total_amount"})
	if err != nil {
		return zero, err
	}
	for key := range query.Filters {
		if key != "status" && key != "parent_id" {
			return zero, ErrInvalidListQuery
		}
	}
	db, _, err := s.open(ctx)
	if err != nil {
		return zero, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	all, err := operationChildrenAll(ctx, tx, kind)
	if err != nil {
		return zero, err
	}
	items := []OperationDocument{}
	for _, d := range all {
		d = s.expire(d)
		if status := query.Filters["status"]; status != "" && d.Status != status {
			continue
		}
		if parent := query.Filters["parent_id"]; parent != "" && d.PurchaseOrderID != parent && d.OrderID != parent {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{d.ID, d.Name, d.CustomerName, d.VendorName, d.OutputDescription, d.Note}, " "))
		if !strings.Contains(haystack, strings.ToLower(query.Keyword)) {
			continue
		}
		if kind == "purchase_order" {
			d.FullyReceived, err = operationFullyReceived(ctx, tx, d)
			if err != nil {
				return zero, err
			}
		}
		if kind == "order" {
			if err = operationFulfillment(ctx, tx, &d); err != nil {
				return zero, err
			}
		}
		items = append(items, d)
	}
	field := func(d OperationDocument) string {
		switch query.Sort {
		case "updated_at":
			return d.UpdatedAt
		case "status":
			return d.Status
		case "name":
			return d.Name + d.CustomerName + d.VendorName + d.OutputDescription
		case "total_amount":
			return d.TotalAmount
		}
		return d.CreatedAt
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := field(items[i]), field(items[j])
		cmp := strings.Compare(a, b)
		if query.Sort == "total_amount" {
			ar, _ := new(big.Rat).SetString(a)
			br, _ := new(big.Rat).SetString(b)
			if ar != nil && br != nil {
				cmp = ar.Cmp(br)
			}
		}
		if cmp == 0 {
			cmp = strings.Compare(items[i].ID, items[j].ID)
		}
		if query.Order == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	total := len(items)
	offset := ListOffset(query)
	if offset < 0 || offset > total {
		offset = total
	}
	end := offset + query.PageSize
	if end > total {
		end = total
	}
	return NewListResult(items[offset:end], query, total), nil
}
func operationChildrenAll(ctx context.Context, tx *sql.Tx, kind string) ([]OperationDocument, error) {
	rows, err := tx.QueryContext(ctx, `SELECT payload FROM operation_documents WHERE kind=?`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []OperationDocument{}
	for rows.Next() {
		var raw string
		var d OperationDocument
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &d); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
func (s *OperationsService) stamp(d *OperationDocument, actor string) {
	d.RequestKey = strings.TrimSpace(d.RequestKey)
	d.ExpectedUpdatedAt = ""
	d.ID = uuid.NewString()
	d.CreatedBy = actor
	d.CreatedAt = s.now().UTC().Format(time.RFC3339Nano)
	d.UpdatedAt = d.CreatedAt
	d.InventoryRecordID = ""
	d.CompletedAt = ""
	d.ResultingOrderID = ""
	d.SourceQuoteID = ""
	d.TotalAmount = ""
	d.FullyReceived = false
}
func (s *OperationsService) Save(ctx context.Context, kind, id, actor string, input OperationDocument) (OperationDocument, error) {
	d := input
	if !s.allowed(kind) || kind == "shipment" || kind == "goods_receipt" || kind == "vendor_invoice" {
		return d, ErrOperationInvalid
	}
	db, _, err := s.open(ctx)
	if err != nil {
		return d, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	s.stamp(&d, actor)
	d.Kind = kind
	d.Status = "draft"
	d.RequestKey = ""
	d.PurchaseOrderID = ""
	d.OrderID = ""
	if kind == "vendor" {
		d.Status = input.Status
		if d.Status == "" {
			d.Status = "active"
		}
	}
	if kind == "production_order" {
		d.Status = "planned"
	}
	if id != "" {
		current, e := readOperation(ctx, tx, kind, id)
		if e != nil {
			return d, e
		}
		if input.ExpectedUpdatedAt != "" && input.ExpectedUpdatedAt != current.UpdatedAt {
			return d, ErrOperationStale
		}
		if current.Status != "draft" && current.Status != "planned" && kind != "vendor" {
			return d, ErrOperationState
		}
		d.ID = id
		d.CreatedAt = current.CreatedAt
		d.CreatedBy = current.CreatedBy
		d.SourceQuoteID = current.SourceQuoteID
		d.UpdatedAt = operationUpdatedAt(s.now(), current.UpdatedAt)
	}
	if err = s.validate(ctx, tx, &d); err != nil {
		return d, err
	}
	if err = persistOperation(ctx, tx, d, actor, "saved"); err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *OperationsService) validate(ctx context.Context, tx *sql.Tx, d *OperationDocument) error {
	var err error
	switch d.Kind {
	case "vendor":
		d.Name = strings.TrimSpace(d.Name)
		if d.Name == "" || (d.Status != "active" && d.Status != "inactive") {
			return ErrOperationInvalid
		}
		return nil
	case "production_order":
		d.OutputDescription = strings.TrimSpace(d.OutputDescription)
		if d.OutputDescription == "" {
			return ErrOperationInvalid
		}
		if _, err = operationQuantity(d.OutputQuantity, true); err != nil {
			return err
		}
		if err = operationDate(d.PlannedDate, true); err != nil {
			return err
		}
		if len(d.Materials) > 1000 {
			return ErrOperationInvalid
		}
		for i := range d.Materials {
			m := &d.Materials[i]
			m.ID = uuid.NewString()
			if strings.TrimSpace(m.Description) == "" {
				return ErrOperationInvalid
			}
			if _, err = operationQuantity(m.QuantityUsed, true); err != nil {
				return err
			}
		}
		return nil
	case "quote", "order":
		if d.CustomerID != "" && strings.TrimSpace(d.CustomerName) == "" {
			crmPath := filepath.Join(s.directory, "crm.db")
			if _, statErr := os.Stat(crmPath); statErr == nil {
				customer, lookupErr := NewCRMService(s.directory).GetCustomer(ctx, d.CustomerID)
				if lookupErr != nil {
					if errors.Is(lookupErr, ErrCRMNotFound) {
						return ErrOperationInvalid
					}
					return lookupErr
				}
				d.CustomerName = customer.Name
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
		}
		d.CustomerName = strings.TrimSpace(d.CustomerName)
		if d.CustomerName == "" {
			return fmt.Errorf("%w: customer_name snapshot is required", ErrOperationInvalid)
		}
		date := d.OrderDate
		if d.Kind == "quote" {
			date = d.QuoteDate
			if err = operationDate(d.ValidUntil, true); err != nil {
				return err
			}
			if d.ValidUntil != "" && d.ValidUntil < date {
				return ErrOperationInvalid
			}
		}
		if err = operationDate(date, false); err != nil {
			return err
		}
	case "purchase_order":
		if d.VendorID != "" {
			v, e := readOperation(ctx, tx, "vendor", d.VendorID)
			if e != nil {
				return e
			}
			if v.Status != "active" {
				return ErrOperationInvalid
			}
			d.VendorName = v.Name
		}
		d.VendorName = strings.TrimSpace(d.VendorName)
		if d.VendorName == "" {
			return ErrOperationInvalid
		}
		if err = operationDate(d.OrderDate, false); err != nil {
			return err
		}
	default:
		return ErrOperationInvalid
	}
	if !currencyPattern.MatchString(d.Currency) {
		return ErrOperationInvalid
	}
	d.Lines, d.TotalAmount, err = normalizeOperationLines(d.Lines)
	return err
}

type OperationEvent struct {
	ID          string `json:"id"`
	DocumentID  string `json:"document_id"`
	ActorUserID string `json:"actor_user_id"`
	Action      string `json:"action"`
	OccurredAt  string `json:"occurred_at"`
}

func (s *OperationsService) Events(ctx context.Context, kind, id string, query ListQuery) (ListResult[OperationEvent], error) {
	var zero ListResult[OperationEvent]
	if !s.allowed(kind) {
		return zero, ErrOperationInvalid
	}
	query, sortColumn, err := NormalizeListQuery(query, "occurred_at", "desc", map[string]string{"occurred_at": "occurred_at", "action": "action"})
	if err != nil {
		return zero, err
	}
	if len(query.Filters) > 0 {
		return zero, ErrInvalidListQuery
	}
	db, _, err := s.open(ctx)
	if err != nil {
		return zero, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	if _, err = readOperation(ctx, tx, kind, id); err != nil {
		return zero, err
	}
	where := ` WHERE document_id=? AND (action LIKE ? OR actor_user_id LIKE ?)`
	keyword := "%" + query.Keyword + "%"
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM operation_events`+where, id, keyword, keyword).Scan(&total); err != nil {
		return zero, err
	}
	offset := ListOffset(query)
	if offset < 0 {
		offset = total
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,document_id,actor_user_id,action,occurred_at FROM operation_events`+where+` ORDER BY `+sortColumn+` `+query.Order+`,id LIMIT ? OFFSET ?`, id, keyword, keyword, query.PageSize, offset)
	if err != nil {
		return zero, err
	}
	defer rows.Close()
	items := []OperationEvent{}
	for rows.Next() {
		var e OperationEvent
		if err = rows.Scan(&e.ID, &e.DocumentID, &e.ActorUserID, &e.Action, &e.OccurredAt); err != nil {
			return zero, err
		}
		items = append(items, e)
	}
	return NewListResult(items, query, total), rows.Err()
}

// Updated timestamps also act as optimistic edit tokens, even with a fixed clock.
func operationUpdatedAt(now time.Time, previous string) string {
	if old, err := time.Parse(time.RFC3339Nano, previous); err == nil && !now.After(old) {
		now = old.Add(time.Nanosecond)
	}
	return now.UTC().Format(time.RFC3339Nano)
}
func operationFulfillment(ctx context.Context, tx *sql.Tx, d *OperationDocument) error {
	shipped, err := shippedQuantities(ctx, tx, d.ID)
	if err != nil {
		return err
	}
	for i := range d.Lines {
		line := &d.Lines[i]
		q, err := operationQuantity(line.Quantity, true)
		if err != nil {
			return err
		}
		done := shipped[line.ID]
		if done == nil {
			done = new(big.Rat)
		}
		line.FulfilledQuantity = operationMoney(done)
		line.RemainingQuantity = operationMoney(new(big.Rat).Sub(q, done))
	}
	return nil
}
