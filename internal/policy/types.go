// Package policy defines the core types and the decision engine interface
// for the AI agent guardrail. The engine evaluates every tool call an agent
// wants to make against a declarative policy, before the tool executes.
package policy

import (
	"context"
	"time"
)

// ToolCall is a single tool invocation an agent wants to make.
type ToolCall struct {
	AgentID   string         // which agent/policy set applies
	SessionID string         // groups calls for rate/spend/sequence state
	Tool      string         // e.g. "issue_refund"
	Args      map[string]any // e.g. {"amount": 5000, "customer": 123}
	Timestamp time.Time
}

// DecisionType is the outcome of evaluating a ToolCall against policy.
type DecisionType int

const (
	// Allow permits the tool call to execute.
	Allow DecisionType = iota
	// Deny rejects the tool call outright.
	Deny
	// RequireApproval parks the tool call pending human review.
	RequireApproval
)

// String implements fmt.Stringer for readable logs and audit output.
func (d DecisionType) String() string {
	switch d {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case RequireApproval:
		return "require_approval"
	default:
		return "unknown"
	}
}

// Decision is the engine's verdict on a ToolCall.
type Decision struct {
	Type   DecisionType
	Reason string // human-readable, always set (esp. for Deny/RequireApproval)
	Rule   string // which rule fired, for auditing
}

// Engine evaluates tool calls against a policy and returns a decision.
// Implementations must be safe for concurrent use.
type Engine interface {
	Check(ctx context.Context, call ToolCall) (Decision, error)
}
