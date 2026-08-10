package approval

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func waitForPending(t *testing.T, s *Store, n int) []PendingApproval {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if list := s.List(); len(list) == n {
			return list
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d pending approval(s)", n)
		case <-time.After(time.Millisecond):
		}
	}
}

func TestStore_ApproveAndReject(t *testing.T) {
	tests := []struct {
		name     string
		resolve  func(s *Store, id string) error
		wantType policy.DecisionType
		wantRule string
	}{
		{"approve", (*Store).Approve, policy.Allow, "approval.approved"},
		{"reject", (*Store).Reject, policy.Deny, "approval.rejected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund"}
			parked := policy.Decision{Type: policy.RequireApproval, Reason: "amount over max", Rule: "constraints.issue_refund.amount"}

			var got policy.Decision
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				got = s.Submit(context.Background(), call, parked, 5*time.Second)
			}()

			list := waitForPending(t, s, 1)
			if list[0].Reason != parked.Reason || list[0].Rule != parked.Rule {
				t.Fatalf("pending entry = %+v, want reason/rule %q/%q", list[0], parked.Reason, parked.Rule)
			}

			if err := tt.resolve(s, list[0].ID); err != nil {
				t.Fatalf("resolve() unexpected error: %v", err)
			}
			wg.Wait()

			if got.Type != tt.wantType {
				t.Errorf("Submit() Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Rule != tt.wantRule {
				t.Errorf("Submit() Rule = %q, want %q", got.Rule, tt.wantRule)
			}
			if len(s.List()) != 0 {
				t.Errorf("List() after resolve = %v, want empty", s.List())
			}
		})
	}
}

func TestStore_TimeoutDefaultsToDeny(t *testing.T) {
	s := NewStore()
	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund"}
	parked := policy.Decision{Type: policy.RequireApproval, Reason: "amount over max", Rule: "constraints.issue_refund.amount"}

	got := s.Submit(context.Background(), call, parked, 20*time.Millisecond)

	if got.Type != policy.Deny {
		t.Errorf("Submit() Type = %v, want Deny", got.Type)
	}
	if got.Rule != "approval.timeout" {
		t.Errorf("Submit() Rule = %q, want %q", got.Rule, "approval.timeout")
	}
	if len(s.List()) != 0 {
		t.Errorf("List() after timeout = %v, want empty", s.List())
	}
}

func TestStore_ContextCancellationDefaultsToDeny(t *testing.T) {
	s := NewStore()
	call := policy.ToolCall{SessionID: "s1", Tool: "issue_refund"}
	parked := policy.Decision{Type: policy.RequireApproval, Reason: "amount over max", Rule: "constraints.issue_refund.amount"}

	ctx, cancel := context.WithCancel(context.Background())

	var got policy.Decision
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		got = s.Submit(ctx, call, parked, 5*time.Second)
	}()

	waitForPending(t, s, 1)
	cancel()
	wg.Wait()

	if got.Type != policy.Deny {
		t.Errorf("Submit() Type = %v, want Deny", got.Type)
	}
	if got.Rule != "approval.canceled" {
		t.Errorf("Submit() Rule = %q, want %q", got.Rule, "approval.canceled")
	}
}

func TestStore_ResolveUnknownIDReturnsErrNotFound(t *testing.T) {
	s := NewStore()

	if err := s.Approve("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Approve() error = %v, want ErrNotFound", err)
	}
	if err := s.Reject("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Reject() error = %v, want ErrNotFound", err)
	}
}

func TestStore_ResolveTwiceReturnsErrNotFound(t *testing.T) {
	s := NewStore()
	call := policy.ToolCall{Tool: "issue_refund"}
	parked := policy.Decision{Type: policy.RequireApproval}

	go s.Submit(context.Background(), call, parked, 5*time.Second)
	list := waitForPending(t, s, 1)
	id := list[0].ID

	if err := s.Approve(id); err != nil {
		t.Fatalf("first Approve() unexpected error: %v", err)
	}
	if err := s.Approve(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Approve() error = %v, want ErrNotFound", err)
	}
	if err := s.Reject(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("Reject() after Approve() error = %v, want ErrNotFound", err)
	}
}

func TestStore_ListOrderedOldestFirst(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		go s.Submit(ctx, policy.ToolCall{Tool: "issue_refund"}, policy.Decision{Type: policy.RequireApproval}, 5*time.Second)
	}

	list := waitForPending(t, s, 3)
	for i := 1; i < len(list); i++ {
		prev, err1 := strconv.Atoi(list[i-1].ID)
		cur, err2 := strconv.Atoi(list[i].ID)
		if err1 != nil || err2 != nil {
			t.Fatalf("non-numeric ID in %v", list)
		}
		if prev >= cur {
			t.Errorf("List() not ordered oldest-first: %v", list)
		}
	}
}

// TestStore_ConcurrentAccess submits and resolves many approvals from
// multiple goroutines at once, to be run with -race.
func TestStore_ConcurrentAccess(t *testing.T) {
	s := NewStore()
	const n = 50

	var wg sync.WaitGroup
	results := make([]policy.Decision, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = s.Submit(context.Background(), policy.ToolCall{Tool: "issue_refund"}, policy.Decision{Type: policy.RequireApproval}, 5*time.Second)
		}(i)
	}

	// Resolve everything currently or eventually pending.
	resolved := 0
	deadline := time.After(2 * time.Second)
	for resolved < n {
		for _, p := range s.List() {
			if err := s.Approve(p.ID); err == nil {
				resolved++
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out resolving approvals, resolved %d/%d", resolved, n)
		case <-time.After(time.Millisecond):
		}
	}

	wg.Wait()

	for i, d := range results {
		if d.Type != policy.Allow {
			t.Errorf("result[%d].Type = %v, want Allow", i, d.Type)
		}
	}
}
