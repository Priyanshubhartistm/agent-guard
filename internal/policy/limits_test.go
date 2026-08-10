package policy

import (
	"context"
	"sync"
	"testing"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

func TestEngineCheck_RateLimit(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"ping"}},
			Limits: config.Limits{
				Rate: config.RateLimit{MaxCallsPerMinute: 3},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: "ping"})
		if err != nil {
			t.Fatalf("Check() unexpected error: %v", err)
		}
		if got.Type != Allow {
			t.Fatalf("call %d: Type = %v, want Allow (reason: %s)", i, got.Type, got.Reason)
		}
	}

	got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: "ping"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("4th call: Type = %v, want Deny", got.Type)
	}
	if got.Rule != "limits.rate" {
		t.Errorf("4th call: Rule = %q, want %q", got.Rule, "limits.rate")
	}
}

func TestEngineCheck_RateLimitIsPerSession(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"ping"}},
			Limits: config.Limits{
				Rate: config.RateLimit{MaxCallsPerMinute: 1},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()
	for _, session := range []string{"a", "b", "c"} {
		got, err := e.Check(ctx, ToolCall{SessionID: session, Tool: "ping"})
		if err != nil {
			t.Fatalf("Check() unexpected error: %v", err)
		}
		if got.Type != Allow {
			t.Errorf("session %q first call: Type = %v, want Allow", session, got.Type)
		}
	}
}

func TestEngineCheck_SpendLimit(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"issue_refund"}},
			Limits: config.Limits{
				Spend: config.SpendLimit{
					Currency:      "USD",
					Meter:         config.Meter{Tool: "issue_refund", Arg: "amount"},
					MaxPerSession: 1000,
				},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()

	got, err := e.Check(ctx, ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 600.0}})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Allow {
		t.Fatalf("first refund: Type = %v, want Allow (reason: %s)", got.Type, got.Reason)
	}

	// 600 + 500 = 1100 > 1000: must be denied, and the denied amount must
	// not be added to the running total.
	got, err = e.Check(ctx, ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 500.0}})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Fatalf("second refund: Type = %v, want Deny (reason: %s)", got.Type, got.Reason)
	}
	if got.Rule != "limits.spend" {
		t.Errorf("second refund: Rule = %q, want %q", got.Rule, "limits.spend")
	}

	// Since the denied 500 wasn't committed, a 400 refund (600 + 400 = 1000)
	// should still fit exactly under the cap.
	got, err = e.Check(ctx, ToolCall{SessionID: "s1", Tool: "issue_refund", Args: map[string]any{"amount": 400.0}})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Allow {
		t.Errorf("third refund: Type = %v, want Allow (reason: %s)", got.Type, got.Reason)
	}
}

func TestEngineCheck_SpendLimitIsPerSession(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"issue_refund"}},
			Limits: config.Limits{
				Spend: config.SpendLimit{
					Meter:         config.Meter{Tool: "issue_refund", Arg: "amount"},
					MaxPerSession: 100,
				},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	ctx := context.Background()
	for _, session := range []string{"a", "b"} {
		got, err := e.Check(ctx, ToolCall{SessionID: session, Tool: "issue_refund", Args: map[string]any{"amount": 100.0}})
		if err != nil {
			t.Fatalf("Check() unexpected error: %v", err)
		}
		if got.Type != Allow {
			t.Errorf("session %q: Type = %v, want Allow", session, got.Type)
		}
	}
}

// TestEngineCheck_ConcurrentAccess exercises rate and spend limiting from
// many goroutines on the same session, to be run with -race. It asserts
// only that access is race-free and that the spend cap is never exceeded
// among the calls the engine actually allowed.
func TestEngineCheck_ConcurrentAccess(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"issue_refund"}},
			Limits: config.Limits{
				Rate: config.RateLimit{MaxCallsPerMinute: 1000},
				Spend: config.SpendLimit{
					Meter:         config.Meter{Tool: "issue_refund", Arg: "amount"},
					MaxPerSession: 1000,
				},
			},
		},
	}
	e, err := New(p)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}

	const goroutines = 50
	const amount = 10.0
	ctx := context.Background()

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		allowedTotal float64
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := e.Check(ctx, ToolCall{SessionID: "shared", Tool: "issue_refund", Args: map[string]any{"amount": amount}})
			if err != nil {
				t.Errorf("Check() unexpected error: %v", err)
				return
			}
			if got.Type == Allow {
				mu.Lock()
				allowedTotal += amount
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowedTotal > 1000 {
		t.Errorf("allowed spend total = %v, want <= 1000", allowedTotal)
	}
}
