package order

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

type Handler struct {
	repository *Repository
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{repository: repository}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must be valid JSON")
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must contain only one JSON object")
		return
	}

	req.Customer = strings.TrimSpace(req.Customer)

	if req.Customer == "" {
		writeError(w, http.StatusBadRequest, "INVALID_CUSTOMER", "customer is required")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "EMPTY_ORDER", "order must contain at least one item")
		return
	}

	seen := make(map[string]bool, len(req.Items))

	for i := range req.Items {
		req.Items[i].SKU = strings.TrimSpace(req.Items[i].SKU)

		if req.Items[i].SKU == "" {
			writeError(w, http.StatusBadRequest, "INVALID_SKU", "item SKU is required")
			return
		}

		if req.Items[i].Qty < 1 || req.Items[i].Qty > maxDBInteger {
			writeError(w, http.StatusBadRequest, "INVALID_QTY", "item quantity must be a positive integer")
			return
		}

		if seen[req.Items[i].SKU] {
			writeError(w, http.StatusBadRequest, "DUPLICATE_SKU", "duplicate SKU: "+req.Items[i].SKU)
			return
		}

		seen[req.Items[i].SKU] = true
	}

	result, err := h.repository.Create(r.Context(), req)
	if err != nil {
		var unknownSKU *UnknownSKUError
		var insufficient *InsufficientStockError

		switch {
		case errors.As(err, &unknownSKU):
			writeError(w, http.StatusBadRequest, "UNKNOWN_SKU", "unknown SKU: "+unknownSKU.SKU)

		case errors.As(err, &insufficient):
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": map[string]any{
					"code":      "INSUFFICIENT_STOCK",
					"message":   "not enough stock for SKU: " + insufficient.SKU,
					"sku":       insufficient.SKU,
					"available": insufficient.Available,
				},
			})

		case errors.Is(err, ErrOrderTotalTooLarge):
			writeError(w, http.StatusBadRequest, "ORDER_TOTAL_TOO_LARGE", "order total exceeds the supported limit")

		default:
			log.Printf("failed to create order: %v", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create order")
		}

		return
	}

	writeJSON(w, http.StatusCreated, CreateOrderResponse{
		Data: result,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
