package product

import "time"

type Product struct {
	ID             int64      `json:"id"`
	SKU            string     `json:"sku"`
	Name           string     `json:"name"`
	Price          int64      `json:"price"`
	Stock          int64      `json:"stock"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	LastMovementAt *time.Time `json:"last_movement_at"`
}

type ListResponse struct {
	Data  []Product `json:"data"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
	Total int64     `json:"total"`
}

type CreateProductRequest struct {
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Price *int64 `json:"price"`
}

type CreateProductResponse struct {
	Data Product `json:"data"`
}

type UpdateProductRequest struct {
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Price    *int64 `json:"price"`
	IsActive *bool  `json:"is_active"`
}
