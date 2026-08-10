// Package mcpproxy sits in front of a real MCP server: it mirrors the
// upstream server's tools onto a local MCP server, and runs every
// tools/call request through a guard.Guard before forwarding it upstream.
// Calls the engine denies never reach the real server. This makes the
// guardrail work with real MCP-based agents with zero agent-side changes:
// point the agent's MCP client at the proxy instead of the real server.
package mcpproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// Proxy is an MCP server that fronts a real ("upstream") MCP server,
// guarding every tool call with a guard.Guard before forwarding it.
type Proxy struct {
	guard    *guard.Guard
	agentID  string
	upstream *mcp.ClientSession
	server   *mcp.Server
	validate OutputValidator
}

// Option configures a Proxy at construction time.
type Option func(*Proxy)

// WithOutputValidator inspects (and may transform or reject) every tool
// result received from upstream before it is returned to the agent.
func WithOutputValidator(v OutputValidator) Option {
	return func(p *Proxy) { p.validate = v }
}

// New connects to the upstream MCP server over upstreamTransport, mirrors
// its tools onto a new local MCP server guarded by g, and returns the
// Proxy. agentID is used as ToolCall.AgentID for every intercepted call.
func New(ctx context.Context, g *guard.Guard, agentID string, upstreamTransport mcp.Transport, opts ...Option) (*Proxy, error) {
	impl := &mcp.Implementation{Name: "agent-guardrail-proxy", Version: "0.1.0"}

	client := mcp.NewClient(impl, nil)
	upstream, err := client.Connect(ctx, upstreamTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcpproxy: connect upstream: %w", err)
	}

	p := &Proxy{
		guard:    g,
		agentID:  agentID,
		upstream: upstream,
	}
	for _, opt := range opts {
		opt(p)
	}

	p.server = mcp.NewServer(impl, nil)

	for tool, err := range upstream.Tools(ctx, nil) {
		if err != nil {
			upstream.Close()
			return nil, fmt.Errorf("mcpproxy: list upstream tools: %w", err)
		}
		p.server.AddTool(tool, p.handleToolCall)
	}

	return p, nil
}

// Run serves the proxy to a downstream MCP client (typically the agent)
// over t, blocking until the session ends.
func (p *Proxy) Run(ctx context.Context, t mcp.Transport) error {
	return p.server.Run(ctx, t)
}

// Close disconnects from the upstream server.
func (p *Proxy) Close() error {
	return p.upstream.Close()
}

// blocked builds the CallToolResult returned to the agent in place of a
// forwarded call, whenever the guard did not allow the call through (or
// upstream's result was rejected by output validation). It is a tool-level
// error (IsError, not a protocol error) so the agent can see why and
// potentially self-correct, per the ToolHandler contract.
func blocked(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
	}
}

// handleToolCall is the mcp.ToolHandler installed for every mirrored tool.
func (p *Proxy) handleToolCall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args map[string]any
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return blocked("guardrail: invalid tool arguments: %v", err), nil
		}
	}

	call := policy.ToolCall{
		AgentID:   p.agentID,
		SessionID: req.Session.ID(),
		Tool:      req.Params.Name,
		Args:      args,
		Timestamp: time.Now(),
	}

	decision, err := p.guard.Check(ctx, call)
	if err != nil {
		return nil, err
	}
	if decision.Type != policy.Allow {
		return blocked("blocked by guardrail (%s): %s", decision.Rule, decision.Reason), nil
	}

	result, err := p.upstream.CallTool(ctx, &mcp.CallToolParams{
		Name:      req.Params.Name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	if p.validate != nil {
		validated, err := p.validate(ctx, req.Params.Name, result)
		if err != nil {
			return blocked("guardrail: output rejected: %v", err), nil
		}
		result = validated
	}

	return result, nil
}
