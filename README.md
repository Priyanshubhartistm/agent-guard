# agent-guardrail

An independent safety layer that sits between an AI agent and the tools it
can call, enforcing human-written policies on every tool call. Think
**"sudo + firewall for AI agents."**

In agent systems, an LLM decides *which* tool to call and with *what*
arguments. LLMs are unpredictable — they hallucinate, get manipulated by
prompt injection, or make reasoning errors — and most frameworks let the LLM
be both the decision-maker *and* the executor, with no independent check.
One bad decision can delete data, spend real money, or take a destructive
action. agent-guardrail is the independent check: every tool call is
intercepted and evaluated against declarative policies **before** it
executes.

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

Design principles: **fail-closed** (a tool not explicitly allowed is
denied), **declarative** (policy lives in YAML, not code), **deterministic
and fast** (no network calls in the decision core), **concurrency-safe**
(many agents/sessions at once), and usable both as an embeddable **Go
library** and a standalone **server/proxy**.

## Quickstart

```bash
git clone https://github.com/Priyanshubhartistm/agent-guard.git
cd agent-guard
go build ./...
```

### 1. Validate a policy

```bash
go run ./cmd/guardrail validate examples/refund-bot-policy.yaml
# examples/refund-bot-policy.yaml: OK (agent "refund-bot")
```

### 2. See it block dangerous calls

```bash
go run ./examples/reckless-agent
```

