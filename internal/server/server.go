// Package server wraps a guard.Guard as a standalone HTTP service: an
// agent (or proxy) POSTs a ToolCall and gets back a Decision. It also
// serves a small live dashboard (stats, pending approvals, recent
// decisions) at GET /.
package server

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

//go:embed dashboard.html
var dashboardHTML []byte

// NewHandler returns an http.Handler exposing the guardrail's HTTP API:
//
//	GET  /             the live dashboard
//	POST /check        intercept a ToolCall (JSON) and return its Decision (JSON)
//	GET  /stats        Allow/Deny/Pending counts (guard.Stats, JSON)
//	GET  /decisions    the most recent decisions, newest first ([]guard.Record, JSON)
//
// If g has human-in-the-loop approval configured, the pending-approvals
// endpoints (GET /pending, POST /pending/{id}/approve,
// POST /pending/{id}/reject) are mounted too.
func NewHandler(g *guard.Guard) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(dashboardHTML)
	})

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

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, g.Stats())
	})

	mux.HandleFunc("GET /decisions", func(w http.ResponseWriter, r *http.Request) {
		decisions := g.RecentDecisions()
		if decisions == nil {
			decisions = []guard.Record{}
		}
		writeJSON(w, http.StatusOK, decisions)
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
