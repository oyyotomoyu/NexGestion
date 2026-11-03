package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func operationsFixture(t *testing.T, inventory bool) (string, *OperationsService, *OperationsService, *OperationsService, *InventoryService) {
	t.Helper()
	dir := t.TempDir()
	specs := []DatabaseSpec{operationsDatabaseSpec("order.db"), operationsDatabaseSpec("procurement.db"), operationsDatabaseSpec("production.db")}
	if inventory {
		specs = append(specs, inventoryDatabaseSpec())
	}
	if err := EnsureDatabases(context.Background(), dir, specs); err != nil {
		t.Fatal(err)
	}
	order, proc, prod := NewOrderService(dir), NewProcurementService(dir), NewProductionService(dir)
	now := func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }
	order.now = now
	proc.now = now
	prod.now = now
	return dir, order, proc, prod, NewInventoryService(dir)
}
func operationTestItem(t *testing.T, s *InventoryService, sku string) InventoryEntity {
	t.Helper()
	item, err := s.SaveEntity(context.Background(), "items", InventoryEntity{Name: sku, SKU: sku, Unit: "piece", Barcode: sku + "-bar"})
	if err != nil {
		t.Fatal(err)
	}
	return item
}
func operationTestOrder(item string) OperationDocument {
	return OperationDocument{CustomerName: "Customer snapshot", OrderDate: "2026-10-19", QuoteDate: "2026-10-19", Currency: "TWD", Lines: []OperationLine{{Description: "Goods", InventoryItemID: item, Quantity: "3", UnitPrice: "0.1"}}}
}
func operationTestTransition(t *testing.T, s *OperationsService, d OperationDocument, status string) OperationDocument {
	t.Helper()
	result, err := s.Transition(context.Background(), d.Kind, d.ID, "actor", status)
	if err != nil {
		t.Fatalf("%s -> %s: %v", d.Kind, status, err)
	}
	return result
}
func operationTestBalance(t *testing.T, s *InventoryService, item, want string) {
	t.Helper()
	stock, err := s.Stock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := "0"
	for _, v := range stock {
		if v.ItemID == item {
			got = v.Quantity
		}
	}
	if got != want {
		t.Fatalf("item %s stock=%s want=%s", item, got, want)
	}
}

