package system

import "context"

type OrderShippingOptions struct {
	InventoryAvailable bool              `json:"inventory_available"`
	Warehouses         []InventoryEntity `json:"warehouses"`
	Locations          []InventoryEntity `json:"locations"`
}

// Order operators can select warehouse destinations without a separate stock-read grant.
// This exposes location names only, never balances or the item catalog.
func (s *OperationsService) ShippingOptions(ctx context.Context, id string) (OrderShippingOptions, error) {
	result := OrderShippingOptions{Warehouses: []InventoryEntity{}, Locations: []InventoryEntity{}}
	if s.module != "order" {
		return result, ErrOperationInvalid
	}
	if _, err := s.Get(ctx, "order", id); err != nil {
		return result, err
	}
	db, available, err := s.open(ctx)
	if err != nil {
		return result, err
	}
	db.Close()
	result.InventoryAvailable = available
	if !available {
		return result, nil
	}
	inventory := NewInventoryService(s.directory)
	warehouses, err := inventory.Entities(ctx, "warehouses")
	if err != nil {
		return result, err
	}
	locations, err := inventory.Entities(ctx, "locations")
	if err != nil {
		return result, err
	}
	active := map[string]bool{}
	for _, w := range warehouses {
		if w.Status == "active" {
			result.Warehouses = append(result.Warehouses, w)
			active[w.ID] = true
		}
	}
	for _, l := range locations {
		if l.Status == "active" && active[l.WarehouseID] {
			result.Locations = append(result.Locations, l)
		}
	}
	return result, nil
}
