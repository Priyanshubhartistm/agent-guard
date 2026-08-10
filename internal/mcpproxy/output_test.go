package mcpproxy

import (
	"context"
	"regexp"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRedactText(t *testing.T) {
	redact := RedactText(regexp.MustCompile(`sk-live-\w+`), regexp.MustCompile(`\bhunter2\b`))

	result := &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "token=sk-live-abc123 password=hunter2"},
			&mcp.TextContent{Text: "no secrets here"},
		},
	}

	got, err := redact(context.Background(), "get_order", result)
	if err != nil {
		t.Fatalf("RedactText() unexpected error: %v", err)
	}

	first := got.Content[0].(*mcp.TextContent).Text
	if first != "token=[REDACTED] password=[REDACTED]" {
		t.Errorf("Content[0].Text = %q, want redacted", first)
	}
	second := got.Content[1].(*mcp.TextContent).Text
	if second != "no secrets here" {
		t.Errorf("Content[1].Text = %q, want unchanged", second)
	}
}

func TestRedactText_NilResult(t *testing.T) {
	redact := RedactText(regexp.MustCompile(`secret`))
	got, err := redact(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("RedactText() unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got = %v, want nil", got)
	}
}

func TestRedactText_NonTextContentUntouched(t *testing.T) {
	redact := RedactText(regexp.MustCompile(`secret`))
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.EmbeddedResource{}},
	}

	got, err := redact(context.Background(), "tool", result)
	if err != nil {
		t.Fatalf("RedactText() unexpected error: %v", err)
	}
	if len(got.Content) != 1 {
		t.Fatalf("Content = %v, want 1 untouched item", got.Content)
	}
}
