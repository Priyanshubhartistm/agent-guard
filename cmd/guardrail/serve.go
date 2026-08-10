package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/guard"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/approval"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
	"github.com/Priyanshubhartistm/agent-guardrail/internal/server"
)

// buildServeHandler loads the policy at policyPath and wires up a Guard
// (audit logging to auditWriter, human-in-the-loop approval with the given
// timeout), returning the HTTP handler that serves it. Split out from
// runServe so it's testable without binding a real network listener.
func buildServeHandler(policyPath string, auditWriter io.Writer, approvalTimeout time.Duration) (http.Handler, error) {
	p, err := config.Load(policyPath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", policyPath, err)
	}

	g, err := guard.New(*p,
		guard.WithApproval(approval.NewStore(), approvalTimeout),
		guard.WithAudit(auditWriter, nil),
		guard.WithHistory(100),
	)
	if err != nil {
		return nil, fmt.Errorf("build engine: %w", err)
	}

	return server.NewHandler(g), nil
}

// runServe implements `guardrail serve`.
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	policyPath := fs.String("policy", "", "path to policy YAML (required)")
	addr := fs.String("addr", ":8080", "address to listen on")
	auditPath := fs.String("audit-log", "", "path to append JSON-lines audit records (default: stdout)")
	approvalTimeout := fs.Duration("approval-timeout", 5*time.Minute, "how long to wait for a human to approve/reject before defaulting to deny")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *policyPath == "" {
		return fmt.Errorf("usage: guardrail serve -policy <policy.yaml> [-addr :8080] [-audit-log <file>] [-approval-timeout 5m]")
	}

	var w io.Writer = os.Stdout
	if *auditPath != "" {
		f, err := os.OpenFile(*auditPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open audit log: %w", err)
		}
		defer f.Close()
		w = f
	}

	handler, err := buildServeHandler(*policyPath, w, *approvalTimeout)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "guardrail: listening on %s (policy: %s)\n", *addr, *policyPath)
	return http.ListenAndServe(*addr, handler)
}
