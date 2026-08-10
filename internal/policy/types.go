// Package policy defines the core types and the decision engine interface
// for the AI agent guardrail. The engine evaluates every tool call an agent
// wants to make against a declarative policy, before the tool executes.
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolCall is a single tool invocation an agent wants to make.
type ToolCall struct {
	AgentID   string         `json:"agent_id"`   // which agent/policy set applies
	SessionID string         `json:"session_id"` // groups calls for rate/spend/sequence state
	Tool      string         `json:"tool"`       // e.g. "issue_refund"
	Args      map[string]any `json:"args"`       // e.g. {"amount": 5000, "customer": 123}
	Timestamp time.Time      `json:"timestamp"`
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

// MarshalJSON encodes DecisionType as its string form (e.g. "allow"), not
// the underlying int, so JSON API consumers don't need to know the iota
// values.
func (d DecisionType) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON decodes DecisionType from its string form.
func (d *DecisionType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case "allow":
		*d = Allow
	case "deny":
		*d = Deny
	case "require_approval":
		*d = RequireApproval
	default:
		return fmt.Errorf("policy: invalid decision type %q", s)
	}
	return nil
}

// Decision is the engine's verdict on a ToolCall.
type Decision struct {
	Type   DecisionType `json:"type"`
	Reason string       `json:"reason"` // human-readable, always set (esp. for Deny/RequireApproval)
	Rule   string       `json:"rule"`   // which rule fired, for auditing
}

// Engine evaluates tool calls against a policy and returns a decision.
// Implementations must be safe for concurrent use.
type Engine interface {
	Check(ctx context.Context, call ToolCall) (Decision, error)
}
