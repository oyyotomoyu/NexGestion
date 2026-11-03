package apis

import (
	"errors"
	"net/http"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

func RegisterOperationsRoutes(router *http.ServeMux, orders, procurement, production *system.OperationsService, users *system.UserService, auth *system.AuthService, logService *applogs.Service) {
	catalog, err := system.LoadPermissionCatalog()
	if err != nil {
		panic(err)
	}
	protected := func(permission string, handler http.HandlerFunc) http.HandlerFunc {
		if !catalog.Contains(permission) {
			panic("undefined operations permission: " + permission)
		}
		return requireAuth(auth, withRequestLogger(logService, requirePermission(users, permission, handler)))
	}
	registerOperationHandlers(router, orders, procurement, production, protected)
}
func registerOperationHandlers(router *http.ServeMux, orders, procurement, production *system.OperationsService, protected func(string, http.HandlerFunc) http.HandlerFunc) {
	register := func(service *system.OperationsService, module, kind, path, key string, editable bool) {
		router.HandleFunc("GET "+path, protected(module+".read", func(w http.ResponseWriter, r *http.Request) {
			query, err := parseListQuery(r)
			if err != nil {
				writeListQueryError(w, err)
				return
			}
			result, err := service.List(r.Context(), kind, query)
			if err != nil {
				writeOperationError(w, err)
				return
			}
			writeJSON(w, 200, listResponse(key, result))
		}))
		router.HandleFunc("GET "+path+"/{id}", protected(module+".read", func(w http.ResponseWriter, r *http.Request) {
			result, err := service.Get(r.Context(), kind, r.PathValue("id"))
			if err != nil {
				writeOperationError(w, err)
				return
			}
			writeJSON(w, 200, result)
		}))
		router.HandleFunc("GET "+path+"/{id}/events", protected(module+".read", func(w http.ResponseWriter, r *http.Request) {
			query, err := parseListQuery(r)
			if err != nil {
				writeListQueryError(w, err)
				return
			}
			result, err := service.Events(r.Context(), kind, r.PathValue("id"), query)
			if err != nil {
				writeOperationError(w, err)
				return
			}
			writeJSON(w, 200, listResponse("events", result))
		}))
		if !editable {
			return
		}
		save := func(w http.ResponseWriter, r *http.Request) {
			var input system.OperationDocument
			if err := decodeJSON(w, r, &input); err != nil {
				writeJSON(w, 400, map[string]string{"code": "invalid_request", "error": err.Error()})
				return
			}
			d, err := service.Save(r.Context(), kind, r.PathValue("id"), authenticatedUserID(r), input)
			if err != nil {
				writeOperationError(w, err)
				return
			}
			recordRequestLog(r, "info", "saved "+kind+" "+d.ID)
			status := 200
			if r.Method == "POST" {
				status = 201
			}
			writeJSON(w, status, d)
		}
		router.HandleFunc("POST "+path, protected(module+".manage", save))
		router.HandleFunc("PUT "+path+"/{id}", protected(module+".manage", save))
		if kind != "vendor" {
			router.HandleFunc("PATCH "+path+"/{id}/status", protected(module+".manage", func(w http.ResponseWriter, r *http.Request) {
				var input struct {
					Status            string `json:"status"`
					ExpectedUpdatedAt string `json:"expected_updated_at"`
				}
				if err := decodeJSON(w, r, &input); err != nil {
					writeJSON(w, 400, map[string]string{"code": "invalid_request", "error": err.Error()})
					return
				}
				d, err := service.TransitionExpected(r.Context(), kind, r.PathValue("id"), authenticatedUserID(r), input.Status, input.ExpectedUpdatedAt)
				if err != nil {
					writeOperationError(w, err)
					return
				}
				recordRequestLog(r, "info", kind+" "+d.ID+" status "+d.Status)
				writeJSON(w, 200, d)
			}))
		}
	}
	register(orders, "orders", "quote", "/api/quotes", "quotes", true)
	register(orders, "orders", "order", "/api/orders", "orders", true)
	register(orders, "orders", "shipment", "/api/order-shipments", "shipments", false)
	register(procurement, "procurement", "vendor", "/api/procurement/vendors", "vendors", true)
	register(procurement, "procurement", "purchase_order", "/api/procurement/purchase-orders", "purchase_orders", true)
	register(procurement, "procurement", "goods_receipt", "/api/procurement/goods-receipts", "goods_receipts", false)
	register(procurement, "procurement", "vendor_invoice", "/api/procurement/vendor-invoices", "vendor_invoices", false)
	register(production, "production", "production_order", "/api/production/orders", "production_orders", true)
	action := func(path, permission string, fn func(*http.Request, system.OperationDocument) (system.OperationDocument, error)) {
		router.HandleFunc("POST "+path, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			var input system.OperationDocument
			if err := decodeJSON(w, r, &input); err != nil {
				writeJSON(w, 400, map[string]string{"code": "invalid_request", "error": err.Error()})
				return
			}
			result, err := fn(r, input)
			if err != nil {
				writeOperationError(w, err)
				return
			}
			recordRequestLog(r, "info", "created "+result.Kind+" "+result.ID)
			writeJSON(w, 201, result)
		}))
	}
	router.HandleFunc("GET /api/orders/{id}/shipping-options", protected("orders.manage", func(w http.ResponseWriter, r *http.Request) {
		result, err := orders.ShippingOptions(r.Context(), r.PathValue("id"))
		if err != nil {
			writeOperationError(w, err)
			return
		}
		writeJSON(w, 200, result)
	}))
	action("/api/orders/{id}/shipments", "orders.manage", func(r *http.Request, input system.OperationDocument) (system.OperationDocument, error) {
		return orders.Ship(r.Context(), authenticatedUserID(r), r.PathValue("id"), input, false)
	})
	action("/api/orders/{id}/fulfill", "orders.manage", func(r *http.Request, input system.OperationDocument) (system.OperationDocument, error) {
		return orders.Ship(r.Context(), authenticatedUserID(r), r.PathValue("id"), input, true)
	})
	action("/api/procurement/purchase-orders/{id}/receipts", "procurement.manage", func(r *http.Request, input system.OperationDocument) (system.OperationDocument, error) {
		return procurement.Receive(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
	})
	action("/api/procurement/purchase-orders/{id}/invoices", "procurement.manage", func(r *http.Request, input system.OperationDocument) (system.OperationDocument, error) {
		return procurement.Invoice(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
	})
}
func writeOperationError(w http.ResponseWriter, err error) {
	status, code, message := 500, "operations_internal_error", "internal server error"
	switch {
	case errors.Is(err, system.ErrInvalidListQuery), errors.Is(err, system.ErrOperationInvalid), errors.Is(err, system.ErrInventoryInvalid):
		status = 400
		code = "operations_invalid_input"
		message = err.Error()
	case errors.Is(err, system.ErrOperationNotFound), errors.Is(err, system.ErrInventoryNotFound):
		status = 404
		code = "operations_not_found"
		message = err.Error()
	case errors.Is(err, system.ErrOperationStale):
		status = 409
		code = "operations_stale_document"
		message = err.Error()
	case errors.Is(err, system.ErrOperationState), errors.Is(err, system.ErrInventoryState):
		status = 409
		code = "operations_invalid_state"
		message = err.Error()
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
