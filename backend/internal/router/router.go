package router

import (
	"net/http"

	"stockflow/backend/internal/dashboard"
	"stockflow/backend/internal/handler"
	"stockflow/backend/internal/order"
	"stockflow/backend/internal/product"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(pool *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	products := product.NewHandler(
		product.NewRepository(pool),
	)
	orders := order.NewHandler(
		order.NewRepository(pool),
	)
	dashboardHandler := dashboard.NewHandler(
		dashboard.NewRepository(pool),
	)

	r.Get("/health", handler.Health(pool))

	r.Route("/api", func(r chi.Router) {
		r.Get("/products", products.List)
		r.Post("/products", products.Create)
		r.Put("/products/{id}", products.Update)
		r.Delete("/products/{id}", products.Delete)

		r.Post("/products/{id}/stock", products.AdjustStock)
		r.Get("/products/{id}/movements", products.ListMovements)

		r.Post("/orders", orders.Create)
		r.Get("/orders/{id}", orders.GetByID)
		r.Post("/orders/{id}/cancel", orders.Cancel)

		r.Get("/dashboard/summary", dashboardHandler.GetSummary)
	})

	return r
}
