package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDuplicateSKU = errors.New("SKU already exists")

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
