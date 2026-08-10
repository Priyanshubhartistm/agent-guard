package approval

import (
	"context"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// Gate wraps a policy.Engine so that a RequireApproval verdict is parked in
// a Store and blocks until a human resolves it, or it times out. Allow and
// Deny verdicts pass straight through untouched.
type Gate struct {
	engine  policy.Engine
	store   *Store
	timeout time.Duration
}

var _ policy.Engine = (*Gate)(nil)

// NewGate returns a Gate that resolves RequireApproval verdicts from engine
// against store, waiting up to timeout for a human decision.
func NewGate(engine policy.Engine, store *Store, timeout time.Duration) *Gate {
	return &Gate{engine: engine, store: store, timeout: timeout}
}

// Check implements policy.Engine.
func (g *Gate) Check(ctx context.Context, call policy.ToolCall) (policy.Decision, error) {
	d, err := g.engine.Check(ctx, call)
	if err != nil || d.Type != policy.RequireApproval {
		return d, err
	}
	return g.store.Submit(ctx, call, d, g.timeout), nil
}
