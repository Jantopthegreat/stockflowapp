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
	ErrOrderUnknownSKU    = errors.New("unknown SKU")
	ErrOrderTotalTooLarge = errors.New("order total exceeds database limit")
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
