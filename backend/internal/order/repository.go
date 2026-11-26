package order

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxDBInteger int64 = 2147483647

var (
	ErrOrderTotalTooLarge    = errors.New("order total exceeds database limit")
	ErrOrderNotFound         = errors.New("order not found")
	ErrOrderAlreadyCancelled = errors.New("order already cancelled")
	ErrStockLimitExceeded    = errors.New("restored stock exceeds database limit")
)

type UnknownSKUError struct {
	SKU string
}

func (e *UnknownSKUError) Error() string {
	return "unknown SKU: " + e.SKU
}

type InsufficientStockError struct {
	SKU       string
	Available int64
}

func (e *InsufficientStockError) Error() string {
	return "insufficient stock for SKU: " + e.SKU
}

type lockedProduct struct {
	SKU   string
	Price int64
	Stock int64
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repo *Repository) Create(
	ctx context.Context,
	req CreateOrderRequest,
) (Order, error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin create order: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock Products in consistent SKU order.
	skus := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		skus = append(skus, item.SKU)
	}
	sort.Strings(skus)

	products := make(map[string]lockedProduct, len(skus))

	for _, sku := range skus {
		var item lockedProduct

		err := tx.QueryRow(ctx, `
			SELECT sku, price, stock
			FROM products
			WHERE sku = $1
			FOR UPDATE
		`, sku).Scan(
			&item.SKU,
			&item.Price,
			&item.Stock,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return Order{}, &UnknownSKUError{SKU: sku}
		}
		if err != nil {
			return Order{}, fmt.Errorf("lock product %s: %w", sku, err)
		}

		products[sku] = item
	}

	// Validate stock and calculate total before writing.
	var total int64

	for _, item := range req.Items {
		product := products[item.SKU]

		if product.Stock < item.Qty {
			return Order{}, &InsufficientStockError{
				SKU:       item.SKU,
				Available: product.Stock,
			}
		}

		if product.Price > 0 &&
			item.Qty > (maxDBInteger-total)/product.Price {
			return Order{}, ErrOrderTotalTooLarge
		}

		total += product.Price * item.Qty
	}

	// Create order inside the same transaction.
	var result Order

	err = tx.QueryRow(ctx, `
		INSERT INTO sales_orders (
			customer,
			status,
			total
		)
		VALUES ($1, 'CREATED', $2)
		RETURNING id, customer, status, total, created_at
	`, req.Customer, total).Scan(
		&result.ID,
		&result.Customer,
		&result.Status,
		&result.Total,
		&result.CreatedAt,
	)

	if err != nil {
		return Order{}, fmt.Errorf("insert sales order: %w", err)
	}

	result.Items = make([]OrderItem, 0, len(req.Items))

	// Decrease stock and persist order items atomically.
	for _, item := range req.Items {
		product := products[item.SKU]

		tag, err := tx.Exec(ctx, `
    UPDATE products
    SET stock = stock - $1
    WHERE sku = $2
      AND stock >= $1
`, item.Qty, item.SKU)

		if err != nil {
			return Order{}, fmt.Errorf("decrease stock for %s: %w", item.SKU, err)
		}

		if tag.RowsAffected() != 1 {
			return Order{}, &InsufficientStockError{
				SKU:       item.SKU,
				Available: product.Stock,
			}
		}

		var orderItem OrderItem

		err = tx.QueryRow(ctx, `
			INSERT INTO sales_order_items (
				order_id,
				sku,
				qty,
				unit_price
			)
			VALUES ($1, $2, $3, $4)
			RETURNING id, order_id, sku, qty, unit_price
		`, result.ID, item.SKU, item.Qty, product.Price).Scan(
			&orderItem.ID,
			&orderItem.OrderID,
			&orderItem.SKU,
			&orderItem.Qty,
			&orderItem.UnitPrice,
		)

		if err != nil {
			return Order{}, fmt.Errorf("insert order item: %w", err)
		}

		result.Items = append(result.Items, orderItem)
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit sales order: %w", err)
	}

	return result, nil
}