func TestOperationsStandaloneAndQuoteAcceptance(t *testing.T) {
	ctx := context.Background()
	dir, orders, proc, prod, _ := operationsFixture(t, false)
	quote, err := orders.Save(ctx, "quote", "", "actor", operationTestOrder("uninstalled-item"))
	if err != nil {
		t.Fatal(err)
	}
	if quote.TotalAmount != "0.3" {
		t.Fatal(quote.TotalAmount)
	}
	if _, err = orders.Transition(ctx, "quote", quote.ID, "actor", "accepted"); !errors.Is(err, ErrOperationState) {
		t.Fatalf("draft accepted: %v", err)
	}
	quote = operationTestTransition(t, orders, quote, "sent")
	quote = operationTestTransition(t, orders, quote, "accepted")
	retry := operationTestTransition(t, orders, quote, "accepted")
	if retry.ResultingOrderID != quote.ResultingOrderID {
		t.Fatal("quote created second order")
	}
	order, err := orders.Get(ctx, "order", quote.ResultingOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.SourceQuoteID != quote.ID || order.CustomerName != quote.CustomerName || order.TotalAmount != "0.3" {
		t.Fatal(order)
	}
	order = operationTestTransition(t, orders, order, "confirmed")
	if _, err = orders.Ship(ctx, "actor", order.ID, OperationDocument{}, true); err != nil {
		t.Fatal(err)
	}
	order, err = orders.Get(ctx, "order", order.ID)
	if err != nil || order.Status != "fulfilled" {
		t.Fatal(order, err)
	}
	run, err := prod.Save(ctx, "production_order", "", "actor", OperationDocument{OutputDescription: "Assembly", OutputInventoryItemID: "absent", OutputQuantity: "1", Materials: []OperationLine{{Description: "Material", InventoryItemID: "absent", QuantityUsed: "1"}}})
	if err != nil {
		t.Fatal(err)
	}
	run = operationTestTransition(t, prod, run, "in_progress")
	run = operationTestTransition(t, prod, run, "completed")
	if run.CompletedAt == "" {
		t.Fatal("completion timestamp missing")
	}
	po, err := proc.Save(ctx, "purchase_order", "", "actor", OperationDocument{VendorName: "Vendor snapshot", OrderDate: "2026-10-19", Currency: "TWD", Lines: []OperationLine{{Description: "Stockless goods", Quantity: "1", UnitPrice: "10"}}})
	if err != nil {
		t.Fatal(err)
	}
	po = operationTestTransition(t, proc, po, "approved")
	po = operationTestTransition(t, proc, po, "sent")
	if _, err = proc.Receive(ctx, "actor", po.ID, OperationDocument{ReceivedDate: "2026-10-19", Lines: []OperationLine{{PurchaseOrderLineID: po.Lines[0].ID, QuantityReceived: "1", QuantityAccepted: "1"}}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"inventory.db", "crm.db", "finance.db"} {
		if _, err = os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created optional database %s: %v", name, err)
		}
	}
	expired := operationTestOrder("")
	expired.QuoteDate = "2026-10-01"
	expired.ValidUntil = "2026-10-18"
	q, err := orders.Save(ctx, "quote", "", "actor", expired)
	if err != nil {
		t.Fatal(err)
	}
	q, err = orders.Get(ctx, "quote", q.ID)
	if err != nil || q.Status != "expired" {
		t.Fatal(q, err)
	}
	if _, err = orders.Transition(ctx, "quote", q.ID, "actor", "sent"); !errors.Is(err, ErrOperationState) {
		t.Fatal(err)
	}
}
func TestOperationsProcurementReceiptsAndInvoiceMatch(t *testing.T) {
	ctx := context.Background()
	_, _, proc, _, inventory := operationsFixture(t, true)
	item := operationTestItem(t, inventory, "RAW")
	vendor, err := proc.Save(ctx, "vendor", "", "actor", OperationDocument{Name: "Supplier", PaymentTerms: "Net 30"})
	if err != nil {
		t.Fatal(err)
	}
	po, err := proc.Save(ctx, "purchase_order", "", "actor", OperationDocument{VendorID: vendor.ID, OrderDate: "2026-10-19", Currency: "TWD", Lines: []OperationLine{{Description: "Raw material", InventoryItemID: item.ID, Quantity: "4", UnitPrice: "2.5"}}})
	if err != nil {
		t.Fatal(err)
	}
	if po.VendorName != "Supplier" {
		t.Fatal(po)
	}
	po = operationTestTransition(t, proc, po, "approved")
	po = operationTestTransition(t, proc, po, "sent")
	invoice, err := proc.Invoice(ctx, "actor", po.ID, OperationDocument{VendorInvoiceNumber: "V-001", InvoiceDate: "2026-10-19", InvoiceAmount: "10", Currency: "TWD"})
	if err != nil || invoice.MatchStatus != "pending" {
		t.Fatal(invoice, err)
	}
	receipt := OperationDocument{RequestKey: "delivery-1", WarehouseID: "default", ReceivedDate: "2026-10-19", Lines: []OperationLine{{PurchaseOrderLineID: po.Lines[0].ID, QuantityReceived: "3", QuantityAccepted: "2", QuantityRejected: "1", RejectionReason: "Damaged"}}}
	first, err := proc.Receive(ctx, "actor", po.ID, receipt)
	if err != nil || first.InventoryRecordID == "" {
		t.Fatal(first, err)
	}
	operationTestBalance(t, inventory, item.ID, "2")
	if _, err = proc.Receive(ctx, "actor", po.ID, receipt); !errors.Is(err, ErrOperationState) {
		t.Fatalf("duplicate receipt: %v", err)
	}
	operationTestBalance(t, inventory, item.ID, "2")
	if _, err = proc.Transition(ctx, "purchase_order", po.ID, "actor", "closed"); !errors.Is(err, ErrOperationState) {
		t.Fatal(err)
	}
	receipt.RequestKey = "delivery-2"
	receipt.Lines[0].QuantityReceived = "2"
	receipt.Lines[0].QuantityAccepted = "2"
	receipt.Lines[0].QuantityRejected = "0"
	if _, err = proc.Receive(ctx, "actor", po.ID, receipt); err != nil {
		t.Fatal(err)
	}
	operationTestBalance(t, inventory, item.ID, "4")
	matched, err := proc.Get(ctx, "vendor_invoice", invoice.ID)
	if err != nil || matched.MatchStatus != "matched" {
		t.Fatal(matched, err)
	}
	po, err = proc.Get(ctx, "purchase_order", po.ID)
	if err != nil || !po.FullyReceived {
		t.Fatal(po, err)
	}
	receipt.RequestKey = "delivery-3"
	if _, err = proc.Receive(ctx, "actor", po.ID, receipt); !errors.Is(err, ErrOperationState) {
		t.Fatalf("over receipt: %v", err)
	}
	operationTestBalance(t, inventory, item.ID, "4")
	operationTestTransition(t, proc, po, "closed")
	if _, err = proc.Invoice(ctx, "actor", po.ID, OperationDocument{VendorInvoiceNumber: "V-001", InvoiceDate: "2026-10-19", InvoiceAmount: "10", Currency: "TWD"}); !errors.Is(err, ErrOperationState) {
		t.Fatalf("duplicate invoice: %v", err)
	}
}
func TestOperationsInventoryRollbackAndPartialShipment(t *testing.T) {
	ctx := context.Background()
	_, orders, _, _, inventory := operationsFixture(t, true)
	a := operationTestItem(t, inventory, "A")
	b := operationTestItem(t, inventory, "B")
	if _, err := inventory.Post(ctx, "actor", InventoryRecord{Kind: "inbound", WarehouseID: "default", Date: "2026-10-19", Lines: []InventoryLine{{ItemID: a.ID, Quantity: "3"}}}); err != nil {
		t.Fatal(err)
	}
	input := operationTestOrder(a.ID)
	input.Lines = append(input.Lines, OperationLine{Description: "B", InventoryItemID: b.ID, Quantity: "1", UnitPrice: "10"})
	order, err := orders.Save(ctx, "order", "", "actor", input)
	if err != nil {
		t.Fatal(err)
	}
	order = operationTestTransition(t, orders, order, "confirmed")
	if _, err = orders.Ship(ctx, "actor", order.ID, OperationDocument{WarehouseID: "default"}, true); !errors.Is(err, ErrInventoryState) {
		t.Fatalf("insufficient stock: %v", err)
	}
	operationTestBalance(t, inventory, a.ID, "3")
	shipments, err := orders.List(ctx, "shipment", ListQuery{})
	if err != nil || shipments.Pagination.Total != 0 {
		t.Fatal(shipments, err)
	}
	if _, err = orders.Ship(ctx, "actor", order.ID, OperationDocument{WarehouseID: "default", Lines: []OperationLine{{OrderLineID: order.Lines[0].ID, Quantity: "1"}}}, false); err != nil {
		t.Fatal(err)
	}
	operationTestBalance(t, inventory, a.ID, "2")
	if _, err = orders.Transition(ctx, "order", order.ID, "actor", "cancelled"); !errors.Is(err, ErrOperationState) {
		t.Fatal(err)
	}
	if _, err = inventory.Post(ctx, "actor", InventoryRecord{Kind: "inbound", WarehouseID: "default", Date: "2026-10-19", Lines: []InventoryLine{{ItemID: b.ID, Quantity: "1"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = orders.Ship(ctx, "actor", order.ID, OperationDocument{WarehouseID: "default"}, true); err != nil {
		t.Fatal(err)
	}
	operationTestBalance(t, inventory, a.ID, "0")
	operationTestBalance(t, inventory, b.ID, "0")
	order, err = orders.Get(ctx, "order", order.ID)
	if err != nil || order.Status != "fulfilled" {
		t.Fatal(order, err)
	}
	if _, err = orders.Ship(ctx, "actor", order.ID, OperationDocument{WarehouseID: "default"}, true); !errors.Is(err, ErrOperationState) {
		t.Fatalf("double fulfillment: %v", err)
	}
}
func TestOperationsProductionAtomicCompletionAndConcurrency(t *testing.T) {
	ctx := context.Background()
	_, _, _, prod, inventory := operationsFixture(t, true)
	raw := operationTestItem(t, inventory, "Raw")
	output := operationTestItem(t, inventory, "Finished")
	if _, err := inventory.Post(ctx, "actor", InventoryRecord{Kind: "inbound", WarehouseID: "default", Date: "2026-10-19", Lines: []InventoryLine{{ItemID: raw.ID, Quantity: "2"}}}); err != nil {
		t.Fatal(err)
	}
	create := func(quantity string) OperationDocument {
		t.Helper()
		d, err := prod.Save(ctx, "production_order", "", "actor", OperationDocument{WarehouseID: "default", OutputDescription: "Finished", OutputInventoryItemID: output.ID, OutputQuantity: "1", Materials: []OperationLine{{Description: "Raw", InventoryItemID: raw.ID, QuantityUsed: quantity}}})
		if err != nil {
			t.Fatal(err)
		}
		return operationTestTransition(t, prod, d, "in_progress")
	}
	bad := create("3")
	if _, err := prod.Transition(ctx, "production_order", bad.ID, "actor", "completed"); !errors.Is(err, ErrInventoryState) {
		t.Fatal(err)
	}
	operationTestBalance(t, inventory, raw.ID, "2")
	operationTestBalance(t, inventory, output.ID, "0")
	run := create("2")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := prod.Transition(ctx, "production_order", run.ID, "actor", "completed")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrOperationState) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("completed %d times", success)
	}
	operationTestBalance(t, inventory, raw.ID, "0")
	operationTestBalance(t, inventory, output.ID, "1")
	movements, err := inventory.Movements(ctx)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, m := range movements {
		types[m.MovementType]++
	}
	if types["production_input"] != 1 || types["production_output"] != 1 {
		t.Fatal(types)
	}
}

func TestOperationsDraftValidationAndAudit(t *testing.T) {
	ctx := context.Background()
	_, orders, _, _, _ := operationsFixture(t, false)
	for _, bad := range []OperationDocument{
		{CustomerName: "", OrderDate: "2026-10-19", Currency: "TWD", Lines: []OperationLine{{Description: "X", Quantity: "1", UnitPrice: "1"}}},
		{CustomerName: "X", OrderDate: "not-date", Currency: "TWD", Lines: []OperationLine{{Description: "X", Quantity: "1", UnitPrice: "1"}}},
		{CustomerName: "X", OrderDate: "2026-10-19", Currency: "invalid", Lines: []OperationLine{{Description: "X", Quantity: "1", UnitPrice: "1"}}},
		{CustomerName: "X", OrderDate: "2026-10-19", Currency: "TWD", Lines: []OperationLine{{Description: "X", Quantity: "-1", UnitPrice: "1"}}},
	} {
		if _, err := orders.Save(ctx, "order", "", "actor", bad); !errors.Is(err, ErrOperationInvalid) {
			t.Fatalf("accepted invalid order: %v", err)
		}
	}
	order, err := orders.Save(ctx, "order", "", "actor", operationTestOrder(""))
	if err != nil {
		t.Fatal(err)
	}
	order = operationTestTransition(t, orders, order, "confirmed")
	if _, err = orders.Save(ctx, "order", order.ID, "actor", operationTestOrder("")); !errors.Is(err, ErrOperationState) {
		t.Fatal("edited confirmed order", err)
	}
	events, err := orders.Events(ctx, "order", order.ID, ListQuery{PageSize: 1, Order: "asc"})
	if err != nil || events.Pagination.Total != 2 || len(events.Items) != 1 || events.Items[0].ActorUserID != "actor" {
		t.Fatal(events, err)
	}
	events, err = orders.Events(ctx, "order", order.ID, ListQuery{Keyword: "status:confirmed"})
	if err != nil || events.Pagination.Total != 1 {
		t.Fatal(events, err)
	}
	list, err := orders.List(ctx, "order", ListQuery{Keyword: "Customer", Filters: map[string]string{"status": "confirmed"}})
	if err != nil || list.Pagination.Total != 1 {
		t.Fatal(list, err)
	}
	db, _, err := orders.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, `DELETE FROM operation_events`); err == nil {
		t.Fatal("deleted audit history")
	}
}
