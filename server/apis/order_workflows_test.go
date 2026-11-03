package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

func TestOrderUIAPIContractAndOptimisticConflicts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("NEXGESTION_ADMIN_PASSWORD", "a-secure-test-password")
	t.Setenv("NEXGESTION_ADMIN_EMAIL", "admin@nexgestion.local")
	if err := system.EnsureRequiredDatabases(ctx, dir); err != nil {
		t.Fatal(err)
	}
	users := system.NewUserService(dir)
	auth := system.NewAuthService(users)
	logger, err := applogs.NewService(t.TempDir(), time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	router := http.NewServeMux()
	RegisterOperationsRoutes(router, system.NewOrderService(dir), system.NewProcurementService(dir), system.NewProductionService(dir), users, auth, logger)
	admin, err := auth.Login(ctx, "admin@nexgestion.local", "a-secure-test-password", "127.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token string, input any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	decode := func(w *httptest.ResponseRecorder, status int) system.OperationDocument {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var d system.OperationDocument
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	input := system.OperationDocument{CustomerName: "UI customer", OrderDate: "2026-10-19", Currency: "TWD", Lines: []system.OperationLine{{Description: "Service", Quantity: "3", UnitPrice: "0.1"}}}
	order := decode(request("POST", "/api/orders", admin.AccessToken, input), 201)
	input.ExpectedUpdatedAt = order.UpdatedAt
	input.Note = "First edit"
	updated := decode(request("PUT", "/api/orders/"+order.ID, admin.AccessToken, input), 200)
	stale := request("PUT", "/api/orders/"+order.ID, admin.AccessToken, input)
	if stale.Code != 409 {
		t.Fatal(stale.Code, stale.Body.String())
	}
	var body map[string]string
	if err = json.Unmarshal(stale.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "operations_stale_document" {
		t.Fatal(body)
	}
	order = decode(request("PATCH", "/api/orders/"+order.ID+"/status", admin.AccessToken, map[string]string{"status": "confirmed", "expected_updated_at": updated.UpdatedAt}), 200)
	opts := request("GET", "/api/orders/"+order.ID+"/shipping-options", admin.AccessToken, nil)
	if opts.Code != 200 {
		t.Fatal(opts.Code, opts.Body.String())
	}
	var choices system.OrderShippingOptions
	if err = json.Unmarshal(opts.Body.Bytes(), &choices); err != nil || !choices.InventoryAvailable || len(choices.Warehouses) != 1 {
		t.Fatal(choices, err)
	}
	if w := request("GET", "/api/orders/"+order.ID+"/shipping-options", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	input = system.OperationDocument{ExpectedUpdatedAt: order.UpdatedAt, RequestKey: "ui-shipment", Lines: []system.OperationLine{{OrderLineID: order.Lines[0].ID, Quantity: "1"}}}
	decode(request("POST", "/api/orders/"+order.ID+"/shipments", admin.AccessToken, input), 201)
	order = decode(request("GET", "/api/orders/"+order.ID, admin.AccessToken, nil), 200)
	if order.Lines[0].FulfilledQuantity != "1" || order.Lines[0].RemainingQuantity != "2" || order.Status != "confirmed" {
		t.Fatal(order)
	}
	if w := request("POST", "/api/orders/"+order.ID+"/shipments", admin.AccessToken, input); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	events := request("GET", "/api/orders/"+order.ID+"/events", admin.AccessToken, nil)
	if events.Code != 200 {
		t.Fatal(events.Code, events.Body.String())
	}
}
