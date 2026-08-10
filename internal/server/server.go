// Package server wraps a guard.Guard as a standalone HTTP service: an
// agent (or proxy) POSTs a ToolCall and gets back a Decision.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// NewHandler returns an http.Handler exposing the guardrail's HTTP API:
//
//	POST /check   intercept a ToolCall (JSON) and return its Decision (JSON)
//
// If g has human-in-the-loop approval configured, the pending-approvals
// endpoints (GET /pending, POST /pending/{id}/approve,
// POST /pending/{id}/reject) are mounted too.
func NewHandler(g *guard.Guard) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /check", func(w http.ResponseWriter, r *http.Request) {
		var call policy.ToolCall
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if call.Timestamp.IsZero() {
			call.Timestamp = time.Now()
		}

		decision, err := g.Check(r.Context(), call)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, decision)
	})

	if store := g.ApprovalStore(); store != nil {
		approvalHandler := approval.NewHandler(store)
		mux.Handle("/pending", approvalHandler)
		mux.Handle("/pending/", approvalHandler)
	}

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
