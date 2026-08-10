package policy

import (
	"context"
	"fmt"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

// engine is the default Engine implementation.
type engine struct {
	allow map[string]struct{}
	deny  map[string]struct{}

	constraints map[string][]compiledConstraint // keyed by tool

	rateLimit int // max calls per minute per session; <= 0 means unlimited

	spendMeterTool string // empty means spend limiting is not configured
	spendMeterArg  string
	spendMax       float64

	sequence compiledSequence

	state SessionState
}

// New builds an Engine from a loaded policy config. It validates
// constraints (regex syntax, on_violation values) up front, so a bad
// policy fails fast at construction instead of on the hot path.
func New(p config.Policy) (Engine, error) {
	e := &engine{
		allow:       make(map[string]struct{}, len(p.Policies.Tools.Allow)),
		deny:        make(map[string]struct{}, len(p.Policies.Tools.Deny)),
		constraints: make(map[string][]compiledConstraint),
		rateLimit:   p.Policies.Limits.Rate.MaxCallsPerMinute,
		state:       NewMemoryState(),
	}

	for _, t := range p.Policies.Tools.Allow {
		e.allow[t] = struct{}{}
	}
	for _, t := range p.Policies.Tools.Deny {
		e.deny[t] = struct{}{}
	}

	for _, c := range p.Policies.Constraints {
		cc, err := compileConstraint(c)
		if err != nil {
			return nil, fmt.Errorf("policy: invalid constraint on %s.%s: %w", c.Tool, c.Arg, err)
		}
		e.constraints[c.Tool] = append(e.constraints[c.Tool], cc)
	}

	if meter := p.Policies.Limits.Spend.Meter; meter.Tool != "" {
		e.spendMeterTool = meter.Tool
		e.spendMeterArg = meter.Arg
		e.spendMax = p.Policies.Limits.Spend.MaxPerSession
	}

	e.sequence = compileSequence(p.Policies.Sequence)

	return e, nil
}

// Check evaluates a ToolCall against the tools allow/deny list, argument
// constraints, sequence (FSM) rules, and the rate/spend limits, in that
// order. The first rule that produces a non-Allow verdict wins.
func (e *engine) Check(ctx context.Context, call ToolCall) (Decision, error) {
	if d, stop := e.checkTools(call); stop {
		return d, nil
	}
	if d, stop := e.checkConstraints(call); stop {
		return d, nil
	}
	if d, stop := e.checkSequence(call); stop {
		return d, nil
	}
	if d, stop := e.checkRateLimit(call); stop {
		return d, nil
	}
	if d, stop := e.checkSpendLimit(call); stop {
		return d, nil
	}

	return Decision{
		Type:   Allow,
		Reason: fmt.Sprintf("tool %q is allowed", call.Tool),
		Rule:   "tools.allow",
	}, nil
}

// checkTools enforces the fail-closed tool allow/deny list. An explicit
// deny always wins over an allow.
func (e *engine) checkTools(call ToolCall) (Decision, bool) {
	if _, denied := e.deny[call.Tool]; denied {
		return Decision{
			Type:   Deny,
			Reason: fmt.Sprintf("tool %q is explicitly denied", call.Tool),
			Rule:   "tools.deny",
		}, true
	}

	if _, allowed := e.allow[call.Tool]; !allowed {
		return Decision{
			Type:   Deny,
			Reason: fmt.Sprintf("tool %q is not in the allowlist (fail-closed default)", call.Tool),
			Rule:   "tools.default-deny",
		}, true
	}

	return Decision{}, false
}

// checkConstraints enforces per-argument constraints for call.Tool.
func (e *engine) checkConstraints(call ToolCall) (Decision, bool) {
	for _, c := range e.constraints[call.Tool] {
		violated, detail := c.evaluate(call.Args)
		if !violated {
			continue
		}

		reason := fmt.Sprintf("tool %q arg %q %s", call.Tool, c.arg, detail)
		rule := fmt.Sprintf("constraints.%s.%s", call.Tool, c.arg)

		if c.onViolation == violationRequireApproval {
			return Decision{Type: RequireApproval, Reason: reason, Rule: rule}, true
		}
		return Decision{Type: Deny, Reason: reason, Rule: rule}, true
	}
	return Decision{}, false
}

// checkSequence enforces the sequence (FSM) rules: call.Tool must be a
// valid transition from the session's current state.
func (e *engine) checkSequence(call ToolCall) (Decision, bool) {
	if !e.sequence.enabled {
		return Decision{}, false
	}

	state, ok := e.state.AdvanceState(call.SessionID, e.sequence.initial, func(current string) (string, bool) {
		return e.sequence.next(current, call.Tool)
	})
	if ok {
		return Decision{}, false
	}

	reason := fmt.Sprintf("tool %q is not valid from sequence state %q", call.Tool, state)
	if allowed := e.sequence.allowedTools(state); len(allowed) > 0 {
		reason += fmt.Sprintf(" (expected one of: %v)", allowed)
	} else {
		reason += " (no further tools are valid from this state)"
	}

	return Decision{
		Type:   Deny,
		Reason: reason,
		Rule:   "sequence",
	}, true
}

// checkRateLimit enforces the per-session max-calls-per-minute limit.
func (e *engine) checkRateLimit(call ToolCall) (Decision, bool) {
	if e.rateLimit <= 0 {
		return Decision{}, false
	}

	if e.state.AllowRate(call.SessionID, e.rateLimit) {
		return Decision{}, false
	}

	return Decision{
		Type:   Deny,
		Reason: fmt.Sprintf("session %q exceeded rate limit of %d calls/minute", call.SessionID, e.rateLimit),
		Rule:   "limits.rate",
	}, true
}

// checkSpendLimit sums the metered arg across the session and denies once
// the session's spend cap would be exceeded.
func (e *engine) checkSpendLimit(call ToolCall) (Decision, bool) {
	if e.spendMeterTool == "" || call.Tool != e.spendMeterTool {
		return Decision{}, false
	}

	raw, present := call.Args[e.spendMeterArg]
	if !present {
		// Nothing to meter on this call.
		return Decision{}, false
	}

	amount, ok := toFloat64(raw)
	if !ok {
		return Decision{
			Type:   Deny,
			Reason: fmt.Sprintf("session %q: could not read a numeric spend amount from arg %q (got %v)", call.SessionID, e.spendMeterArg, raw),
			Rule:   "limits.spend",
		}, true
	}

	meterKey := e.spendMeterTool + "." + e.spendMeterArg
	total, ok := e.state.AddSpend(call.SessionID, meterKey, amount, e.spendMax)
	if !ok {
		return Decision{
			Type:   Deny,
			Reason: fmt.Sprintf("session %q would exceed spend limit of %v (attempted total %v)", call.SessionID, e.spendMax, total),
			Rule:   "limits.spend",
		}, true
	}

	return Decision{}, false
}
