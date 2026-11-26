package product

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
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
