package apis

import (
	"errors"
	"net/http"
	"nexgestion/server/system"
)

// RegisterInventoryRoutes uses the same authentication and catalog checks as all other modules.
func RegisterInventoryRoutes(router *http.ServeMux, inventory *system.InventoryService, protected func(string, http.HandlerFunc) http.HandlerFunc) {
	handle := func(pattern, permission string, fn func(*http.Request) (any, error)) {
		router.HandleFunc(pattern, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			result, err := fn(r)
			if err != nil {
				status := http.StatusInternalServerError
				code := "inventory_internal_error"
				message := "internal server error"
				switch {
				case errors.Is(err, system.ErrInventoryInvalid):
					status = 400
					code = "inventory_invalid_input"
					message = err.Error()
				case errors.Is(err, system.ErrInventoryNotFound):
					status = 404
					code = "inventory_not_found"
					message = err.Error()
				case errors.Is(err, system.ErrInventoryState):
					status = 409
					code = "inventory_invalid_state"
					message = err.Error()
				}
				writeJSON(w, status, map[string]string{"code": code, "error": message})
				return
			}
			status := http.StatusOK
			if r.Method == "POST" {
				status = http.StatusCreated
				recordRequestLog(r, "info", "inventory operation "+r.URL.Path)
			}
			writeJSON(w, status, result)
		}))
	}
	for _, kind := range []string{"warehouses", "locations", "items"} {
		kind := kind
		handle("GET /api/inventory/"+kind, "inventory.read", func(r *http.Request) (any, error) { return inventory.Entities(r.Context(), kind) })
		save := func(r *http.Request) (any, error) {
			var input system.InventoryEntity
			if err := decodeJSON(nil, r, &input); err != nil {
				return nil, system.ErrInventoryInvalid
			}
			input.ID = r.PathValue("id")
			return inventory.SaveEntity(r.Context(), kind, input)
		}
		handle("POST /api/inventory/"+kind, "inventory.manage", save)
		handle("PUT /api/inventory/"+kind+"/{id}", "inventory.manage", save)
	}
	handle("GET /api/inventory/scan", "inventory.read", func(r *http.Request) (any, error) { return inventory.Scan(r.Context(), r.URL.Query().Get("code")) })
	handle("GET /api/inventory/movements", "inventory.read", func(r *http.Request) (any, error) { return inventory.Movements(r.Context()) })
	handle("GET /api/inventory/stock", "inventory.read", func(r *http.Request) (any, error) { return inventory.Stock(r.Context()) })
	handle("GET /api/inventory/records", "inventory.read", func(r *http.Request) (any, error) { return inventory.Records(r.Context()) })
	handle("POST /api/inventory/records", "inventory.manage", func(r *http.Request) (any, error) {
		var input system.InventoryRecord
		if err := decodeJSON(nil, r, &input); err != nil {
			return nil, system.ErrInventoryInvalid
		}
		return inventory.Post(r.Context(), authenticatedUserID(r), input)
	})
	handle("PATCH /api/inventory/records/{id}", "inventory.manage", func(r *http.Request) (any, error) {
		var input struct {
			Status string `json:"status"`
		}
		if err := decodeJSON(nil, r, &input); err != nil {
			return nil, system.ErrInventoryInvalid
		}
		return inventory.Transition(r.Context(), authenticatedUserID(r), r.PathValue("id"), input.Status)
	})
}
