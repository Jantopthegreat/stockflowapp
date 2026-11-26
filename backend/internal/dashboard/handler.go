package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
)

type Handler struct {
	repository *Repository
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{repository: repository}
}

func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	result, err := h.repository.GetSummary(r.Context())

	if err != nil {
		log.Printf("get dashboard summary: %v", err)

		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{
				"code":    "INTERNAL_ERROR",
				"message": "internal server error",
			},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": result,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