func (repo *Repository) GetByID(ctx context.Context, id int64) (Order, error) {
	var result Order

	err := repo.pool.QueryRow(ctx, `
        SELECT id, customer, status, total, created_at, cancelled_at
        FROM sales_orders
        WHERE id = $1
    `, id).Scan(
		&result.ID,
		&result.Customer,
		&result.Status,
		&result.Total,
		&result.CreatedAt,
		&result.CancelledAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}

	if err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}

	result.Items = make([]OrderItem, 0)

	rows, err := repo.pool.Query(ctx, `
        SELECT id, order_id, sku, qty, unit_price
        FROM sales_order_items
        WHERE order_id = $1
        ORDER BY id ASC
    `, id)

	if err != nil {
		return Order{}, fmt.Errorf("get order items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item OrderItem

		if err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.SKU,
			&item.Qty,
			&item.UnitPrice,
		); err != nil {
			return Order{}, fmt.Errorf("scan order item: %w", err)
		}

		result.Items = append(result.Items, item)
	}

	if err := rows.Err(); err != nil {
		return Order{}, fmt.Errorf("iterate order items: %w", err)
	}

	return result, nil
}

func (repo *Repository) Cancel(ctx context.Context, id int64) (Order, error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("begin cancel order transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock Order
	var status string

	err = tx.QueryRow(ctx, `
        SELECT status
        FROM sales_orders
        WHERE id = $1
        FOR UPDATE
    `, id).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}

	if err != nil {
		return Order{}, fmt.Errorf("lock order: %w", err)
	}

	if status == "CANCELLED" {
		return Order{}, ErrOrderAlreadyCancelled
	}

	// Get Order Items
	rows, err := tx.Query(ctx, `
        SELECT id, order_id, sku, qty, unit_price
        FROM sales_order_items
        WHERE order_id = $1
        ORDER BY sku ASC
    `, id)

	if err != nil {
		return Order{}, fmt.Errorf("get order items for cancellation: %w", err)
	}

	items := make([]OrderItem, 0)
	skuSet := make(map[string]bool)

	for rows.Next() {
		var item OrderItem

		if err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.SKU,
			&item.Qty,
			&item.UnitPrice,
		); err != nil {
			rows.Close()
			return Order{}, fmt.Errorf("scan order item: %w", err)
		}

		items = append(items, item)
		skuSet[item.SKU] = true
	}

	rowsErr := rows.Err()
	rows.Close()

	if rowsErr != nil {
		return Order{}, fmt.Errorf("iterate order items: %w", rowsErr)
	}

	// Lock Products in SKU order
	skus := make([]string, 0, len(skuSet))

	for sku := range skuSet {
		skus = append(skus, sku)
	}

	sort.Strings(skus)

	stocks := make(map[string]int64, len(skus))

	for _, sku := range skus {
		var stock int64

		err := tx.QueryRow(ctx, `
            SELECT stock
            FROM products
            WHERE sku = $1
            FOR UPDATE
        `, sku).Scan(&stock)

		if err != nil {
			return Order{}, fmt.Errorf("lock product %s: %w", sku, err)
		}

		stocks[sku] = stock
	}

	// Restore Stock
	for _, item := range items {
		currentStock := stocks[item.SKU]

		if item.Qty > maxDBInteger-currentStock {
			return Order{}, ErrStockLimitExceeded
		}

		tag, err := tx.Exec(ctx, `
            UPDATE products
            SET stock = stock + $1
            WHERE sku = $2
              AND stock <= $3
        `, item.Qty, item.SKU, maxDBInteger-item.Qty)

		if err != nil {
			return Order{}, fmt.Errorf("restore stock for %s: %w", item.SKU, err)
		}

		if tag.RowsAffected() != 1 {
			return Order{}, ErrStockLimitExceeded
		}

		stocks[item.SKU] = currentStock + item.Qty
	}

	// Mark Order as Cancelled
	var result Order

	err = tx.QueryRow(ctx, `
        UPDATE sales_orders
        SET status = 'CANCELLED',
            cancelled_at = NOW()
        WHERE id = $1
          AND status = 'CREATED'
        RETURNING id, customer, status, total, created_at, cancelled_at
    `, id).Scan(
		&result.ID,
		&result.Customer,
		&result.Status,
		&result.Total,
		&result.CreatedAt,
		&result.CancelledAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderAlreadyCancelled
	}

	if err != nil {
		return Order{}, fmt.Errorf("update order status: %w", err)
	}

	result.Items = items

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit cancel order transaction: %w", err)
	}

	return result, nil
}
