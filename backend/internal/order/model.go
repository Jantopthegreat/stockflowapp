package order

import "time"

type CreateOrderRequest struct {
	Customer string                   `json:"customer"`
	Items    []CreateOrderItemRequest `json:"items"`
}

type CreateOrderItemRequest struct {
	SKU string `json:"sku"`
	Qty int64  `json:"qty"`
}

type OrderItem struct {
	ID        int64  `json:"id"`
	OrderID   int64  `json:"order_id"`
	SKU       string `json:"sku"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}

type Order struct {
	ID          int64       `json:"id"`
	Customer    string      `json:"customer"`
	Status      string      `json:"status"`
	Total       int64       `json:"total"`
	CreatedAt   time.Time   `json:"created_at"`
	CancelledAt *time.Time  `json:"cancelled_at"`
	Items       []OrderItem `json:"items"`
}

type CreateOrderResponse struct {
	Data Order `json:"data"`
}
