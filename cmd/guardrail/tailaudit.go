package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// runTailAudit implements `guardrail tail-audit`: it reads JSON-lines audit
// records (from -file, or stdin if omitted) and pretty-prints one line per
// record.
func runTailAudit(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("tail-audit", flag.ContinueOnError)
	path := fs.String("file", "", "audit log file to read (default: stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r := stdin
	if *path != "" {
		f, err := os.Open(*path)
		if err != nil {
			return fmt.Errorf("open %s: %w", *path, err)
		}
		defer f.Close()
		r = f
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			fmt.Fprintf(stdout, "(unparsable line: %v)\n", err)
			continue
		}
		fmt.Fprintln(stdout, formatAuditRecord(rec))
	}
	return scanner.Err()
}

func formatAuditRecord(rec map[string]any) string {
	return fmt.Sprintf("%s  %-16s agent=%s session=%s tool=%s rule=%s reason=%q",
		recStr(rec, "timestamp"),
		strings.ToUpper(recStr(rec, "decision")),
		recStr(rec, "agent"),
		recStr(rec, "session"),
		recStr(rec, "tool"),
		recStr(rec, "rule"),
		recStr(rec, "reason"),
	)
}

func recStr(rec map[string]any, key string) string {
	s, _ := rec[key].(string)
	return s
}
