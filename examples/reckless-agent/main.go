// Command reckless-agent is a deliberately dangerous demo "AI agent". It
// tries to nuke data with a shell command, issue a $50,000 refund, and
// delete a database table — interspersed with the routine, harmless calls
// a real refund bot would make. Every call, good and bad, is routed
// through agent-guardrail first, so you can see exactly what it catches
// and what it lets through.
//
// Run: go run ./examples/reckless-agent
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

//go:embed policy.yaml
var policyYAML []byte

// attempt is one thing the reckless agent decides to do.
type attempt struct {
	label string // what the agent is about to do, in plain English
	call  policy.ToolCall
}

func main() {
	p, err := config.Parse(policyYAML)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reckless-agent: parse policy:", err)
		os.Exit(1)
	}

	// No human is watching this demo, so give approval requests a short
	// timeout: they'll default-deny quickly instead of hanging forever.
	store := approval.NewStore()
	g, err := guard.New(*p, guard.WithApproval(store, 2*time.Second))
	if err != nil {
		fmt.Fprintln(os.Stderr, "reckless-agent: build guard:", err)
		os.Exit(1)
	}

	const session = "demo-session"

	attempts := []attempt{
		{
			label: "look up order #42 (routine, harmless)",
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "get_order", Args: map[string]any{"order_id": 42}},
		},
		{
			label: "issue a $50,000 refund",
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "issue_refund", Args: map[string]any{"amount": 50000, "customer": 42}},
		},
		{
			label: `run "rm -rf /data" on the host shell`,
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "run_shell", Args: map[string]any{"cmd": "rm -rf /data"}},
		},
		{
			label: "delete the customers database table",
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "delete_database", Args: map[string]any{"table": "customers"}},
		},
		{
			label: "issue a legitimate $150 refund for order #42",
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "issue_refund", Args: map[string]any{"amount": 150, "customer": 42}},
		},
		{
			label: "email the customer their refund confirmation",
			call:  policy.ToolCall{AgentID: "refund-bot", SessionID: session, Tool: "send_email", Args: map[string]any{"to": "customer@example.com"}},
		},
	}

	allowed, blocked := 0, 0

	for i, a := range attempts {
		a.call.Timestamp = time.Now()

		fmt.Printf("\n--- Attempt %d ---\n", i+1)
		fmt.Printf("Agent wants to: %s\n", a.label)
		fmt.Println("  BEFORE (no guardrail): this would just happen.")

		decision, err := g.Check(context.Background(), a.call)
		if err != nil {
			fmt.Fprintln(os.Stderr, "reckless-agent: check:", err)
			os.Exit(1)
		}

		if decision.Type == policy.Allow {
			allowed++
			fmt.Println("  AFTER  (with agent-guardrail): ALLOWED -- tool executes.")
		} else {
			blocked++
			fmt.Printf("  AFTER  (with agent-guardrail): %s -- tool never runs.\n", strings.ToUpper(decision.Type.String()))
		}
		fmt.Printf("    reason: %s\n", decision.Reason)
		fmt.Printf("    rule:   %s\n", decision.Rule)
	}

	fmt.Printf("\n=== Summary: %d call(s) allowed, %d call(s) blocked by agent-guardrail ===\n", allowed, blocked)
}
