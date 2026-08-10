package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestGuard_CheckAllowAndDeny(t *testing.T) {
	g, err := New(refundBotPolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	tests := []struct {
		name     string
		tool     string
		wantType policy.DecisionType
	}{
		{"allowed tool", "get_order", policy.Allow},
		{"denied tool", "run_shell", policy.Deny},
		{"unlisted tool", "delete_database", policy.Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := g.Check(context.Background(), policy.ToolCall{Tool: tt.tool})
			if err != nil {
				t.Fatalf("Check() unexpected error: %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Check().Type = %v, want %v", got.Type, tt.wantType)
			}
		})
	}
}

func TestGuard_ApprovalStoreNilWhenNotConfigured(t *testing.T) {
	g, err := New(refundBotPolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if g.ApprovalStore() != nil {
		t.Error("ApprovalStore() = non-nil, want nil when WithApproval was not used")
	}
}

func TestGuard_WithApprovalBlocksUntilResolved(t *testing.T) {
	store := approval.NewStore()
	g, err := New(refundBotPolicy(), WithApproval(store, 5*time.Second))
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	if g.ApprovalStore() != store {
		t.Fatal("ApprovalStore() did not return the configured store")
	}

	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 5000}}

	var got policy.Decision
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		got, err = g.Check(context.Background(), call)
	}()

	deadline := time.After(time.Second)
	for {
		if list := store.List(); len(list) == 1 {
			if approveErr := store.Approve(list[0].ID); approveErr != nil {
				t.Fatalf("Approve() unexpected error: %v", approveErr)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(time.Millisecond):
		}
	}
	wg.Wait()

	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != policy.Allow {
		t.Errorf("Check().Type = %v, want Allow", got.Type)
	}
}

func TestGuard_WithAuditLogsDecisions(t *testing.T) {
	var buf bytes.Buffer
	g, err := New(refundBotPolicy(), WithAudit(&buf, nil))
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	_, err = g.Check(context.Background(), policy.ToolCall{AgentID: "refund-bot", SessionID: "s1", Tool: "get_order"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected an audit record to be written, got none")
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("audit output is not valid JSON: %v", err)
	}
	if rec["tool"] != "get_order" || rec["decision"] != "allow" {
		t.Errorf("unexpected audit record: %v", rec)
	}
}

func TestNewFromFile(t *testing.T) {
	g, err := NewFromFile("../examples/refund-bot-policy.yaml")
	if err != nil {
		t.Fatalf("NewFromFile() unexpected error: %v", err)
	}

	got, err := g.Check(context.Background(), policy.ToolCall{Tool: "get_order"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != policy.Allow {
		t.Errorf("Check().Type = %v, want Allow", got.Type)
	}
}

func TestNewFromFile_MissingFile(t *testing.T) {
	if _, err := NewFromFile("does-not-exist.yaml"); err == nil {
		t.Fatal("NewFromFile() expected an error for a missing file, got nil")
	}
}
