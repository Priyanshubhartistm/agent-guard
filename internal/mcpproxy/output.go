package mcpproxy

import (
	"context"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputValidator inspects — and may transform or reject — a tool's result
// before it is returned to the agent, e.g. to redact secrets or enforce a
// schema. Returning an error rejects the result outright (the agent sees a
// tool error instead of the raw upstream result).
type OutputValidator func(ctx context.Context, tool string, result *mcp.CallToolResult) (*mcp.CallToolResult, error)

// RedactText returns an OutputValidator that replaces every match of any of
// patterns, in every TextContent block of a result, with "[REDACTED]". It
// never rejects a result outright.
func RedactText(patterns ...*regexp.Regexp) OutputValidator {
	return func(_ context.Context, _ string, result *mcp.CallToolResult) (*mcp.CallToolResult, error) {
		if result == nil {
			return result, nil
		}
		for _, c := range result.Content {
			tc, ok := c.(*mcp.TextContent)
			if !ok {
				continue
			}
			for _, p := range patterns {
				tc.Text = p.ReplaceAllString(tc.Text, "[REDACTED]")
			}
		}
		return result, nil
	}
}
