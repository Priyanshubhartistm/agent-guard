package mcpproxy

import (
	"context"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

type getOrderArgs struct {
	OrderID string `json:"order_id"`
}

type issueRefundArgs struct {
	Amount float64 `json:"amount"`
}

// newFakeUpstream starts an in-memory MCP server exposing get_order and
// issue_refund tools, and returns a Transport a client can connect through,
// plus a counter of how many times issue_refund was actually invoked (to
// prove a blocked call never reaches it).
func newFakeUpstream(t *testing.T) (mcp.Transport, *atomic.Int32) {
	t.Helper()

	var refundCalls atomic.Int32

	server := mcp.NewServer(&mcp.Implementation{Name: "fake-upstream", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "get_order", Description: "fetch an order"},
		func(_ context.Context, _ *mcp.CallToolRequest, in getOrderArgs) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "order " + in.OrderID + ": status=shipped, secret_token=sk-live-abc123"}},
			}, nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "issue_refund", Description: "issue a refund"},
		func(_ context.Context, _ *mcp.CallToolRequest, _ issueRefundArgs) (*mcp.CallToolResult, any, error) {
			refundCalls.Add(1)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "refunded"}},
			}, nil, nil
		})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(t.Context(), serverTransport) }()

	return clientTransport, &refundCalls
}

func refundBotPolicy(onViolation string) config.Policy {
	max := 500.0
	return config.Policy{
		Agent: "test-agent",
		Policies: config.Policies{
			Tools: config.Tools{Allow: []string{"get_order", "issue_refund"}},
			Constraints: []config.Constraint{
				{Tool: "issue_refund", Arg: "amount", Max: &max, OnViolation: onViolation},
			},
		},
	}
}

// connectDownstream runs p on a fresh in-memory transport pair and returns
// a client session already connected to it.
func connectDownstream(t *testing.T, p *Proxy) *mcp.ClientSession {
	t.Helper()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _ = p.Run(t.Context(), serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestProxy_AllowedCallForwardsToUpstream(t *testing.T) {
	ctx := t.Context()
	upstreamTransport, refundCalls := newFakeUpstream(t)

	g, err := guard.New(refundBotPolicy("deny"))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}

	p, err := New(ctx, g, "test-agent", upstreamTransport)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer p.Close()

	session := connectDownstream(t, p)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_order", Arguments: map[string]any{"order_id": "42"}})
	if err != nil {
		t.Fatalf("CallTool(get_order) unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(get_order) result is an error: %+v", res)
	}
	if text := textOf(t, res); !strings.Contains(text, "order 42") {
		t.Errorf("result text = %q, want it to contain %q", text, "order 42")
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "issue_refund", Arguments: map[string]any{"amount": 100.0}})
	if err != nil {
		t.Fatalf("CallTool(issue_refund) unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool(issue_refund) result is an error: %+v", res)
	}
	if refundCalls.Load() != 1 {
		t.Errorf("refundCalls = %d, want 1", refundCalls.Load())
	}
}

func TestProxy_DeniedCallNeverReachesUpstream(t *testing.T) {
	ctx := t.Context()
	upstreamTransport, refundCalls := newFakeUpstream(t)

	g, err := guard.New(refundBotPolicy("deny"))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}

	p, err := New(ctx, g, "test-agent", upstreamTransport)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer p.Close()

	session := connectDownstream(t, p)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "issue_refund", Arguments: map[string]any{"amount": 5000.0}})
	if err != nil {
		t.Fatalf("CallTool() unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("CallTool() result = %+v, want IsError true", res)
	}
	if text := textOf(t, res); !strings.Contains(text, "blocked by guardrail") {
		t.Errorf("result text = %q, want it to mention being blocked", text)
	}
	if refundCalls.Load() != 0 {
		t.Errorf("refundCalls = %d, want 0 (upstream must not be called)", refundCalls.Load())
	}
}

func TestProxy_OutputValidatorRedactsSecrets(t *testing.T) {
	ctx := t.Context()
	upstreamTransport, _ := newFakeUpstream(t)

	g, err := guard.New(refundBotPolicy("deny"))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}

	secretPattern := regexp.MustCompile(`sk-live-\w+`)
	p, err := New(ctx, g, "test-agent", upstreamTransport, WithOutputValidator(RedactText(secretPattern)))
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer p.Close()

	session := connectDownstream(t, p)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_order", Arguments: map[string]any{"order_id": "1"}})
	if err != nil {
		t.Fatalf("CallTool() unexpected error: %v", err)
	}
	text := textOf(t, res)
	if strings.Contains(text, "sk-live-abc123") {
		t.Errorf("result text = %q, secret was not redacted", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("result text = %q, want it to contain [REDACTED]", text)
	}
}

func TestProxy_RequireApprovalBlocksUntilApproved(t *testing.T) {
	ctx := t.Context()
	upstreamTransport, refundCalls := newFakeUpstream(t)

	store := approval.NewStore()
	g, err := guard.New(refundBotPolicy("require_approval"), guard.WithApproval(store, 5*time.Second))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}

	p, err := New(ctx, g, "test-agent", upstreamTransport)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer p.Close()

	session := connectDownstream(t, p)

	type callResult struct {
		res *mcp.CallToolResult
		err error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "issue_refund", Arguments: map[string]any{"amount": 5000.0}})
		resultCh <- callResult{res, err}
	}()

	var pendingID string
	deadline := time.After(2 * time.Second)
	for pendingID == "" {
		if list := store.List(); len(list) == 1 {
			pendingID = list[0].ID
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := store.Approve(pendingID); err != nil {
		t.Fatalf("Approve() unexpected error: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("CallTool() unexpected error: %v", r.err)
		}
		if r.res.IsError {
			t.Fatalf("CallTool() result = %+v, want success after approval", r.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CallTool to return")
	}

	if refundCalls.Load() != 1 {
		t.Errorf("refundCalls = %d, want 1 (forwarded only after approval)", refundCalls.Load())
	}
}

func TestProxy_RequireApprovalRejectedNeverReachesUpstream(t *testing.T) {
	ctx := t.Context()
	upstreamTransport, refundCalls := newFakeUpstream(t)

	store := approval.NewStore()
	g, err := guard.New(refundBotPolicy("require_approval"), guard.WithApproval(store, 5*time.Second))
	if err != nil {
		t.Fatalf("guard.New() unexpected error: %v", err)
	}

	p, err := New(ctx, g, "test-agent", upstreamTransport)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer p.Close()

	session := connectDownstream(t, p)

	type callResult struct {
		res *mcp.CallToolResult
		err error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "issue_refund", Arguments: map[string]any{"amount": 5000.0}})
		resultCh <- callResult{res, err}
	}()

	var pendingID string
	deadline := time.After(2 * time.Second)
	for pendingID == "" {
		if list := store.List(); len(list) == 1 {
			pendingID = list[0].ID
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pending approval")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := store.Reject(pendingID); err != nil {
		t.Fatalf("Reject() unexpected error: %v", err)
	}

	select {
	case r := <-resultCh:
		if r.err != nil {
			t.Fatalf("CallTool() unexpected error: %v", r.err)
		}
		if !r.res.IsError {
			t.Fatalf("CallTool() result = %+v, want IsError true after rejection", r.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CallTool to return")
	}

	if refundCalls.Load() != 0 {
		t.Errorf("refundCalls = %d, want 0 (rejected call must not reach upstream)", refundCalls.Load())
	}
}
