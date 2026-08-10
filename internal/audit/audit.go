// Package audit provides structured, pluggable logging of every policy
// decision: timestamp, agent, session, tool, args, decision, reason, rule.
package audit

import (
	"context"
	"io"
	"log/slog"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// Redactor transforms a tool call's args before they're written to the
// audit log, e.g. to mask secrets. It must not mutate its input.
type Redactor func(args map[string]any) map[string]any

// RedactKeys returns a Redactor that replaces the value of any of the given
// arg keys with "[REDACTED]", leaving every other key untouched.
func RedactKeys(keys ...string) Redactor {
	redact := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		redact[k] = struct{}{}
	}
	return func(args map[string]any) map[string]any {
		if len(args) == 0 {
			return args
		}
		out := make(map[string]any, len(args))
		for k, v := range args {
			if _, ok := redact[k]; ok {
				out[k] = "[REDACTED]"
				continue
			}
			out[k] = v
		}
		return out
	}
}

// Logger writes one JSON-lines record per policy decision.
type Logger struct {
	log    *slog.Logger
	redact Redactor
}

// New returns a Logger that writes JSON-lines audit records to w. If
// redact is non-nil, it is applied to a call's args before they're logged.
func New(w io.Writer, redact Redactor) *Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		// The record carries its own "timestamp" attribute (the tool
		// call's time), so drop slog's default top-level time key to
		// avoid two competing timestamps in the same line.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})
	return &Logger{
		log:    slog.New(handler),
		redact: redact,
	}
}

// LogDecision writes an audit record for the decision the engine made
// about call.
func (l *Logger) LogDecision(ctx context.Context, call policy.ToolCall, decision policy.Decision) {
	args := call.Args
	if l.redact != nil {
		args = l.redact(args)
	}

	l.log.LogAttrs(ctx, slog.LevelInfo, "tool_call_decision",
		slog.Time("timestamp", call.Timestamp),
		slog.String("agent", call.AgentID),
		slog.String("session", call.SessionID),
		slog.String("tool", call.Tool),
		slog.Any("args", args),
		slog.String("decision", decision.Type.String()),
		slog.String("reason", decision.Reason),
		slog.String("rule", decision.Rule),
	)
}
