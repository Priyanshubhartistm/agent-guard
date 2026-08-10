package policy

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecisionType_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		d    DecisionType
		want string
	}{
		{"allow", Allow, `"allow"`},
		{"deny", Deny, `"deny"`},
		{"require_approval", RequireApproval, `"require_approval"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.d)
			if err != nil {
				t.Fatalf("Marshal() unexpected error: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("Marshal() = %s, want %s", b, tt.want)
			}

			var got DecisionType
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal() unexpected error: %v", err)
			}
			if got != tt.d {
				t.Errorf("Unmarshal() = %v, want %v", got, tt.d)
			}
		})
	}
}

func TestDecisionType_UnmarshalInvalidValue(t *testing.T) {
	var d DecisionType
	if err := json.Unmarshal([]byte(`"maybe"`), &d); err == nil {
		t.Fatal("Unmarshal() expected error for invalid decision type, got nil")
	}
}

func TestDecision_JSONRoundTrip(t *testing.T) {
	want := Decision{Type: RequireApproval, Reason: "amount over max", Rule: "constraints.issue_refund.amount"}

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() unexpected error: %v", err)
	}

	var got Decision
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal() unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestToolCall_JSONRoundTrip(t *testing.T) {
	want := ToolCall{
		AgentID:   "refund-bot",
		SessionID: "s1",
		Tool:      "issue_refund",
		Args:      map[string]any{"amount": 100.0},
		Timestamp: time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC),
	}

	b, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal() unexpected error: %v", err)
	}

	var got ToolCall
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal() unexpected error: %v", err)
	}
	if got.AgentID != want.AgentID || got.SessionID != want.SessionID || got.Tool != want.Tool {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("Timestamp round trip = %v, want %v", got.Timestamp, want.Timestamp)
	}
	if got.Args["amount"] != 100.0 {
		t.Errorf("Args round trip = %v, want amount 100.0", got.Args)
	}
}
