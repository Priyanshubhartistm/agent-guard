package policy

import (
	"context"
	"sync"
	"testing"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

func refundBotSequencePolicy() config.Policy {
	return config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"get_order", "issue_refund", "send_email"}},
			Sequence: config.Sequence{
				Initial: "start",
				Transitions: []config.Transition{
					{From: "start", Tool: "get_order", To: "order_loaded"},
					{From: "order_loaded", Tool: "issue_refund", To: "refunded"},
					{From: "refunded", Tool: "send_email", To: "done"},
				},
			},
		},
	}
}

func TestEngineCheck_Sequence_ValidOrderingPasses(t *testing.T) {
	e, err := New(refundBotSequencePolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()
	steps := []string{"get_order", "issue_refund", "send_email"}

	for _, tool := range steps {
		got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: tool})
		if err != nil {
			t.Fatalf("Check(%q) unexpected error: %v", tool, err)
		}
		if got.Type != Allow {
			t.Fatalf("Check(%q).Type = %v, want Allow (reason: %s)", tool, got.Type, got.Reason)
		}
	}
}

func TestEngineCheck_Sequence_SkippingStepDenied(t *testing.T) {
	e, err := New(refundBotSequencePolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	// issue_refund before get_order: no transition from "start" on
	// issue_refund.
	got, err := e.Check(context.Background(), ToolCall{SessionID: "s1", Tool: "issue_refund"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("Type = %v, want Deny", got.Type)
	}
	if got.Rule != "sequence" {
		t.Errorf("Rule = %q, want %q", got.Rule, "sequence")
	}
	if got.Reason == "" {
		t.Errorf("Reason should not be empty")
	}
}

func TestEngineCheck_Sequence_OutOfOrderDenied(t *testing.T) {
	e, err := New(refundBotSequencePolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()

	// Valid: get_order, issue_refund.
	for _, tool := range []string{"get_order", "issue_refund"} {
		got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: tool})
		if err != nil {
			t.Fatalf("Check(%q) unexpected error: %v", tool, err)
		}
		if got.Type != Allow {
			t.Fatalf("Check(%q).Type = %v, want Allow (reason: %s)", tool, got.Type, got.Reason)
		}
	}

	// Now in state "refunded": calling get_order again is out of order.
	got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: "get_order"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("Type = %v, want Deny", got.Type)
	}
	if got.Rule != "sequence" {
		t.Errorf("Rule = %q, want %q", got.Rule, "sequence")
	}
}

func TestEngineCheck_Sequence_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		calls    []string // tool calls made in order on the same session
		wantLast DecisionType
	}{
		{"first step from start", []string{"get_order"}, Allow},
		{"second step after first", []string{"get_order", "issue_refund"}, Allow},
		{"third step after first two", []string{"get_order", "issue_refund", "send_email"}, Allow},
		{"skip get_order", []string{"issue_refund"}, Deny},
		{"skip issue_refund", []string{"get_order", "send_email"}, Deny},
		{"repeat get_order", []string{"get_order", "get_order"}, Deny},
		{"replay after done", []string{"get_order", "issue_refund", "send_email", "get_order"}, Deny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := New(refundBotSequencePolicy())
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}

			var got Decision
			ctx := context.Background()
			for _, tool := range tt.calls {
				got, err = e.Check(ctx, ToolCall{SessionID: "s1", Tool: tool})
				if err != nil {
					t.Fatalf("Check(%q) unexpected error: %v", tool, err)
				}
			}

			if got.Type != tt.wantLast {
				t.Errorf("last call Type = %v, want %v (reason: %s)", got.Type, tt.wantLast, got.Reason)
			}
		})
	}
}

func TestEngineCheck_Sequence_IsPerSession(t *testing.T) {
	e, err := New(refundBotSequencePolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()

	// Session "a" progresses to order_loaded.
	if got, err := e.Check(ctx, ToolCall{SessionID: "a", Tool: "get_order"}); err != nil || got.Type != Allow {
		t.Fatalf("session a get_order: got %+v, err %v", got, err)
	}

	// Session "b" is independent and still at "start", so issue_refund
	// (which requires order_loaded) must be denied for it.
	got, err := e.Check(ctx, ToolCall{SessionID: "b", Tool: "issue_refund"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("session b issue_refund: Type = %v, want Deny", got.Type)
	}
}

func TestEngineCheck_Sequence_NotConfiguredMeansUnrestricted(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"issue_refund"}},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	got, err := e.Check(context.Background(), ToolCall{SessionID: "s1", Tool: "issue_refund"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Allow {
		t.Errorf("Type = %v, want Allow (reason: %s)", got.Type, got.Reason)
	}
}

// TestEngineCheck_Sequence_ConcurrentAccess fires many concurrent identical
// transition attempts at a shared session and asserts, under -race, that
// exactly one succeeds: the FSM's read-then-advance must be atomic.
func TestEngineCheck_Sequence_ConcurrentAccess(t *testing.T) {
	e, err := New(refundBotSequencePolicy())
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	const goroutines = 50
	ctx := context.Background()

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := e.Check(ctx, ToolCall{SessionID: "shared", Tool: "get_order"})
			if err != nil {
				t.Errorf("Check() unexpected error: %v", err)
				return
			}
			if got.Type == Allow {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 1 {
		t.Errorf("allowed = %d, want exactly 1", allowed)
	}
}
