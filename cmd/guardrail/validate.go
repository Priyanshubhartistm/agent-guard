package main

import (
	"fmt"
	"io"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

// runValidate implements `guardrail validate <policy.yaml>`: it loads and
// fully compiles the policy (catching bad regexes, bad on_violation
// values, etc.) without starting a server.
func runValidate(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: guardrail validate <policy.yaml>")
	}
	path := args[0]

	p, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	if _, err := policy.New(*p); err != nil {
		return fmt.Errorf("invalid policy: %w", err)
	}

	fmt.Fprintf(stdout, "%s: OK (agent %q)\n", path, p.Agent)
	return nil
}
