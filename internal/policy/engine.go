package policy

import (
	"context"
	"fmt"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

// engine is the default Engine implementation. Phase 1 evaluates only the
// tools allow/deny section of the policy.
type engine struct {
	allow map[string]struct{}
	deny  map[string]struct{}
}

// New builds an Engine from a loaded policy config.
func New(p config.Policy) Engine {
	e := &engine{
		allow: make(map[string]struct{}, len(p.Policies.Tools.Allow)),
		deny:  make(map[string]struct{}, len(p.Policies.Tools.Deny)),
	}
	for _, t := range p.Policies.Tools.Allow {
		e.allow[t] = struct{}{}
	}
	for _, t := range p.Policies.Tools.Deny {
		e.deny[t] = struct{}{}
	}
	return e
}

// Check evaluates a ToolCall against the tools allow/deny policy.
// An explicit deny always wins; anything not explicitly allowed is denied
// (fail-closed).
func (e *engine) Check(ctx context.Context, call ToolCall) (Decision, error) {
	if _, denied := e.deny[call.Tool]; denied {
		return Decision{
			Type:   Deny,
			Reason: fmt.Sprintf("tool %q is explicitly denied", call.Tool),
			Rule:   "tools.deny",
		}, nil
	}

	if _, allowed := e.allow[call.Tool]; allowed {
		return Decision{
			Type:   Allow,
			Reason: fmt.Sprintf("tool %q is allowed", call.Tool),
			Rule:   "tools.allow",
		}, nil
	}

	return Decision{
		Type:   Deny,
		Reason: fmt.Sprintf("tool %q is not in the allowlist (fail-closed default)", call.Tool),
		Rule:   "tools.default-deny",
	}, nil
}
