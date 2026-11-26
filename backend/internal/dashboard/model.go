package dashboard

type Summary struct {
	TotalProducts    int64 `json:"total_products"`
	ActiveProducts   int64 `json:"active_products"`
	TotalStockValue  int64 `json:"total_stock_value"`
	LowStockProducts int64 `json:"low_stock_products"`
}
