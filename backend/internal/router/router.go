package router

import (
	"net/http"

	"stockflow/backend/internal/handler"
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

	r.Get("/health", handler.Health(pool))

	r.Route("/api", func(r chi.Router) {
		r.Get("/products", products.List)
		r.Post("/products", products.Create)
		r.Put("/products/{id}", products.Update)
		r.Delete("/products/{id}", products.Delete)

	})

	return r
}
