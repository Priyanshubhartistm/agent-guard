# agent-guardrail

An independent safety layer that sits between an AI agent and the tools it
can call, enforcing human-written policies on every tool call. Think
**"sudo + firewall for AI agents."**

```
AI Agent  ──wants to call a tool──►  Guardrail Engine  ──►  decision
                                          │
                          ┌───────────────┼───────────────┐
                          ▼               ▼               ▼
                       ALLOW          ASK HUMAN         BLOCK
                   (run the tool)  (wait for approval) (reject + reason)
                          │
                          ▼
                     Real tool executes → result returned to agent
       Every decision is written to an audit log.
```

## Status

Early scaffolding. Core types are defined; no enforcement logic yet.

## Layout

- `cmd/guardrail/` — CLI / server entry point
- `internal/policy/` — engine core (decision types, `Engine` interface)
- `internal/config/` — policy YAML loading
- `internal/audit/` — decision audit logging
- `examples/` — example policies and sample agents

## Config reference

See `examples/refund-bot-policy.yaml` for a full example. Sections:
`tools` (allow/deny), `constraints` (per-argument rules), `limits`
(rate + spend), `sequence` (finite state machine over tool calls).

More detail will be added here as each section is implemented.
