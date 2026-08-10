package policy

import (
	"context"
	"testing"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

func float64ptr(f float64) *float64 { return &f }

func TestEngineCheck_Constraints(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{
				Allow: []string{"issue_refund", "set_status", "search"},
			},
			Constraints: []config.Constraint{
				{Tool: "issue_refund", Arg: "amount", Max: float64ptr(500), OnViolation: "require_approval"},
				{Tool: "issue_refund", Arg: "amount", Min: float64ptr(1), OnViolation: "deny"},
				{Tool: "set_status", Arg: "status", OneOf: []any{"open", "closed", "pending"}, OnViolation: "deny"},
				{Tool: "search", Arg: "query", Regex: `^[a-zA-Z0-9 ]+$`, OnViolation: "deny"},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantType DecisionType
	}{
		{"amount within max (float64)", "issue_refund", map[string]any{"amount": 100.0}, Allow},
		{"amount within max (int, JSON-number coercion)", "issue_refund", map[string]any{"amount": 100}, Allow},
		{"amount over max requires approval", "issue_refund", map[string]any{"amount": 5000}, RequireApproval},
		{"amount below min is denied", "issue_refund", map[string]any{"amount": 0}, Deny},
		{"missing constrained arg passes through", "issue_refund", map[string]any{}, Allow},
		{"one_of match allowed", "set_status", map[string]any{"status": "closed"}, Allow},
		{"one_of mismatch denied", "set_status", map[string]any{"status": "archived"}, Deny},
		{"regex match allowed", "search", map[string]any{"query": "hello world 123"}, Allow},
		{"regex mismatch denied", "search", map[string]any{"query": "hello; rm -rf /"}, Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{SessionID: "s1", Tool: tt.tool, Args: tt.args}
			got, err := e.Check(context.Background(), call)
			if err != nil {
				t.Fatalf("Check() unexpected error: %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Check().Type = %v, want %v (reason: %s)", got.Type, tt.wantType, got.Reason)
			}
			if got.Type != Allow && got.Reason == "" {
				t.Errorf("Check().Reason should not be empty for a non-Allow decision")
			}
		})
	}
}

func TestEngineCheck_ConstraintNonNumericValueDenied(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools:       config.Tools{Allow: []string{"issue_refund"}},
			Constraints: []config.Constraint{{Tool: "issue_refund", Arg: "amount", Max: float64ptr(500), OnViolation: "deny"}},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	got, err := e.Check(context.Background(), ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": "a lot"}})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("Check().Type = %v, want Deny", got.Type)
	}
}

func TestNew_InvalidConstraintRejected(t *testing.T) {
	tests := []struct {
		name string
		c    config.Constraint
	}{
		{"bad on_violation", config.Constraint{Tool: "t", Arg: "a", OnViolation: "explode"}},
		{"bad regex", config.Constraint{Tool: "t", Arg: "a", Regex: "(unterminated", OnViolation: "deny"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := config.Policy{
				Policies: config.Policies{
					Tools:       config.Tools{Allow: []string{"t"}},
					Constraints: []config.Constraint{tt.c},
				},
			}
			if _, err := New(p); err == nil {
				t.Fatal("New() expected an error, got nil")
			}
		})
	}
}