This runs a demo "agent" that tries a $50,000 refund, `rm -rf /data`, and
deleting a database table — see [Before / after](#before--after) below for
the real output.

### 3. Run the server

```bash
go run ./cmd/guardrail serve -policy examples/refund-bot-policy.yaml -addr :8080
```

Intercept a tool call:

```bash
curl -s -X POST localhost:8080/check -d '{
  "agent_id": "refund-bot",
  "session_id": "s1",
  "tool": "issue_refund",
  "args": {"amount": 5000}
}'
# {"type":"require_approval","reason":"tool \"issue_refund\" arg \"amount\" value 5000 exceeds max 500","rule":"constraints.issue_refund.amount"}
```

Since that call requires approval, it's parked pending a human decision —
approve or reject it:

```bash
curl -s localhost:8080/pending
# [{"ID":"1","Call":{...},"Reason":"...","Rule":"constraints.issue_refund.amount"}]

curl -s -X POST localhost:8080/pending/1/approve   # or /reject
```

(If nobody resolves it, it defaults to **deny** after the configured
timeout — fail-closed, per the design principles above.)

### 4. Embed it as a library

```go
import (
    "github.com/Priyanshubhartistm/agent-guardrail/guard"
    "github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

g, err := guard.NewFromFile("policy.yaml")
if err != nil {
    log.Fatal(err)
}

decision, err := g.Check(ctx, policy.ToolCall{
    AgentID:   "refund-bot",
    SessionID: session.ID,
    Tool:      "issue_refund",
    Args:      map[string]any{"amount": 5000},
})
if decision.Type != policy.Allow {
    return fmt.Errorf("blocked: %s", decision.Reason)
}
// ... only now actually call the real tool
```

## How it works

1. Write a policy in YAML (see [Config reference](#config-reference)).
2. Every tool call your agent wants to make is checked against it — via the
   `guard` library, the HTTP server, or the MCP proxy — **before** the real
   tool runs.
3. The engine evaluates, in order: **tool allow/deny** → **argument
   constraints** → **sequence rules** → **rate limit** → **spend limit**.
   The first rule that doesn't say Allow wins.
4. Every decision is written to a structured audit log.
5. A `require_approval` decision parks the call and blocks until a human
   approves or rejects it (or it times out, which defaults to deny).

## Project layout

```
cmd/guardrail/        CLI: serve, validate, tail-audit
guard/                 public library API — embed the engine directly
internal/policy/       the pure decision core (Engine.Check)
internal/config/       policy YAML loading
internal/audit/        structured JSON-lines decision logging
internal/approval/      pending-approvals store + HTTP endpoints
internal/server/       HTTP server wrapping guard.Guard
internal/mcpproxy/     MCP proxy: guards a real MCP server's tools/call
examples/              example policies and the reckless-agent demo
```

## Config reference

A policy is a YAML file with an `agent` name and a `policies` block. See
[`examples/refund-bot-policy.yaml`](examples/refund-bot-policy.yaml) for a
complete, working example (also used by the demo below).

```yaml
agent: "refund-bot"
policies:
  tools:
    allow: ["get_order", "issue_refund", "send_email"]
    deny: ["delete_database", "run_shell"]

  constraints:
    - tool: "issue_refund"
      arg: "amount"
      max: 500
      on_violation: require_approval

  limits:
    rate:
      max_calls_per_minute: 30
    spend:
      currency: "USD"
      meter: { tool: "issue_refund", arg: "amount" }
      max_per_session: 2000

  sequence:
    initial: "start"
    transitions:
      - { from: "start", tool: "get_order", to: "order_loaded" }
      - { from: "order_loaded", tool: "issue_refund", to: "refunded" }
      - { from: "refunded", tool: "send_email", to: "done" }
```

### `tools`

Which tools may be called at all. **Fail-closed**: any tool not in `allow`
is denied. `deny` is checked first and always wins, even if the same tool
also appears in `allow`.

| Field   | Type       | Meaning                              |
|---------|------------|---------------------------------------|
| `allow` | `[]string` | Tools permitted to run.               |
| `deny`  | `[]string` | Tools always blocked, deny wins over allow. |

### `constraints`

Per-argument rules on a specific tool. Multiple constraints may target the
same tool/arg; the first one violated wins.

| Field          | Type     | Meaning                                                        |
|----------------|----------|------------------------------------------------------------------|
| `tool`         | string   | Tool this constraint applies to.                                |
| `arg`          | string   | Argument name to check.                                         |
| `max` / `min`  | number   | Numeric bound. Works across `int`/`float64`/`json.Number`.      |
| `equals`       | any      | Value must exactly equal this.                                  |
| `one_of`       | `[]any`  | Value must be one of these.                                     |
| `regex`        | string   | Value (stringified) must match this pattern.                    |
| `on_violation` | string   | `deny` or `require_approval`. Required.                         |

A missing arg never violates a constraint (there's nothing to check). A
present-but-non-numeric value against `max`/`min` is treated as a
violation (fail-closed).

### `limits`

Tracked per `session_id`, in memory (the interface is Redis-ready — see
[`internal/policy/state.go`](internal/policy/state.go)).

| Field                             | Type   | Meaning                                              |
|------------------------------------|--------|-------------------------------------------------------|
| `rate.max_calls_per_minute`        | int    | Token-bucket limit per session. 0/omitted = unlimited. |
| `spend.currency`                   | string | Informational (e.g. `"USD"`).                          |
| `spend.meter.tool` / `.arg`        | string | Which tool+arg counts as "spending money".            |
| `spend.max_per_session`            | number | Cap on the summed metered amount per session. A call that would exceed it is denied, and the amount is **not** counted (it never happened). |

### `sequence`

An optional finite state machine over tool calls, tracked per
`session_id`. If omitted, calls are unrestricted by ordering.

| Field         | Type            | Meaning                                            |
|---------------|-----------------|------------------------------------------------------|
| `initial`     | string          | Starting state for a new session.                   |
| `transitions` | `[]{from,tool,to}` | Valid `(state, tool) → state` edges. A tool call with no matching transition from the session's current state is denied. |

## HTTP API (`guardrail serve`)

| Method & path                  | Purpose                                              |
|---------------------------------|--------------------------------------------------------|
| `POST /check`                    | Interception endpoint. Body: a `ToolCall` JSON object (`agent_id`, `session_id`, `tool`, `args`, `timestamp`). Returns a `Decision` (`type`, `reason`, `rule`). |
| `GET /pending`                   | List calls currently awaiting human approval.        |
| `POST /pending/{id}/approve`     | Approve a pending call — it resolves to `allow`.      |
| `POST /pending/{id}/reject`      | Reject a pending call — it resolves to `deny`.        |

`guardrail serve` flags: `-policy <file>` (required), `-addr` (default
`:8080`), `-audit-log <file>` (default: stdout), `-approval-timeout`
(default `5m`).

## CLI

```
guardrail serve -policy <policy.yaml> [-addr :8080] [-audit-log <file>] [-approval-timeout 5m]
guardrail validate <policy.yaml>
guardrail tail-audit [-file <audit.log>]
```

- `validate` fully compiles the policy (catches a bad regex or an invalid
  `on_violation` value) without starting a server.
- `tail-audit` reads JSON-lines audit records (from a file, or stdin — so
  it composes with `tail -f audit.log | guardrail tail-audit`) and
  pretty-prints one line per decision.

## Before / after

[`examples/reckless-agent`](examples/reckless-agent) is a demo "agent" that
tries a $50,000 refund, `rm -rf /data`, and deleting a database table,
alongside the routine calls a real refund bot would make — all against
[`examples/reckless-agent/policy.yaml`](examples/reckless-agent/policy.yaml)
(the same policy shown above). Run it yourself with
`go run ./examples/reckless-agent`. Real output:

```
--- Attempt 1 ---
Agent wants to: look up order #42 (routine, harmless)
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): ALLOWED -- tool executes.
    reason: tool "get_order" is allowed
    rule:   tools.allow

--- Attempt 2 ---
Agent wants to: issue a $50,000 refund
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): DENY -- tool never runs.
    reason: approval request timed out; defaulting to deny
    rule:   approval.timeout

--- Attempt 3 ---
Agent wants to: run "rm -rf /data" on the host shell
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): DENY -- tool never runs.
    reason: tool "run_shell" is explicitly denied
    rule:   tools.deny

--- Attempt 4 ---
Agent wants to: delete the customers database table
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): DENY -- tool never runs.
    reason: tool "delete_database" is explicitly denied
    rule:   tools.deny

--- Attempt 5 ---
Agent wants to: issue a legitimate $150 refund for order #42
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): ALLOWED -- tool executes.
    reason: tool "issue_refund" is allowed
    rule:   tools.allow

--- Attempt 6 ---
Agent wants to: email the customer their refund confirmation
  BEFORE (no guardrail): this would just happen.
  AFTER  (with agent-guardrail): ALLOWED -- tool executes.
    reason: tool "send_email" is allowed
    rule:   tools.allow

=== Summary: 3 call(s) allowed, 3 call(s) blocked by agent-guardrail ===
```

Note attempt 2: the policy sets `on_violation: require_approval` for
over-limit refunds, not a hard `deny`. The demo configures a short
approval timeout and no human is present to approve it, so it correctly
**fails closed** — denied by default rather than left hanging or, worse,
silently allowed.

## MCP proxy

For real MCP-based agents, [`internal/mcpproxy`](internal/mcpproxy) sits in
front of a real MCP server, mirrors its tools, and runs every `tools/call`
through the guard before forwarding it — zero agent-side changes required
beyond pointing the agent's MCP client at the proxy. It also supports
output validation (e.g. redacting secrets in a tool's result) before the
result reaches the agent. See the package's tests for usage.

## Development

```bash
make build   # go build ./...
make test    # go test -race ./...
make vet     # go vet ./...
make lint    # fmt + vet
```

CI (`.github/workflows/ci.yml`) runs build, vet, and `go test -race ./...`
on every push and pull request.
