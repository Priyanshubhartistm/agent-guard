package approval

import (
	"context"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// PendingHook is called synchronously whenever Check parks a call for
// human approval, before it blocks waiting for resolution. Useful for
// recording that a call is now pending (e.g. for a live dashboard); the
// eventual outcome is still just Check's ordinary return value.
type PendingHook func(ctx context.Context, call policy.ToolCall, decision policy.Decision)

// Gate wraps a policy.Engine so that a RequireApproval verdict is parked in
// a Store and blocks until a human resolves it, or it times out. Allow and
// Deny verdicts pass straight through untouched.
type Gate struct {
	engine    policy.Engine
	store     *Store
	timeout   time.Duration
	onPending PendingHook
}

var _ policy.Engine = (*Gate)(nil)

// NewGate returns a Gate that resolves RequireApproval verdicts from engine
// against store, waiting up to timeout for a human decision.
func NewGate(engine policy.Engine, store *Store, timeout time.Duration) *Gate {
	return &Gate{engine: engine, store: store, timeout: timeout}
}

// OnPending sets fn as the Gate's PendingHook and returns the Gate, for
// chaining after NewGate.
func (g *Gate) OnPending(fn PendingHook) *Gate {
	g.onPending = fn
	return g
}

// Check implements policy.Engine.
func (g *Gate) Check(ctx context.Context, call policy.ToolCall) (policy.Decision, error) {
	d, err := g.engine.Check(ctx, call)
	if err != nil || d.Type != policy.RequireApproval {
		return d, err
	}
	if g.onPending != nil {
		g.onPending(ctx, call, d)
	}
	return g.store.Submit(ctx, call, d, g.timeout), nil
}
