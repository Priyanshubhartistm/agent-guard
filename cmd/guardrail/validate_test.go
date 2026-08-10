package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempPolicy(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile() unexpected error: %v", err)
	}
	return path
}

const validPolicyYAML = `
agent: "refund-bot"
policies:
  tools:
    allow: ["get_order", "issue_refund"]
    deny: ["run_shell"]
`

const invalidPolicyYAML = `
agent: "refund-bot"
policies:
  tools:
    allow: ["issue_refund"]
  constraints:
    - tool: "issue_refund"
      arg: "amount"
      max: 500
      on_violation: "explode"
`

func TestRunValidate(t *testing.T) {
	tests := []struct {
		name    string
		args    func(t *testing.T) []string
		wantErr bool
		wantOut string
	}{
		{
			name: "valid policy",
			args: func(t *testing.T) []string {
				return []string{writeTempPolicy(t, validPolicyYAML)}
			},
			wantErr: false,
			wantOut: "OK",
		},
		{
			name: "invalid policy (bad on_violation)",
			args: func(t *testing.T) []string {
				return []string{writeTempPolicy(t, invalidPolicyYAML)}
			},
			wantErr: true,
		},
		{
			name: "missing file",
			args: func(t *testing.T) []string {
				return []string{"does-not-exist.yaml"}
			},
			wantErr: true,
		},
		{
			name: "wrong number of args",
			args: func(t *testing.T) []string {
				return []string{}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runValidate(tt.args(t), &buf)

			if tt.wantErr && err == nil {
				t.Fatalf("runValidate() expected error, got nil (output: %s)", buf.String())
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("runValidate() unexpected error: %v", err)
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", buf.String(), tt.wantOut)
			}
		})
	}
}
