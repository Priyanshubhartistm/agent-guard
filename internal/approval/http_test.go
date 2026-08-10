package approval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func TestHTTPHandler_ListPending(t *testing.T) {
	store := NewStore()
	handler := NewHandler(store)

	go store.Submit(context.Background(), policy.ToolCall{SessionID: "s1", Tool: "issue_refund"}, policy.Decision{Type: policy.RequireApproval, Reason: "amount over max", Rule: "constraints.issue_refund.amount"}, 5*time.Second)
	waitForPending(t, store, 1)

	req := httptest.NewRequest(http.MethodGet, "/pending", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []PendingApproval
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Call.Tool != "issue_refund" || got[0].Rule != "constraints.issue_refund.amount" {
		t.Errorf("got[0] = %+v, unexpected fields", got[0])
	}
}

func TestHTTPHandler_ApproveAndReject(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantType policy.DecisionType
	}{
		{"approve", "/approve", policy.Allow},
		{"reject", "/reject", policy.Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore()
			handler := NewHandler(store)

			var got policy.Decision
			done := make(chan struct{})
			go func() {
				got = store.Submit(context.Background(), policy.ToolCall{Tool: "issue_refund"}, policy.Decision{Type: policy.RequireApproval}, 5*time.Second)
				close(done)
			}()

			list := waitForPending(t, store, 1)

			req := httptest.NewRequest(http.MethodPost, "/pending/"+list[0].ID+tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusNoContent, rec.Body.String())
			}

			<-done
			if got.Type != tt.wantType {
				t.Errorf("Submit() Type = %v, want %v", got.Type, tt.wantType)
			}
		})
	}
}

func TestHTTPHandler_ResolveUnknownIDReturns404(t *testing.T) {
	store := NewStore()
	handler := NewHandler(store)

	for _, path := range []string{"/pending/does-not-exist/approve", "/pending/does-not-exist/reject"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestHTTPHandler_ListPendingEmpty(t *testing.T) {
	store := NewStore()
	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/pending", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []PendingApproval
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty", got)
	}
}
