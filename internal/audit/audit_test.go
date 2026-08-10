package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func TestLogger_LogDecision(t *testing.T) {
	ts := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		call       policy.ToolCall
		decision   policy.Decision
		wantFields map[string]string
	}{
		{
			name: "allowed call",
			call: policy.ToolCall{
				AgentID:   "refund-bot",
				SessionID: "s1",
				Tool:      "get_order",
				Args:      map[string]any{"order_id": 42},
				Timestamp: ts,
			},
			decision: policy.Decision{
				Type:   policy.Allow,
				Reason: `tool "get_order" is allowed`,
				Rule:   "tools.allow",
			},
			wantFields: map[string]string{
				"agent":    "refund-bot",
				"session":  "s1",
				"tool":     "get_order",
				"decision": "allow",
				"reason":   `tool "get_order" is allowed`,
				"rule":     "tools.allow",
			},
		},
		{
			name: "denied call",
			call: policy.ToolCall{
				AgentID:   "refund-bot",
				SessionID: "s1",
				Tool:      "delete_database",
				Args:      map[string]any{},
				Timestamp: ts,
			},
			decision: policy.Decision{
				Type:   policy.Deny,
				Reason: `tool "delete_database" is explicitly denied`,
				Rule:   "tools.deny",
			},
			wantFields: map[string]string{
				"agent":    "refund-bot",
				"session":  "s1",
				"tool":     "delete_database",
				"decision": "deny",
				"reason":   `tool "delete_database" is explicitly denied`,
				"rule":     "tools.deny",
			},
		},
		{
			name: "require_approval call",
			call: policy.ToolCall{
				AgentID:   "refund-bot",
				SessionID: "s1",
				Tool:      "issue_refund",
				Args:      map[string]any{"amount": 5000},
				Timestamp: ts,
			},
			decision: policy.Decision{
				Type:   policy.RequireApproval,
				Reason: `tool "issue_refund" arg "amount" exceeds max 500`,
				Rule:   "constraints.issue_refund.amount",
			},
			wantFields: map[string]string{
				"decision": "require_approval",
				"rule":     "constraints.issue_refund.amount",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(&buf, nil)

			logger.LogDecision(context.Background(), tt.call, tt.decision)

			line := strings.TrimSpace(buf.String())
			if line == "" {
				t.Fatal("LogDecision() wrote no output")
			}

			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("output is not valid JSON: %v\nline: %s", err, line)
			}

			for key, want := range tt.wantFields {
				got, ok := record[key]
				if !ok {
					t.Errorf("record missing key %q (record: %v)", key, record)
					continue
				}
				if got != want {
					t.Errorf("record[%q] = %v, want %v", key, got, want)
				}
			}

			gotTimestamp, ok := record["timestamp"]
			if !ok {
				t.Fatal("record missing key \"timestamp\"")
			}
			gotTime, err := time.Parse(time.RFC3339Nano, gotTimestamp.(string))
			if err != nil {
				t.Fatalf("timestamp %q is not RFC3339: %v", gotTimestamp, err)
			}
			if !gotTime.Equal(ts) {
				t.Errorf("timestamp = %v, want %v", gotTime, ts)
			}

			args, ok := record["args"].(map[string]any)
			if !ok {
				t.Fatalf("record[\"args\"] is not an object: %v", record["args"])
			}
			if len(args) != len(tt.call.Args) {
				t.Errorf("args = %v, want %v", args, tt.call.Args)
			}
		})
	}
}

func TestLogger_LogDecision_WritesOneLinePerCall(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, nil)
	ctx := context.Background()

	logger.LogDecision(ctx, policy.ToolCall{Tool: "get_order"}, policy.Decision{Type: policy.Allow, Reason: "ok", Rule: "tools.allow"})
	logger.LogDecision(ctx, policy.ToolCall{Tool: "run_shell"}, policy.Decision{Type: policy.Deny, Reason: "blocked", Rule: "tools.deny"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (output: %q)", len(lines), buf.String())
	}
	for i, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestLogger_LogDecision_Redaction(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, RedactKeys("api_key", "password"))

	call := policy.ToolCall{
		Tool: "call_api",
		Args: map[string]any{
			"api_key":  "sk-super-secret",
			"password": "hunter2",
			"endpoint": "/v1/orders",
		},
	}
	logger.LogDecision(context.Background(), call, policy.Decision{Type: policy.Allow, Reason: "ok", Rule: "tools.allow"})

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	args := record["args"].(map[string]any)

	if args["api_key"] != "[REDACTED]" {
		t.Errorf(`args["api_key"] = %v, want "[REDACTED]"`, args["api_key"])
	}
	if args["password"] != "[REDACTED]" {
		t.Errorf(`args["password"] = %v, want "[REDACTED]"`, args["password"])
	}
	if args["endpoint"] != "/v1/orders" {
		t.Errorf(`args["endpoint"] = %v, want "/v1/orders" (unredacted)`, args["endpoint"])
	}

	// The original call args must not be mutated by redaction.
	if call.Args["api_key"] != "sk-super-secret" {
		t.Errorf("original call.Args was mutated: %v", call.Args)
	}
}

func TestLogger_LogDecision_NoRedactorLeavesArgsUnchanged(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, nil)

	call := policy.ToolCall{
		Tool: "call_api",
		Args: map[string]any{"api_key": "sk-super-secret"},
	}
	logger.LogDecision(context.Background(), call, policy.Decision{Type: policy.Allow, Reason: "ok", Rule: "tools.allow"})

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	args := record["args"].(map[string]any)
	if args["api_key"] != "sk-super-secret" {
		t.Errorf(`args["api_key"] = %v, want unredacted value`, args["api_key"])
	}
}
