package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInventoryInvalid = errors.New("invalid inventory input")
var ErrInventoryNotFound = errors.New("inventory record not found")
var ErrInventoryState = errors.New("invalid inventory state or insufficient stock")

type InventoryService struct{ path string }

func NewInventoryService(directory string) *InventoryService {
	if directory == "" {
		directory = defaultDatabaseDirectory
	}
	return &InventoryService{filepath.Join(directory, "inventory.db")}
}
func (s *InventoryService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err == nil {
		db.SetMaxOpenConns(1)
	}
	return db, err
}

type InventoryEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Status      string `json:"status"`
	WarehouseID string `json:"warehouse_id"`
	SKU         string `json:"sku"`
	Unit        string `json:"unit_of_measure"`
	Barcode     string `json:"barcode"`
}
type InventoryLine struct {
	OrderLineID string `json:"order_line_id,omitempty"`
	ItemID      string `json:"item_id"`
	LocationID  string `json:"location_id"`
	Quantity    string `json:"quantity"`
}
type InventoryRecord struct {
	SourceModule      string          `json:"source_module,omitempty"`
	SourceReferenceID string          `json:"source_reference_id,omitempty"`
	ID                string          `json:"id"`
	Kind              string          `json:"kind"`
	Status            string          `json:"status"`
	WarehouseID       string          `json:"warehouse_id"`
	ToWarehouseID     string          `json:"to_warehouse_id"`
	ToLocationID      string          `json:"to_location_id"`
	Date              string          `json:"date"`
	Note              string          `json:"note"`
	TrackingReference string          `json:"tracking_reference"`
	CreatedAt         string          `json:"created_at"`
	CreatedBy         string          `json:"created_by"`
	Lines             []InventoryLine `json:"lines"`
}

