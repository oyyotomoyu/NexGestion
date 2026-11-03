package system

import (
	"context"
	"database/sql"
	"math/big"
)

func sumCheckoutAmounts(ctx context.Context, tx *sql.Tx, query, id string) (*big.Rat, error) {
	rows, err := tx.QueryContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sum := new(big.Rat)
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return nil, err
		}
		r, err := parseCheckoutAmount(value)
		if err != nil {
			return nil, err
		}
		sum.Add(sum, r)
	}
	return sum, rows.Err()
}
func postCheckoutInventory(ctx context.Context, tx *sql.Tx, id, now string) error {
	var attached int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM pragma_database_list WHERE name='inventory'`).Scan(&attached); err != nil {
		return err
	}
	if attached == 0 {
		return nil
	}
	var warehouse, actor string
	if err := tx.QueryRowContext(ctx, `SELECT warehouse_id,cashier_user_id FROM checkout_transactions WHERE id=?`, id).Scan(&warehouse, &actor); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT inventory_item_id,quantity FROM checkout_transaction_lines WHERE checkout_transaction_id=? AND inventory_item_id IS NOT NULL`, id)
	if err != nil {
		return err
	}
	lines := []InventoryLine{}
	for rows.Next() {
		var line InventoryLine
		if err = rows.Scan(&line.ItemID, &line.Quantity); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, line)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	_, err = postOperationInventory(ctx, tx, actor, "checkout", id, "outbound", warehouse, now[:10], "Checkout sale", lines)
	return err
}
