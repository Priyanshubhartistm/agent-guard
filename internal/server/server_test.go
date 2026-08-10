package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func refundBotPolicy() config.Policy {
	max := 500.0
	return config.Policy{
		Agent: "refund-bot",
		Policies: config.Policies{
			Tools: config.Tools{
				Allow: []string{"get_order", "issue_refund"},
				Deny:  []string{"run_shell"},
			},
			Constraints: []config.Constraint{
				{Tool: "issue_refund", Arg: "amount", Max: &max, OnViolation: "require_approval"},
			},
		},
	}
}

func TestHandler_Check_AllowAndDeny(t *testing.T) {
	g, err := guard.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}
	handler := NewHandler(g)

	tests := []struct {
		name     string
		tool     string
		wantType policy.DecisionType
	}{
		{"allowed tool", "get_order", policy.Allow},
		{"denied tool", "run_shell", policy.Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(policy.ToolCall{SessionID: "s1", Tool: tt.tool})
			req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got policy.Decision
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response is not valid JSON: %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Decision.Type = %v, want %v (reason: %s)", got.Type, tt.wantType, got.Reason)
			}
		})
	}
}

func TestHandler_Check_InvalidJSONReturns400(t *testing.T) {
	g, err := guard.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}
	handler := NewHandler(g)

	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandler_PendingEndpointsNotMountedWithoutApproval(t *testing.T) {
	g, err := guard.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}
	handler := NewHandler(g)

	req := httptest.NewRequest(http.MethodGet, "/pending", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (approval not configured)", rec.Code, http.StatusNotFound)
	}
}

func TestHandler_RequireApprovalEndToEndOverHTTP(t *testing.T) {
	store := approval.NewStore()
	g, err := guard.New(refundBotPolicy(), guard.WithApproval(store, 5*time.Second))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}
	handler := NewHandler(g)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body, _ := json.Marshal(policy.ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 5000}})

	type checkResult struct {
		decision policy.Decision
		err      error
	}
	resultCh := make(chan checkResult, 1)

	go func() {
		resp, err := http.Post(srv.URL+"/check", "application/json", bytes.NewReader(body))
		if err != nil {
			resultCh <- checkResult{err: err}
			return
		}
		defer resp.Body.Close()
		var d policy.Decision
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			resultCh <- checkResult{err: err}
			return
		}
		resultCh <- checkResult{decision: d}
	}()

	// Poll GET /pending until the call shows up.
	var pendingID string
	deadline := time.After(2 * time.Second)
	for pendingID == "" {
		resp, err := http.Get(srv.URL + "/pending")
		if err != nil {
			t.Fatalf("GET /pending: %v", err)
		}
		var list []approval.PendingApproval
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			t.Fatalf("decode /pending: %v", err)
		}
		resp.Body.Close()
		if len(list) == 1 {
			pendingID = list[0].ID
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval to appear")
		case <-time.After(10 * time.Millisecond):
		}
	}

	resp, err := http.Post(srv.URL+"/pending/"+pendingID+"/approve", "", nil)
	if err != nil {
		t.Fatalf("POST approve: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("approve status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("POST /check: %v", res.err)
		}
		if res.decision.Type != policy.Allow {
			t.Errorf("final Decision.Type = %v, want Allow", res.decision.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for /check to return")
	}
}
