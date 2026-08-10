// Package guard is the public library API for the AI agent guardrail: a
// Guard wraps a policy engine (plus optional human-in-the-loop approval and
// audit logging) behind a single Check(ctx, call) method, so a Go agent can
// embed the guardrail directly instead of calling it over HTTP.
package guard

import (
	"context"
	"io"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/audit"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// Guard is the embeddable entry point: call Check before executing a tool.
type Guard struct {
	engine policy.Engine
	audit  *audit.Logger
	store  *approval.Store
}

// Option configures a Guard at construction time.
type Option func(*options)

type options struct {
	auditWriter     io.Writer
	auditRedact     audit.Redactor
	approvalStore   *approval.Store
	approvalTimeout time.Duration
}

// WithAudit logs every decision as JSON-lines to w. redact, if non-nil, is
// applied to a call's args before they're logged.
func WithAudit(w io.Writer, redact audit.Redactor) Option {
	return func(o *options) {
		o.auditWriter = w
		o.auditRedact = redact
	}
}

// WithApproval enables human-in-the-loop approval: a RequireApproval
// decision is parked in store and Check blocks until a human resolves it
// via store.Approve/Reject, or timeout elapses (which defaults to deny).
func WithApproval(store *approval.Store, timeout time.Duration) Option {
	return func(o *options) {
		o.approvalStore = store
		o.approvalTimeout = timeout
	}
}

// New builds a Guard from an already-loaded policy.
func New(p config.Policy, opts ...Option) (*Guard, error) {
	engine, err := policy.New(p)
	if err != nil {
		return nil, err
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	g := &Guard{engine: engine}

	if o.approvalStore != nil {
		g.store = o.approvalStore
		g.engine = approval.NewGate(g.engine, g.store, o.approvalTimeout)
	}
	if o.auditWriter != nil {
		g.audit = audit.New(o.auditWriter, o.auditRedact)
	}

	return g, nil
}

// NewFromFile loads a policy YAML file from path and builds a Guard from
// it.
func NewFromFile(path string, opts ...Option) (*Guard, error) {
	p, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return New(*p, opts...)
}

// Check evaluates call against the policy — blocking on human approval if
// one is configured and the decision requires it — and, if audit logging
// is configured, records the outcome. Call this before executing a tool.
func (g *Guard) Check(ctx context.Context, call policy.ToolCall) (policy.Decision, error) {
	decision, err := g.engine.Check(ctx, call)
	if err != nil {
		return decision, err
	}
	if g.audit != nil {
		g.audit.LogDecision(ctx, call, decision)
	}
	return decision, nil
}

// ApprovalStore returns the pending-approvals store configured via
// WithApproval, or nil if human-in-the-loop approval is not enabled. A
// server can use this to mount the approval HTTP endpoints alongside the
// interception endpoint.
func (g *Guard) ApprovalStore() *approval.Store {
	return g.store
}
