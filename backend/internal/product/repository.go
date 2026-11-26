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

var (
	ErrDuplicateSKU           = errors.New("SKU already exists")
	ErrProductNotFound        = errors.New("product not found")
	ErrProductHasHistory      = errors.New("product has stock movement history")
	ErrProductHasSalesHistory = errors.New("product has sales history")
)

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
