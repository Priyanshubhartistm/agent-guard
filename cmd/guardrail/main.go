// Command guardrail is the CLI/server entry point for the AI agent
// guardrail & policy engine.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:], os.Stdout)
	case "tail-audit":
		err = runTailAudit(os.Args[2:], os.Stdin, os.Stdout)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "guardrail: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardrail:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `guardrail is the CLI/server for the AI agent guardrail & policy engine.

Usage:
  guardrail serve -policy <policy.yaml> [-addr :8080] [-audit-log <file>] [-approval-timeout 5m]
  guardrail validate <policy.yaml>
  guardrail tail-audit [-file <audit.log>]`)
}
