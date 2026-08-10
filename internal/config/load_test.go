package config

import "testing"

const examplePolicyYAML = `
agent: "refund-bot"
policies:
  tools:
    allow: ["get_order", "issue_refund", "send_email"]
    deny: ["delete_database", "run_shell"]
  constraints:
    - tool: "issue_refund"
      arg: "amount"
      max: 500
      on_violation: require_approval
  limits:
    rate:
      max_calls_per_minute: 30
    spend:
      currency: "USD"
      meter: { tool: "issue_refund", arg: "amount" }
      max_per_session: 2000
  sequence:
    initial: "start"
    transitions:
      - { from: "start", tool: "get_order", to: "order_loaded" }
      - { from: "order_loaded", tool: "issue_refund", to: "refunded" }
      - { from: "refunded", tool: "send_email", to: "done" }
`

func TestParse(t *testing.T) {
	p, err := Parse([]byte(examplePolicyYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if p.Agent != "refund-bot" {
		t.Errorf("Agent = %q, want %q", p.Agent, "refund-bot")
	}

	wantAllow := []string{"get_order", "issue_refund", "send_email"}
	if !equalStrings(p.Policies.Tools.Allow, wantAllow) {
		t.Errorf("Tools.Allow = %v, want %v", p.Policies.Tools.Allow, wantAllow)
	}

	wantDeny := []string{"delete_database", "run_shell"}
	if !equalStrings(p.Policies.Tools.Deny, wantDeny) {
		t.Errorf("Tools.Deny = %v, want %v", p.Policies.Tools.Deny, wantDeny)
	}

	if len(p.Policies.Constraints) != 1 {
		t.Fatalf("len(Constraints) = %d, want 1", len(p.Policies.Constraints))
	}
	c := p.Policies.Constraints[0]
	if c.Tool != "issue_refund" || c.Arg != "amount" || c.OnViolation != "require_approval" {
		t.Errorf("unexpected constraint: %+v", c)
	}
	if c.Max == nil || *c.Max != 500 {
		t.Errorf("Constraint.Max = %v, want 500", c.Max)
	}

	if p.Policies.Limits.Rate.MaxCallsPerMinute != 30 {
		t.Errorf("Rate.MaxCallsPerMinute = %d, want 30", p.Policies.Limits.Rate.MaxCallsPerMinute)
	}
	if p.Policies.Limits.Spend.MaxPerSession != 2000 {
		t.Errorf("Spend.MaxPerSession = %v, want 2000", p.Policies.Limits.Spend.MaxPerSession)
	}
	if p.Policies.Limits.Spend.Meter.Tool != "issue_refund" || p.Policies.Limits.Spend.Meter.Arg != "amount" {
		t.Errorf("unexpected meter: %+v", p.Policies.Limits.Spend.Meter)
	}

	if p.Policies.Sequence.Initial != "start" {
		t.Errorf("Sequence.Initial = %q, want %q", p.Policies.Sequence.Initial, "start")
	}
	if len(p.Policies.Sequence.Transitions) != 3 {
		t.Fatalf("len(Transitions) = %d, want 3", len(p.Policies.Sequence.Transitions))
	}
}

func TestLoad(t *testing.T) {
	p, err := Load("../../examples/refund-bot-policy.yaml")
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if p.Agent != "refund-bot" {
		t.Errorf("Agent = %q, want %q", p.Agent, "refund-bot")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load("does-not-exist.yaml"); err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
