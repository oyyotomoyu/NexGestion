package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/google/uuid"
)

func (s *OperationsService) Transition(ctx context.Context, kind, id, actor, status string) (OperationDocument, error) {
	return s.TransitionExpected(ctx, kind, id, actor, status, "")
}

func (s *OperationsService) TransitionExpected(ctx context.Context, kind, id, actor, status, expected string) (OperationDocument, error) {
	var d OperationDocument
	if !s.allowed(kind) {
		return d, ErrOperationInvalid
	}
	db, inventory, err := s.open(ctx)
	if err != nil {
		return d, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	d, err = readOperation(ctx, tx, kind, id)
	if err != nil {
		return d, err
	}
	if expected != "" && expected != d.UpdatedAt {
		return d, ErrOperationStale
	}
	if kind == "quote" && d.Status == "accepted" && status == "accepted" {
		return d, nil
	}
	d = s.expire(d)
	d.UpdatedAt = operationUpdatedAt(s.now(), d.UpdatedAt)
	switch kind {
	case "quote":
		if !(d.Status == "draft" && status == "sent" || d.Status == "sent" && (status == "accepted" || status == "rejected")) {
			return d, ErrOperationState
		}
		if status == "accepted" {
			order := d
			order.ID = uuid.NewString()
			order.Kind = "order"
			order.Status = "draft"
			order.SourceQuoteID = d.ID
			order.ResultingOrderID = ""
			order.OrderDate = s.now().UTC().Format("2006-01-02")
			order.QuoteDate = ""
			order.ValidUntil = ""
			order.CreatedAt = d.UpdatedAt
			order.UpdatedAt = d.UpdatedAt
			order.CreatedBy = actor
			order.RequestKey = ""
			order.Lines = append([]OperationLine(nil), d.Lines...)
			for i := range order.Lines {
				order.Lines[i].ID = uuid.NewString()
			}
			if err = persistOperation(ctx, tx, order, actor, "quote_accepted"); err != nil {
				return d, err
			}
			d.ResultingOrderID = order.ID
		}
	case "order":
		if !(d.Status == "draft" && (status == "confirmed" || status == "cancelled") || d.Status == "confirmed" && status == "cancelled") {
			return d, ErrOperationState
		}
		if status == "cancelled" {
			shipments, e := operationChildren(ctx, tx, "shipment", d.ID)
			if e != nil {
				return d, e
			}
			if len(shipments) > 0 {
				return d, ErrOperationState
			}
		}
	case "purchase_order":
		valid := d.Status == "draft" && (status == "approved" || status == "cancelled") || d.Status == "approved" && (status == "sent" || status == "cancelled") || d.Status == "sent" && (status == "closed" || status == "cancelled")
		if !valid {
			return d, ErrOperationState
		}
		if status == "closed" {
			full, e := operationFullyReceived(ctx, tx, d)
			if e != nil {
				return d, e
			}
			if !full {
				return d, ErrOperationState
			}
		}
		if status == "cancelled" {
			receipts, e := operationChildren(ctx, tx, "goods_receipt", d.ID)
			if e != nil {
				return d, e
			}
			invoices, e := operationChildren(ctx, tx, "vendor_invoice", d.ID)
			if e != nil {
				return d, e
			}
			if len(receipts) > 0 || len(invoices) > 0 {
				return d, ErrOperationState
			}
		}
	case "production_order":
		if !(d.Status == "planned" && (status == "in_progress" || status == "cancelled") || d.Status == "in_progress" && (status == "completed" || status == "cancelled")) {
			return d, ErrOperationState
		}
		if status == "completed" {
			d.CompletedAt = d.UpdatedAt
			if inventory {
				d.InventoryRecordID, err = postProductionInventory(ctx, tx, actor, d)
				if err != nil {
					return d, err
				}
			}
		}
	default:
		return d, ErrOperationInvalid
	}
	d.Status = status
	if err = persistOperation(ctx, tx, d, actor, "status:"+status); err != nil {
		return d, err
	}
	return d, tx.Commit()
}

func postOperationInventory(ctx context.Context, tx *sql.Tx, actor, module, source, kind, warehouse, date, note string, lines []InventoryLine) (string, error) {
	if len(lines) == 0 {
		return "", nil
	}
	r, err := writeInventoryRecord(ctx, tx, actor, InventoryRecord{Kind: kind, Status: "shipped", WarehouseID: warehouse, Date: date, Note: note, SourceModule: module, SourceReferenceID: source, Lines: lines}, "")
	return r.ID, err
}
func postProductionInventory(ctx context.Context, tx *sql.Tx, actor string, d OperationDocument) (string, error) {
	linked := d.OutputInventoryItemID != ""
	for _, m := range d.Materials {
		linked = linked || m.InventoryItemID != ""
	}
	if !linked {
		return "", nil
	}
	if err := inventoryLocation(ctx, tx, d.WarehouseID, d.LocationID); err != nil {
		return "", err
	}
	// The existing ledger requires a provenance row, not a warehouse inbound/outbound document.
	r := InventoryRecord{ID: uuid.NewString(), Kind: "production", Status: "posted", WarehouseID: d.WarehouseID, Date: d.CompletedAt[:10], CreatedAt: d.CompletedAt, CreatedBy: actor, SourceModule: "production", SourceReferenceID: d.ID, Note: d.OutputDescription}
	for _, m := range d.Materials {
		if m.InventoryItemID != "" {
			r.Lines = append(r.Lines, InventoryLine{ItemID: m.InventoryItemID, LocationID: m.LocationID, Quantity: "-" + m.QuantityUsed})
		}
	}
	if d.OutputInventoryItemID != "" {
		r.Lines = append(r.Lines, InventoryLine{ItemID: d.OutputInventoryItemID, LocationID: d.LocationID, Quantity: d.OutputQuantity})
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO inventory_records VALUES(?,?,?,?,?,?)`, r.ID, r.Kind, r.Status, string(raw), r.CreatedAt, actor); err != nil {
		return "", err
	}
	move := func(item, location, quantity, kind string, negative bool) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM inventory_items WHERE id=?`, item).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return ErrInventoryInvalid
		}
		if err := inventoryLocation(ctx, tx, d.WarehouseID, location); err != nil {
			return err
		}
		q, err := inventoryQuantity(quantity)
		if err != nil || q <= 0 {
			return ErrOperationInvalid
		}
		if negative {
			q = -q
		}
		return inventoryMove(ctx, tx, r, InventoryLine{ItemID: item}, d.WarehouseID, location, q, kind)
	}
	for _, m := range d.Materials {
		if m.InventoryItemID != "" {
			if err = move(m.InventoryItemID, m.LocationID, m.QuantityUsed, "production_input", true); err != nil {
				return "", err
			}
		}
	}
	if d.OutputInventoryItemID != "" {
		if err = move(d.OutputInventoryItemID, d.LocationID, d.OutputQuantity, "production_output", false); err != nil {
			return "", err
		}
	}
	return r.ID, nil
}
func acceptedQuantities(ctx context.Context, tx *sql.Tx, parent string) (map[string]*big.Rat, error) {
	children, err := operationChildren(ctx, tx, "goods_receipt", parent)
	if err != nil {
		return nil, err
	}
	result := map[string]*big.Rat{}
	for _, child := range children {
		for _, line := range child.Lines {
			if result[line.PurchaseOrderLineID] == nil {
				result[line.PurchaseOrderLineID] = new(big.Rat)
			}
			q, e := operationQuantity(line.QuantityAccepted, false)
			if e != nil {
				return nil, e
			}
			result[line.PurchaseOrderLineID].Add(result[line.PurchaseOrderLineID], q)
		}
	}
	return result, nil
}
func operationFullyReceived(ctx context.Context, tx *sql.Tx, po OperationDocument) (bool, error) {
	accepted, err := acceptedQuantities(ctx, tx, po.ID)
	if err != nil {
		return false, err
	}
	for _, line := range po.Lines {
		q, e := operationQuantity(line.Quantity, true)
		if e != nil {
			return false, e
		}
		if accepted[line.ID] == nil || accepted[line.ID].Cmp(q) < 0 {
			return false, nil
		}
	}
	return len(po.Lines) > 0, nil
}
func (s *OperationsService) Receive(ctx context.Context, actor, parent string, input OperationDocument) (OperationDocument, error) {
	d := input
	if s.module != "procurement" {
		return d, ErrOperationInvalid
	}
	db, inventory, err := s.open(ctx)
	if err != nil {
		return d, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	po, err := readOperation(ctx, tx, "purchase_order", parent)
	if err != nil {
		return d, err
	}
	if po.Status != "sent" {
		return d, ErrOperationState
	}
	s.stamp(&d, actor)
	d.Kind = "goods_receipt"
	d.Status = "posted"
	d.PurchaseOrderID = parent
	d.OrderID = ""
	d.ReceivedByUserID = actor
	d.InspectedByUserID = actor
	d.InspectedAt = d.CreatedAt
	if err = operationDate(d.ReceivedDate, false); err != nil {
		return d, err
	}
	if len(d.Lines) == 0 || len(d.Lines) > 1000 {
		return d, ErrOperationInvalid
	}
	accepted, err := acceptedQuantities(ctx, tx, parent)
	if err != nil {
		return d, err
	}
	stockLines := []InventoryLine{}
	for i := range d.Lines {
		line := &d.Lines[i]
		var original *OperationLine
		for j := range po.Lines {
			if po.Lines[j].ID == line.PurchaseOrderLineID {
				original = &po.Lines[j]
				break
			}
		}
		if original == nil {
			return d, ErrOperationInvalid
		}
		received, e := operationQuantity(line.QuantityReceived, true)
		if e != nil {
			return d, e
		}
		a, e := operationQuantity(line.QuantityAccepted, false)
		if e != nil {
			return d, e
		}
		if line.QuantityRejected == "" {
			line.QuantityRejected = "0"
		}
		rejected, e := operationQuantity(line.QuantityRejected, false)
		if e != nil {
			return d, e
		}
		if new(big.Rat).Add(a, rejected).Cmp(received) != 0 || rejected.Sign() > 0 && strings.TrimSpace(line.RejectionReason) == "" {
			return d, ErrOperationInvalid
		}
		if accepted[original.ID] == nil {
			accepted[original.ID] = new(big.Rat)
		}
		accepted[original.ID].Add(accepted[original.ID], a)
		ordered, _ := operationQuantity(original.Quantity, true)
		if accepted[original.ID].Cmp(ordered) > 0 {
			return d, ErrOperationState
		}
		line.ID = uuid.NewString()
		line.Description = original.Description
		line.InventoryItemID = original.InventoryItemID
		if inventory && line.InventoryItemID != "" && a.Sign() > 0 {
			stockLines = append(stockLines, InventoryLine{ItemID: line.InventoryItemID, LocationID: line.LocationID, Quantity: operationMoney(a)})
		}
	}
	d.InventoryRecordID, err = postOperationInventory(ctx, tx, actor, "procurement", d.ID, "inbound", d.WarehouseID, d.ReceivedDate, d.Note, stockLines)
	if err != nil {
		return d, err
	}
	if err = persistOperation(ctx, tx, d, actor, "received"); err != nil {
		return d, err
	}
	if err = s.matchInvoices(ctx, tx, po, actor, d.CreatedAt); err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *OperationsService) Invoice(ctx context.Context, actor, parent string, input OperationDocument) (OperationDocument, error) {
	d := input
	if s.module != "procurement" {
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
	po, err := readOperation(ctx, tx, "purchase_order", parent)
	if err != nil {
		return d, err
	}
	if po.Status != "sent" && po.Status != "closed" {
		return d, ErrOperationState
	}
	s.stamp(&d, actor)
	d.Kind = "vendor_invoice"
	d.Status = "recorded"
	d.PurchaseOrderID = parent
	d.OrderID = ""
	d.MatchStatus = "pending"
	d.VendorInvoiceNumber = strings.TrimSpace(d.VendorInvoiceNumber)
	if d.VendorInvoiceNumber == "" || !currencyPattern.MatchString(d.Currency) {
		return d, ErrOperationInvalid
	}
	if err = operationDate(d.InvoiceDate, false); err != nil {
		return d, err
	}
	if _, err = operationDecimal(d.InvoiceAmount, true); err != nil {
		return d, err
	}
	if err = persistOperation(ctx, tx, d, actor, "invoice_recorded"); err != nil {
		return d, err
	}
	if err = s.matchInvoices(ctx, tx, po, actor, d.CreatedAt); err != nil {
		return d, err
	}
	d, err = readOperation(ctx, tx, d.Kind, d.ID)
	if err != nil {
		return d, err
	}
	return d, tx.Commit()
}
func (s *OperationsService) matchInvoices(ctx context.Context, tx *sql.Tx, po OperationDocument, actor, stamp string) error {
	invoices, err := operationChildren(ctx, tx, "vendor_invoice", po.ID)
	if err != nil {
		return err
	}
	full, err := operationFullyReceived(ctx, tx, po)
	if err != nil {
		return err
	}
	sum := new(big.Rat)
	currencyOK := true
	for _, invoice := range invoices {
		amount, e := operationDecimal(invoice.InvoiceAmount, true)
		if e != nil {
			return e
		}
		sum.Add(sum, amount)
		currencyOK = currencyOK && invoice.Currency == po.Currency
	}
	total, err := operationDecimal(po.TotalAmount, false)
	if err != nil {
		return err
	}
	status := "pending"
	if !currencyOK || sum.Cmp(total) > 0 {
		status = "discrepancy"
	} else if full && sum.Cmp(total) == 0 {
		status = "matched"
	}
	for _, invoice := range invoices {
		if invoice.MatchStatus != status {
			invoice.MatchStatus = status
			invoice.UpdatedAt = stamp
			if err = persistOperation(ctx, tx, invoice, actor, "invoice_match:"+status); err != nil {
				return err
			}
		}
	}
	return nil
}
func shippedQuantities(ctx context.Context, tx *sql.Tx, parent string) (map[string]*big.Rat, error) {
	children, err := operationChildren(ctx, tx, "shipment", parent)
	if err != nil {
		return nil, err
	}
	result := map[string]*big.Rat{}
	for _, child := range children {
		for _, line := range child.Lines {
			if result[line.OrderLineID] == nil {
				result[line.OrderLineID] = new(big.Rat)
			}
			q, e := operationQuantity(line.Quantity, true)
			if e != nil {
				return nil, e
			}
			result[line.OrderLineID].Add(result[line.OrderLineID], q)
		}
	}
	return result, nil
}
func (s *OperationsService) Ship(ctx context.Context, actor, parent string, input OperationDocument, fulfill bool) (OperationDocument, error) {
	d := input
	if s.module != "order" {
		return d, ErrOperationInvalid
	}
	db, inventory, err := s.open(ctx)
	if err != nil {
		return d, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	order, err := readOperation(ctx, tx, "order", parent)
	if err != nil {
		return d, err
	}
	if input.ExpectedUpdatedAt != "" && input.ExpectedUpdatedAt != order.UpdatedAt {
		return d, ErrOperationStale
	}
	if order.Status != "confirmed" {
		return d, ErrOperationState
	}
	s.stamp(&d, actor)
	d.Kind = "shipment"
	d.Status = "shipped"
	d.OrderID = parent
	d.PurchaseOrderID = ""
	if d.ShippedDate == "" {
		d.ShippedDate = s.now().UTC().Format("2006-01-02")
	}
	if err = operationDate(d.ShippedDate, false); err != nil {
		return d, err
	}
	shipped, err := shippedQuantities(ctx, tx, parent)
	if err != nil {
		return d, err
	}
	if fulfill {
		d.Lines = nil
		for _, line := range order.Lines {
			q, _ := operationQuantity(line.Quantity, true)
			if shipped[line.ID] != nil {
				q.Sub(q, shipped[line.ID])
			}
			if q.Sign() > 0 {
				d.Lines = append(d.Lines, OperationLine{OrderLineID: line.ID, Quantity: operationMoney(q), LocationID: d.LocationID})
			}
		}
	}
	if len(d.Lines) == 0 || len(d.Lines) > 1000 {
		return d, ErrOperationInvalid
	}
	stockLines := []InventoryLine{}
	for i := range d.Lines {
		line := &d.Lines[i]
		var original *OperationLine
		for j := range order.Lines {
			if order.Lines[j].ID == line.OrderLineID {
				original = &order.Lines[j]
				break
			}
		}
		if original == nil {
			return d, ErrOperationInvalid
		}
		q, e := operationQuantity(line.Quantity, true)
		if e != nil {
			return d, e
		}
		if shipped[original.ID] == nil {
			shipped[original.ID] = new(big.Rat)
		}
		shipped[original.ID].Add(shipped[original.ID], q)
		ordered, _ := operationQuantity(original.Quantity, true)
		if shipped[original.ID].Cmp(ordered) > 0 {
			return d, ErrOperationState
		}
		line.ID = uuid.NewString()
		line.Description = original.Description
		line.InventoryItemID = original.InventoryItemID
		if inventory && line.InventoryItemID != "" {
			stockLines = append(stockLines, InventoryLine{ItemID: line.InventoryItemID, LocationID: line.LocationID, Quantity: line.Quantity, OrderLineID: line.OrderLineID})
		}
	}
	d.InventoryRecordID, err = postOperationInventory(ctx, tx, actor, "order", parent, "outbound", d.WarehouseID, d.ShippedDate, d.Note, stockLines)
	if err != nil {
		return d, err
	}
	if err = persistOperation(ctx, tx, d, actor, "shipped"); err != nil {
		return d, err
	}
	full := true
	for _, line := range order.Lines {
		ordered, _ := operationQuantity(line.Quantity, true)
		full = full && shipped[line.ID] != nil && shipped[line.ID].Cmp(ordered) == 0
	}
	if full {
		order.Status = "fulfilled"
	}
	order.UpdatedAt = operationUpdatedAt(s.now(), order.UpdatedAt)
	if err = persistOperation(ctx, tx, order, actor, "shipment_recorded"); err != nil {
		return d, err
	}
	return d, tx.Commit()
}
