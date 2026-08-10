package policy

import (
	"context"
	"testing"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

func TestEngineCheck_Tools(t *testing.T) {
	p := config.Policy{
		Agent: "refund-bot",
		Policies: config.Policies{
			Tools: config.Tools{
				Allow: []string{"get_order", "issue_refund", "send_email"},
				Deny:  []string{"delete_database", "run_shell"},
			},
		},
	}
	e := New(p)

	tests := []struct {
		name     string
		tool     string
		wantType DecisionType
		wantRule string
	}{
		{"allowed tool", "get_order", Allow, "tools.allow"},
		{"another allowed tool", "issue_refund", Allow, "tools.allow"},
		{"explicitly denied tool", "delete_database", Deny, "tools.deny"},
		{"another explicitly denied tool", "run_shell", Deny, "tools.deny"},
		{"unlisted tool", "totally_unknown_tool", Deny, "tools.default-deny"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{AgentID: "refund-bot", SessionID: "s1", Tool: tt.tool}
			got, err := e.Check(context.Background(), call)
			if err != nil {
				t.Fatalf("Check() unexpected error: %v", err)
			}
			if got.Type != tt.wantType {
				t.Errorf("Check().Type = %v, want %v", got.Type, tt.wantType)
			}
			if got.Rule != tt.wantRule {
				t.Errorf("Check().Rule = %q, want %q", got.Rule, tt.wantRule)
			}
			if got.Reason == "" {
				t.Errorf("Check().Reason should not be empty")
			}
		})
	}
}

func TestEngineCheck_DenyWinsOverAllow(t *testing.T) {
	p := config.Policy{
		Policies: config.Policies{
			Tools: config.Tools{
				Allow: []string{"issue_refund"},
				Deny:  []string{"issue_refund"},
			},
		},
	}
	e := New(p)

	got, err := e.Check(context.Background(), ToolCall{Tool: "issue_refund"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("Check().Type = %v, want Deny", got.Type)
	}
	if got.Rule != "tools.deny" {
		t.Errorf("Check().Rule = %q, want %q", got.Rule, "tools.deny")
	}
}

func TestEngineCheck_EmptyPolicyDeniesEverything(t *testing.T) {
	e := New(config.Policy{})

	got, err := e.Check(context.Background(), ToolCall{Tool: "anything"})
	if err != nil {
		t.Fatalf("Check() unexpected error: %v", err)
	}
	if got.Type != Deny {
		t.Errorf("Check().Type = %v, want Deny", got.Type)
	}
	if got.Rule != "tools.default-deny" {
		t.Errorf("Check().Rule = %q, want %q", got.Rule, "tools.default-deny")
	}
}