func inventoryQuantity(value string) (int64, error) {
	if !checkoutDecimalPattern.MatchString(strings.TrimPrefix(value, "-")) {
		return 0, ErrInventoryInvalid
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return 0, ErrInventoryInvalid
	}
	r.Mul(r, big.NewRat(1000000, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, ErrInventoryInvalid
	}
	return r.Num().Int64(), nil
}
func inventoryAmount(q int64) string { return normalizeRat(big.NewRat(q, 1000000)) }
func (s *InventoryService) Entities(ctx context.Context, kind string) ([]InventoryEntity, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	query := ""
	switch kind {
	case "warehouses":
		query = `SELECT id,name,address,status,'','','','' FROM inventory_warehouses ORDER BY name`
	case "locations":
		query = `SELECT id,name,'',status,warehouse_id,'','','' FROM inventory_locations ORDER BY name`
	case "items":
		query = `SELECT id,name,'','','',sku,unit_of_measure,COALESCE(barcode,'') FROM inventory_items ORDER BY name`
	default:
		return nil, ErrInventoryInvalid
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InventoryEntity{}
	for rows.Next() {
		var e InventoryEntity
		if err := rows.Scan(&e.ID, &e.Name, &e.Address, &e.Status, &e.WarehouseID, &e.SKU, &e.Unit, &e.Barcode); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
func (s *InventoryService) SaveEntity(ctx context.Context, kind string, e InventoryEntity) (InventoryEntity, error) {
	e.Name = strings.TrimSpace(e.Name)
	e.SKU = strings.TrimSpace(e.SKU)
	e.Barcode = strings.TrimSpace(e.Barcode)
	e.Unit = strings.TrimSpace(e.Unit)
	if e.Name == "" {
		return e, ErrInventoryInvalid
	}
	if e.Status == "" {
		e.Status = "active"
	}
	if e.Status != "active" && e.Status != "inactive" {
		return e, ErrInventoryInvalid
	}
	db, err := s.open()
	if err != nil {
		return e, err
	}
	defer db.Close()
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch kind {
	case "items", "warehouses", "locations":
	default:
		return e, ErrInventoryInvalid
	}
	// Existing locations cannot be silently reassigned while their ledger retains the old warehouse.
	if kind == "locations" {
		var oldWarehouse string
		lookupErr := db.QueryRowContext(ctx, `SELECT warehouse_id FROM inventory_locations WHERE id=?`, e.ID).Scan(&oldWarehouse)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return e, lookupErr
		}
		if lookupErr == nil && oldWarehouse != e.WarehouseID {
			return e, ErrInventoryInvalid
		}
	}
	switch kind {
	case "warehouses":
		_, err = db.ExecContext(ctx, `INSERT INTO inventory_warehouses VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,address=excluded.address,status=excluded.status`, e.ID, e.Name, e.Address, e.Status, now)
	case "locations":
		var n int
		err = db.QueryRowContext(ctx, `SELECT count(*) FROM inventory_warehouses WHERE id=? AND status='active'`, e.WarehouseID).Scan(&n)
		if err != nil {
			return e, err
		}
		if n != 1 {
			return e, ErrInventoryInvalid
		}
		_, err = db.ExecContext(ctx, `INSERT INTO inventory_locations VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,status=excluded.status WHERE inventory_locations.warehouse_id=excluded.warehouse_id`, e.ID, e.WarehouseID, e.Name, e.Status, now)
	case "items":
		if e.SKU == "" || e.Unit == "" {
			return e, ErrInventoryInvalid
		}
		_, err = db.ExecContext(ctx, `INSERT INTO inventory_items VALUES(?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET sku=excluded.sku,name=excluded.name,unit_of_measure=excluded.unit_of_measure,barcode=excluded.barcode`, e.ID, e.SKU, e.Name, e.Unit, checkoutNullableTrim(e.Barcode))
	default:
		return e, ErrInventoryInvalid
	}
	if err != nil {
		return e, fmt.Errorf("%w: duplicate code or invalid reference", ErrInventoryInvalid)
	}
	return e, nil
}
func (s *InventoryService) Scan(ctx context.Context, code string) (InventoryEntity, error) {
	items, err := s.Entities(ctx, "items")
	if err != nil {
		return InventoryEntity{}, err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return InventoryEntity{}, ErrInventoryInvalid
	}
	for _, e := range items {
		if e.Barcode == code {
			return e, nil
		}
	}
	for _, e := range items {
		if e.SKU == code {
			return e, nil
		}
	}
	return InventoryEntity{}, ErrInventoryNotFound
}
func (s *InventoryService) Records(ctx context.Context) ([]InventoryRecord, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT payload FROM inventory_records ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InventoryRecord{}
	for rows.Next() {
		var raw string
		var r InventoryRecord
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func inventoryLocation(ctx context.Context, tx *sql.Tx, warehouse, location string) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_warehouses WHERE id=? AND status='active'`, warehouse).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInventoryInvalid
	}
	if location != "" {
		err = tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_locations WHERE id=? AND warehouse_id=? AND status='active'`, location, warehouse).Scan(&n)
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrInventoryInvalid
		}
	}
	return nil
}
func (s *InventoryService) Post(ctx context.Context, actor string, r InventoryRecord) (InventoryRecord, error) {
	return s.writeRecord(ctx, actor, r, "")
}
func (s *InventoryService) Transition(ctx context.Context, actor, id, status string) (InventoryRecord, error) {
	return s.writeRecord(ctx, actor, InventoryRecord{Status: status}, id)
}
func (s *InventoryService) writeRecord(ctx context.Context, actor string, r InventoryRecord, id string) (InventoryRecord, error) {
	db, err := s.open()
	if err != nil {
		return r, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	result, err := writeInventoryRecord(ctx, tx, actor, r, id)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func writeInventoryRecord(ctx context.Context, tx *sql.Tx, actor string, r InventoryRecord, id string) (InventoryRecord, error) {
	var err error
	post := true
	if id != "" {
		var raw string
		err = tx.QueryRowContext(ctx, `SELECT payload FROM inventory_records WHERE id=?`, id).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return r, ErrInventoryNotFound
		}
		if err != nil {
			return r, err
		}
		status := r.Status
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			return r, err
		}
		if r.Kind != "outbound" || !(r.Status == "pending" && (status == "packed" || status == "shipped" || status == "cancelled") || r.Status == "packed" && (status == "shipped" || status == "cancelled")) {
			return r, ErrInventoryState
		}
		r.Status = status
		post = status == "shipped"
	} else {
		r.ID = uuid.NewString()
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		r.CreatedBy = actor
		if _, err = time.Parse("2006-01-02", r.Date); err != nil {
			return r, ErrInventoryInvalid
		}
		if len(r.Lines) == 0 {
			return r, ErrInventoryInvalid
		}
		switch r.Kind {
		case "inbound", "transfer", "adjustment":
			r.Status = "posted"
		case "outbound":
			if r.Status == "" {
				r.Status = "pending"
			}
			if r.Status != "pending" && r.Status != "packed" && r.Status != "shipped" {
				return r, ErrInventoryInvalid
			}
			post = r.Status == "shipped"
		default:
			return r, ErrInventoryInvalid
		}
	}
	for _, line := range r.Lines {
		q, e := inventoryQuantity(line.Quantity)
		if e != nil || q == 0 || q < 0 && r.Kind != "adjustment" {
			return r, ErrInventoryInvalid
		}
		if e = inventoryLocation(ctx, tx, r.WarehouseID, line.LocationID); e != nil {
			return r, e
		}
		var n int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_items WHERE id=?`, line.ItemID).Scan(&n); e != nil {
			return r, e
		}
		if n != 1 {
			return r, ErrInventoryInvalid
		}
		if r.Kind == "transfer" {
			if r.WarehouseID == r.ToWarehouseID && line.LocationID == r.ToLocationID {
				return r, ErrInventoryInvalid
			}
			if e = inventoryLocation(ctx, tx, r.ToWarehouseID, r.ToLocationID); e != nil {
				return r, e
			}
		}
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	if id == "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO inventory_records VALUES(?,?,?,?,?,?)`, r.ID, r.Kind, r.Status, string(raw), r.CreatedAt, r.CreatedBy)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE inventory_records SET status=?,payload=? WHERE id=?`, r.Status, string(raw), r.ID)
	}
	if err != nil {
		return r, err
	}
	if post {
		for _, line := range r.Lines {
			q, _ := inventoryQuantity(line.Quantity)
			kind := "receipt"
			if r.Kind == "outbound" {
				q = -q
				kind = "issue"
			}
			if r.Kind == "adjustment" {
				kind = "adjustment"
			}
			if r.Kind == "transfer" {
				q = -q
				kind = "transfer_out"
			}
			if err = inventoryMove(ctx, tx, r, line, r.WarehouseID, line.LocationID, q, kind); err != nil {
				return r, err
			}
			if r.Kind == "transfer" {
				if err = inventoryMove(ctx, tx, r, line, r.ToWarehouseID, r.ToLocationID, -q, "transfer_in"); err != nil {
					return r, err
				}
			}
		}
	}
	return r, nil
}
func inventoryMove(ctx context.Context, tx *sql.Tx, r InventoryRecord, line InventoryLine, warehouse, location string, q int64, kind string) error {
	var balance int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity_delta),0) FROM inventory_movements WHERE item_id=? AND warehouse_id=? AND COALESCE(location_id,'')=?`, line.ItemID, warehouse, location).Scan(&balance)
	if err != nil {
		return err
	}
	if q < 0 && (q == math.MinInt64 || balance < -q) {
		return ErrInventoryState
	}
	if q > 0 && balance > math.MaxInt64-q {
		return ErrInventoryInvalid
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO inventory_movements VALUES(?,?,?,?,?,?,?,?)`, uuid.NewString(), line.ItemID, warehouse, checkoutNullableTrim(location), q, kind, r.ID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

type InventoryStock struct {
	ItemID      string `json:"item_id"`
	WarehouseID string `json:"warehouse_id"`
	LocationID  string `json:"location_id"`
	Quantity    string `json:"quantity"`
}

func (s *InventoryService) Stock(ctx context.Context) ([]InventoryStock, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT item_id,warehouse_id,COALESCE(location_id,''),SUM(quantity_delta) FROM inventory_movements GROUP BY item_id,warehouse_id,location_id ORDER BY item_id,warehouse_id,location_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InventoryStock{}
	for rows.Next() {
		var v InventoryStock
		var q int64
		if err = rows.Scan(&v.ItemID, &v.WarehouseID, &v.LocationID, &q); err != nil {
			return nil, err
		}
		v.Quantity = inventoryAmount(q)
		result = append(result, v)
	}
	return result, rows.Err()
}

type InventoryMovement struct {
	ID                string `json:"id"`
	ItemID            string `json:"item_id"`
	WarehouseID       string `json:"warehouse_id"`
	LocationID        string `json:"location_id"`
	QuantityDelta     string `json:"quantity_delta"`
	MovementType      string `json:"movement_type"`
	SourceReferenceID string `json:"source_reference_id"`
	OccurredAt        string `json:"occurred_at"`
}

func (s *InventoryService) Movements(ctx context.Context) ([]InventoryMovement, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id,item_id,warehouse_id,COALESCE(location_id,''),quantity_delta,movement_type,source_reference_id,occurred_at FROM inventory_movements ORDER BY occurred_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InventoryMovement{}
	for rows.Next() {
		var v InventoryMovement
		var q int64
		if err = rows.Scan(&v.ID, &v.ItemID, &v.WarehouseID, &v.LocationID, &q, &v.MovementType, &v.SourceReferenceID, &v.OccurredAt); err != nil {
			return nil, err
		}
		v.QuantityDelta = inventoryAmount(q)
		result = append(result, v)
	}
	return result, rows.Err()
}
