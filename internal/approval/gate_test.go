package approval

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func refundBotPolicy() config.Policy {
	max := 500.0
	return config.Policy{
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

func TestGate_AllowAndDenyPassThroughWithoutBlocking(t *testing.T) {
	engine, err := policy.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("policy.New() unexpected error: %v", err)
	}
	store := NewStore()
	gate := NewGate(engine, store, 5*time.Second)

	tests := []struct {
		name     string
		call     policy.ToolCall
		wantType policy.DecisionType
	}{
		{"allowed tool", policy.ToolCall{Tool: "get_order"}, policy.Allow},
		{"denied tool", policy.ToolCall{Tool: "run_shell"}, policy.Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gate.Check(context.Background(), tt.call)
			if err != nil {
				t.Fatalf("Check() unexpected error: %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Check().Type = %v, want %v", got.Type, tt.wantType)
			}
			if len(store.List()) != 0 {
				t.Errorf("store.List() = %v, want empty (should not park Allow/Deny)", store.List())
			}
		})
	}
}

func TestGate_RequireApprovalBlocksUntilApproved(t *testing.T) {
	engine, err := policy.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("policy.New() unexpected error: %v", err)
	}
	store := NewStore()
	gate := NewGate(engine, store, 5*time.Second)

	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 5000}}

	var got policy.Decision
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err = gate.Check(context.Background(), call)
	}()

	list := waitForPending(t, store, 1)
	if list[0].Call.Tool != "issue_refund" {
		t.Fatalf("pending call = %+v, want tool issue_refund", list[0].Call)
	}

	if approveErr := store.Approve(list[0].ID); approveErr != nil {
		t.Fatalf("Approve() unexpected error: %v", approveErr)
	}
	wg.Wait()

	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != policy.Allow {
		t.Errorf("Check().Type = %v, want Allow", got.Type)
	}
}

func TestGate_RequireApprovalRejectedDenies(t *testing.T) {
	engine, err := policy.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("policy.New() unexpected error: %v", err)
	}
	store := NewStore()
	gate := NewGate(engine, store, 5*time.Second)

	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 5000}}

	var got policy.Decision
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err = gate.Check(context.Background(), call)
	}()

	list := waitForPending(t, store, 1)
	if rejectErr := store.Reject(list[0].ID); rejectErr != nil {
		t.Fatalf("Reject() unexpected error: %v", rejectErr)
	}
	wg.Wait()

	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != policy.Deny {
		t.Errorf("Check().Type = %v, want Deny", got.Type)
	}
}

func TestGate_RequireApprovalTimeoutDeniesByDefault(t *testing.T) {
	engine, err := policy.New(refundBotPolicy())
	if err != nil {
		t.Fatalf("policy.New() unexpected error: %v", err)
	}
	store := NewStore()
	gate := NewGate(engine, store, 20*time.Millisecond)

	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 5000}}

	got, err := gate.Check(context.Background(), call)
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != policy.Deny {
		t.Errorf("Check().Type = %v, want Deny", got.Type)
	}
	if got.Rule != "approval.timeout" {
		t.Errorf("Check().Rule = %q, want %q", got.Rule, "approval.timeout")
	}
}
