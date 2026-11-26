package dashboard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repo *Repository) GetSummary(ctx context.Context) (Summary, error) {
	var result Summary

	err := repo.pool.QueryRow(ctx, `
        SELECT
            COUNT(*)::BIGINT,
            COUNT(*) FILTER (WHERE is_active = TRUE)::BIGINT,
            COALESCE(SUM(price::BIGINT * stock::BIGINT), 0)::BIGINT,
            COUNT(*) FILTER (WHERE stock <= 10)::BIGINT
        FROM products
    `).Scan(
		&result.TotalProducts,
		&result.ActiveProducts,
		&result.TotalStockValue,
		&result.LowStockProducts,
	)

	if err != nil {
		return Summary{}, fmt.Errorf("get dashboard summary: %w", err)
	}

	return result, nil
}
