package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunTailAudit(t *testing.T) {
	input := strings.Join([]string{
		`{"timestamp":"2026-08-11T12:00:00Z","agent":"refund-bot","session":"s1","tool":"get_order","args":{},"decision":"allow","reason":"tool \"get_order\" is allowed","rule":"tools.allow"}`,
		`{"timestamp":"2026-08-11T12:00:01Z","agent":"refund-bot","session":"s1","tool":"run_shell","args":{},"decision":"deny","reason":"tool \"run_shell\" is explicitly denied","rule":"tools.deny"}`,
		"",
		"not json",
	}, "\n")

	var out bytes.Buffer
	err := runTailAudit(nil, strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("runTailAudit() unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d output lines, want 3 (output: %q)", len(lines), out.String())
	}

	if !strings.Contains(lines[0], "ALLOW") || !strings.Contains(lines[0], "get_order") || !strings.Contains(lines[0], "tools.allow") {
		t.Errorf("line 0 = %q, missing expected fields", lines[0])
	}
	if !strings.Contains(lines[1], "DENY") || !strings.Contains(lines[1], "run_shell") || !strings.Contains(lines[1], "tools.deny") {
		t.Errorf("line 1 = %q, missing expected fields", lines[1])
	}
	if !strings.Contains(lines[2], "unparsable") {
		t.Errorf("line 2 = %q, want it to flag the unparsable input", lines[2])
	}
}

func TestRunTailAudit_EmptyInput(t *testing.T) {
	var out bytes.Buffer
	if err := runTailAudit(nil, strings.NewReader(""), &out); err != nil {
		t.Fatalf("runTailAudit() unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want empty", out.String())
	}
}

func TestRunTailAudit_FileNotFound(t *testing.T) {
	var out bytes.Buffer
	err := runTailAudit([]string{"-file", "does-not-exist.log"}, strings.NewReader(""), &out)
	if err == nil {
		t.Fatal("runTailAudit() expected error for missing file, got nil")
	}
}
