package product

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	repository *Repository
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{repository: repository}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	search := strings.TrimSpace(query.Get("search"))

	page := 1
	if value := query.Get("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "INVALID_PAGE", "page must be a positive integer")
			return
		}
		page = parsed
	}

	limit := 10
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}

	if page-1 > math.MaxInt/limit {
		writeError(w, http.StatusBadRequest, "INVALID_PAGE", "page value is too large")
		return
	}

	offset := (page - 1) * limit

	products, total, err := h.repository.List(
		r.Context(),
		search,
		limit,
		offset,
	)
	if err != nil {
		log.Printf("failed to list products: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve products")
		return
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Data:  products,
		Page:  page,
		Limit: limit,
		Total: total,
	})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var typeErr *json.UnmarshalTypeError

		if errors.As(err, &typeErr) && typeErr.Field == "price" {
			writeError(w, http.StatusBadRequest, "INVALID_PRICE", "price must be an integer")
			return
		}

		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must be valid JSON")
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must contain only one JSON object")
		return
	}

	req.SKU = strings.TrimSpace(req.SKU)
	req.Name = strings.TrimSpace(req.Name)

	if req.SKU == "" {
		writeError(w, http.StatusBadRequest, "INVALID_SKU", "SKU is required")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_NAME", "name is required")
		return
	}

	if req.Price == nil || *req.Price < 0 || *req.Price > 2147483647 {
		writeError(w, http.StatusBadRequest, "INVALID_PRICE", "price must be an integer between 0 and 2147483647")
		return
	}

	item, err := h.repository.Create(
		r.Context(),
		req.SKU,
		req.Name,
		*req.Price,
	)
	if err != nil {
		if errors.Is(err, ErrDuplicateSKU) {
			writeError(w, http.StatusConflict, "DUPLICATE_SKU", "SKU already exists")
			return
		}

		log.Printf("failed to create product: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create product")
		return
	}

	writeJSON(w, http.StatusCreated, CreateProductResponse{
		Data: item,
	})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "product ID must be a positive integer")
		return
	}

	var req UpdateProductRequest

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var typeErr *json.UnmarshalTypeError

		if errors.As(err, &typeErr) && typeErr.Field == "price" {
			writeError(w, http.StatusBadRequest, "INVALID_PRICE", "price must be an integer")
			return
		}

		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must be valid JSON")
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "request body must contain only one JSON object")
		return
	}

	req.SKU = strings.TrimSpace(req.SKU)
	req.Name = strings.TrimSpace(req.Name)

	if req.SKU == "" {
		writeError(w, http.StatusBadRequest, "INVALID_SKU", "SKU is required")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_NAME", "name is required")
		return
	}

	if req.Price == nil || *req.Price < 0 || *req.Price > 2147483647 {
		writeError(w, http.StatusBadRequest, "INVALID_PRICE", "price must be an integer between 0 and 2147483647")
		return
	}

	if req.IsActive == nil {
		writeError(w, http.StatusBadRequest, "INVALID_IS_ACTIVE", "is_active is required")
		return
	}

	item, err := h.repository.Update(
		r.Context(),
		id,
		req.SKU,
		req.Name,
		*req.Price,
		*req.IsActive,
	)
	if err != nil {
		switch {
		case errors.Is(err, ErrProductNotFound):
			writeError(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "product not found")
		case errors.Is(err, ErrDuplicateSKU):
			writeError(w, http.StatusConflict, "DUPLICATE_SKU", "SKU already exists")
		case errors.Is(err, ErrProductHasSalesHistory):
			writeError(w, http.StatusConflict, "PRODUCT_HAS_SALES_HISTORY", "product SKU cannot be changed because it has sales history")
		default:
			log.Printf("failed to update product: %v", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update product")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]Product{
		"data": item,
	})
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "product ID must be a positive integer")
		return
	}

	err = h.repository.Delete(r.Context(), id)

	if err != nil {
		switch {
		case errors.Is(err, ErrProductNotFound):
			writeError(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "product not found")

		case errors.Is(err, ErrProductHasHistory):
			writeError(w, http.StatusConflict, "PRODUCT_HAS_HISTORY", "product has stock movement history")

		case errors.Is(err, ErrProductHasSalesHistory):
			writeError(w, http.StatusConflict, "PRODUCT_HAS_SALES_HISTORY", "product has sales history")

		default:
			log.Printf("failed to delete product: %v", err)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to delete product")
		}

		return
	}

	w.WriteHeader(http.StatusNoContent)
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
