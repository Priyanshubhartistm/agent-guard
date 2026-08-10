package approval

import (
	"encoding/json"
	"errors"
	"net/http"
)

// NewHandler returns an http.Handler exposing the pending-approvals API:
//
//	GET  /pending               list calls awaiting a human decision
//	POST /pending/{id}/approve  approve a pending call
//	POST /pending/{id}/reject   reject a pending call
func NewHandler(store *Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /pending", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.List())
	})
	mux.HandleFunc("POST /pending/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
		handleResolve(w, r, store.Approve)
	})
	mux.HandleFunc("POST /pending/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
		handleResolve(w, r, store.Reject)
	})

	return mux
}

func handleResolve(w http.ResponseWriter, r *http.Request, action func(id string) error) {
	id := r.PathValue("id")
	switch err := action(id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
