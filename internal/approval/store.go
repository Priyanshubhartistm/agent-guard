// Package approval implements human-in-the-loop resolution of
// RequireApproval decisions: a pending-approvals store, a Gate that blocks
// a tool call on a human's verdict (or a timeout), and the HTTP endpoints a
// human uses to approve or reject.
package approval

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// ErrNotFound is returned by Approve/Reject when id does not name a call
// currently awaiting approval (already resolved, timed out, or unknown).
var ErrNotFound = errors.New("approval: pending call not found")

// PendingApproval is a read-only snapshot of a call parked for human
// review.
type PendingApproval struct {
	ID     string
	Call   policy.ToolCall
	Reason string // why the engine required approval
	Rule   string // which rule triggered it
}

type entry struct {
	seq      uint64
	call     policy.ToolCall
	decision policy.Decision
	resolved chan policy.Decision // buffered(1); receives the human's verdict
}

// Store tracks tool calls parked pending human approval, and lets a human
// resolve them via Approve or Reject. It is safe for concurrent use.
type Store struct {
	mu      sync.Mutex
	pending map[string]*entry
	nextID  uint64
}

// NewStore returns an empty, ready-to-use Store.
func NewStore() *Store {
	return &Store{pending: make(map[string]*entry)}
}

// Submit parks call for human approval and blocks until it is approved,
// rejected, the timeout elapses, or ctx is canceled. A timeout or
// cancellation defaults to Deny (fail-closed).
func (s *Store) Submit(ctx context.Context, call policy.ToolCall, decision policy.Decision, timeout time.Duration) policy.Decision {
	e := &entry{call: call, decision: decision, resolved: make(chan policy.Decision, 1)}

	s.mu.Lock()
	s.nextID++
	e.seq = s.nextID
	id := strconv.FormatUint(s.nextID, 10)
	s.pending[id] = e
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case d := <-e.resolved:
		return d
	case <-timer.C:
		return policy.Decision{
			Type:   policy.Deny,
			Reason: "approval request timed out; defaulting to deny",
			Rule:   "approval.timeout",
		}
	case <-ctx.Done():
		return policy.Decision{
			Type:   policy.Deny,
			Reason: "approval request canceled; defaulting to deny",
			Rule:   "approval.canceled",
		}
	}
}

// Approve resolves a pending call as Allow.
func (s *Store) Approve(id string) error {
	return s.resolve(id, policy.Decision{
		Type:   policy.Allow,
		Reason: "approved by human reviewer",
		Rule:   "approval.approved",
	})
}

// Reject resolves a pending call as Deny.
func (s *Store) Reject(id string) error {
	return s.resolve(id, policy.Decision{
		Type:   policy.Deny,
		Reason: "rejected by human reviewer",
		Rule:   "approval.rejected",
	})
}

// resolve removes id from the pending set and delivers d to whatever
// Submit call is waiting on it. Removing before sending ensures a racing
// Approve/Reject/timeout can resolve a given id only once.
func (s *Store) resolve(id string, d policy.Decision) error {
	s.mu.Lock()
	e, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()

	if !ok {
		return ErrNotFound
	}
	e.resolved <- d
	return nil
}

// List returns a snapshot of every call currently awaiting a human
// decision, ordered oldest-first.
func (s *Store) List() []PendingApproval {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.pending))
	for id := range s.pending {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return s.pending[ids[i]].seq < s.pending[ids[j]].seq
	})

	out := make([]PendingApproval, 0, len(ids))
	for _, id := range ids {
		e := s.pending[id]
		out = append(out, PendingApproval{
			ID:     id,
			Call:   e.call,
			Reason: e.decision.Reason,
			Rule:   e.decision.Rule,
		})
	}
	return out
}
