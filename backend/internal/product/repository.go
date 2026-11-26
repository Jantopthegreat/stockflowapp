package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Error handling Product
var (
	ErrDuplicateSKU           = errors.New("SKU already exists")
	ErrProductNotFound        = errors.New("product not found")
	ErrProductHasHistory      = errors.New("product has stock movement history")
	ErrProductHasSalesHistory = errors.New("product has sales history")
)

// Error handling Stock
var ErrStockLimitExceeded = errors.New("stock limit exceeded")

type InsufficientStockError struct {
	Available int64
}

func (e *InsufficientStockError) Error() string {
	return "insufficient stock"
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repo *Repository) List(
	ctx context.Context,
	search string,
	limit int,
	offset int,
) ([]Product, int64, error) {
	var total int64

	countQuery := `
		SELECT COUNT(*)
		FROM products
		WHERE
			$1 = ''
			OR sku ILIKE '%' || $1 || '%'
			OR name ILIKE '%' || $1 || '%'
	`

	if err := repo.pool.QueryRow(ctx, countQuery, search).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	query := `
		SELECT
			p.id,
			p.sku,
			p.name,
			p.price,
			p.stock,
			p.is_active,
			p.created_at,
			(
				SELECT MAX(sm.created_at)
				FROM stock_movements sm
				WHERE sm.product_id = p.id
			) AS last_movement_at
		FROM products p
		WHERE
			$1 = ''
			OR p.sku ILIKE '%' || $1 || '%'
			OR p.name ILIKE '%' || $1 || '%'
		ORDER BY p.id ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := repo.pool.Query(ctx, query, search, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	products := make([]Product, 0)

	for rows.Next() {
		var item Product
		var lastMovementAt pgtype.Timestamptz

		if err := rows.Scan(
			&item.ID,
			&item.SKU,
			&item.Name,
			&item.Price,
			&item.Stock,
			&item.IsActive,
			&item.CreatedAt,
			&lastMovementAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan product: %w", err)
		}

		if lastMovementAt.Valid {
			item.LastMovementAt = &lastMovementAt.Time
		}

		products = append(products, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate products: %w", err)
	}

	return products, total, nil
}

func (repo *Repository) Create(
	ctx context.Context,
	sku string,
	name string,
	price int64,
) (Product, error) {
	query := `
		INSERT INTO products (sku, name, price)
		VALUES ($1, $2, $3)
		RETURNING
			id,
			sku,
			name,
			price,
			stock,
			is_active,
			created_at
	`

	var item Product

	err := repo.pool.QueryRow(
		ctx,
		query,
		sku,
		name,
		price,
	).Scan(
		&item.ID,
		&item.SKU,
		&item.Name,
		&item.Price,
		&item.Stock,
		&item.IsActive,
		&item.CreatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "products_sku_key" {
			return Product{}, ErrDuplicateSKU
		}

		return Product{}, fmt.Errorf("create product: %w", err)
	}

	return item, nil
}

func (repo *Repository) Update(
	ctx context.Context,
	id int64,
	sku string,
	name string,
	price int64,
	isActive bool,
) (Product, error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return Product{}, fmt.Errorf("begin update product: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentSKU string

	err = tx.QueryRow(ctx, `
		SELECT sku
		FROM products
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&currentSKU)

	if errors.Is(err, pgx.ErrNoRows) {
		return Product{}, ErrProductNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("lock product: %w", err)
	}

	if sku != currentSKU {
		var hasSalesHistory bool

		err = tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM sales_order_items
				WHERE sku = $1
			)
		`, currentSKU).Scan(&hasSalesHistory)

		if err != nil {
			return Product{}, fmt.Errorf("check sales history: %w", err)
		}

		if hasSalesHistory {
			return Product{}, ErrProductHasSalesHistory
		}
	}

	var item Product
	var lastMovementAt pgtype.Timestamptz

	err = tx.QueryRow(ctx, `
		UPDATE products
		SET sku = $1,
			name = $2,
			price = $3,
			is_active = $4
		WHERE id = $5
		RETURNING
			id,
			sku,
			name,
			price,
			stock,
			is_active,
			created_at,
			(
				SELECT MAX(sm.created_at)
				FROM stock_movements sm
				WHERE sm.product_id = products.id
			)
	`, sku, name, price, isActive, id).Scan(
		&item.ID,
		&item.SKU,
		&item.Name,
		&item.Price,
		&item.Stock,
		&item.IsActive,
		&item.CreatedAt,
		&lastMovementAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "products_sku_key" {
			return Product{}, ErrDuplicateSKU
		}

		return Product{}, fmt.Errorf("update product: %w", err)
	}

	if lastMovementAt.Valid {
		item.LastMovementAt = &lastMovementAt.Time
	}

	if err := tx.Commit(ctx); err != nil {
		return Product{}, fmt.Errorf("commit update product: %w", err)
	}

	return item, nil
}

func (repo *Repository) Delete(
	ctx context.Context,
	id int64,
) error {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete product: %w", err)
	}
	defer tx.Rollback(ctx)

	var sku string

	err = tx.QueryRow(ctx, `
		SELECT sku
		FROM products
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&sku)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProductNotFound
	}

	if err != nil {
		return fmt.Errorf("lock product: %w", err)
	}

	var hasMovement bool

	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM stock_movements
			WHERE product_id = $1
		)
	`, id).Scan(&hasMovement)

	if err != nil {
		return fmt.Errorf("check stock movements: %w", err)
	}

	if hasMovement {
		return ErrProductHasHistory
	}

	var hasSalesHistory bool

	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sales_order_items
			WHERE sku = $1
		)
	`, sku).Scan(&hasSalesHistory)

	if err != nil {
		return fmt.Errorf("check sales history: %w", err)
	}

	if hasSalesHistory {
		return ErrProductHasSalesHistory
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM products
		WHERE id = $1
	`, id)

	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete product: %w", err)
	}

	return nil
}
func (repo *Repository) AdjustStock(
	ctx context.Context,
	id int64,
	movementType string,
	qty int64,
	note string,
) (StockMovement, int64, error) {
	tx, err := repo.pool.Begin(ctx)
	if err != nil {
		return StockMovement{}, 0, fmt.Errorf("begin stock adjustment: %w", err)
	}
	defer tx.Rollback(ctx)

	var available int64

	err = tx.QueryRow(ctx, `
		SELECT stock
		FROM products
		WHERE id = $1
		FOR UPDATE
	`, id).Scan(&available)

	if errors.Is(err, pgx.ErrNoRows) {
		return StockMovement{}, 0, ErrProductNotFound
	}
	if err != nil {
		return StockMovement{}, 0, fmt.Errorf("lock product: %w", err)
	}

	const maxStock int64 = 2147483647

	var newStock int64

	if movementType == "in" {
		if available > maxStock-qty {
			return StockMovement{}, available, ErrStockLimitExceeded
		}

		err = tx.QueryRow(ctx, `
			UPDATE products
			SET stock = stock + $1
			WHERE id = $2
			  AND stock <= $3
			RETURNING stock
		`, qty, id, maxStock-qty).Scan(&newStock)
	} else {
		if available < qty {
			return StockMovement{}, available, &InsufficientStockError{
				Available: available,
			}
		}

		err = tx.QueryRow(ctx, `
			UPDATE products
			SET stock = stock - $1
			WHERE id = $2
			  AND stock >= $1
			RETURNING stock
		`, qty, id).Scan(&newStock)
	}

	if err != nil {
		return StockMovement{}, available, fmt.Errorf("update product stock: %w", err)
	}

	var movement StockMovement

	err = tx.QueryRow(ctx, `
		INSERT INTO stock_movements (
			product_id,
			type,
			qty,
			note
		)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING
			id,
			product_id,
			type,
			qty,
			COALESCE(note, ''),
			created_at
	`, id, movementType, qty, note).Scan(
		&movement.ID,
		&movement.ProductID,
		&movement.Type,
		&movement.Qty,
		&movement.Note,
		&movement.CreatedAt,
	)

	if err != nil {
		return StockMovement{}, 0, fmt.Errorf("create stock movement: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return StockMovement{}, 0, fmt.Errorf("commit stock adjustment: %w", err)
	}

	return movement, newStock, nil
}
func (repo *Repository) ListMovements(
	ctx context.Context,
	id int64,
) ([]StockMovement, error) {
	var productID int64

	err := repo.pool.QueryRow(ctx, `
		SELECT id
		FROM products
		WHERE id = $1
	`, id).Scan(&productID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("check product: %w", err)
	}

	rows, err := repo.pool.Query(ctx, `
		SELECT
			id,
			product_id,
			type,
			qty,
			COALESCE(note, ''),
			created_at
		FROM stock_movements
		WHERE product_id = $1
		ORDER BY created_at DESC, id DESC
	`, id)

	if err != nil {
		return nil, fmt.Errorf("list stock movements: %w", err)
	}
	defer rows.Close()

	movements := make([]StockMovement, 0)

	for rows.Next() {
		var movement StockMovement

		if err := rows.Scan(
			&movement.ID,
			&movement.ProductID,
			&movement.Type,
			&movement.Qty,
			&movement.Note,
			&movement.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan stock movement: %w", err)
		}

		movements = append(movements, movement)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stock movements: %w", err)
	}

	return movements, nil
}
