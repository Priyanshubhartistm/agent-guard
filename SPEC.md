# Build Prompt — AI Agent Guardrail & Policy Engine (Go)

> Copy everything below the line into your AI coding assistant. It is written as
> instructions *to the AI*. Read the "How to use this" note first.

---

## How to use this prompt

- **If you use an agentic tool (e.g. Claude Code):** paste this whole document,
  then say: *"Set up the repo and build Phase 1 only. Show me the code and tests,
  then wait for me to say continue."* Go phase by phase.
- **If you use a chat model:** paste the **Context** + **one Phase** per message.
  Don't ask it to build everything at once — quality drops badly.
- After each phase: run the code, run the tests, confirm it works, *then* continue.

---

## Context (paste this once, at the start)

You are a senior Go engineer. We are building a real, production-quality open-source
project **from scratch**, incrementally. Follow the spec exactly, write idiomatic Go,
write tests as you go, and **build only the phase I ask for** — do not jump ahead.

### What we're building

An **AI Agent Guardrail & Policy Engine**: an independent safety layer that sits
between an AI agent and the "tools" (functions) it can call, and enforces
human-written rules on every tool call. Think **"sudo + firewall for AI agents."**

**The problem it solves:** In agent systems, an LLM decides *which* tool to call and
with *what* arguments. LLMs are unpredictable — they hallucinate, get manipulated by
prompt injection, or make reasoning errors — and most frameworks let the LLM be both
the decision-maker *and* the executor, with no independent check. One bad decision can
delete data, spend real money, or take a destructive action. Our engine is the
independent check: every tool call is intercepted and evaluated against declarative
policies **before** it executes.

**The core flow:**

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

### Design principles

- **Two usable forms:** a Go **library** (the engine core, importable) *and* a
  standalone **proxy/server** that wraps it. Build the library first; the server wraps it.
- **Declarative policies:** rules live in a YAML config, not in code. A non-programmer
  should be able to read and edit them.
- **Fail-closed by default:** if a tool isn't explicitly allowed, deny it.
- **Deterministic & fast:** the engine is on the hot path of every tool call.
  No network calls in the core decision path; keep it pure and testable.
- **Concurrency-safe:** many agents/sessions may run at once.
- **Single static binary** for the server, with a small CLI.

### Tech stack

- Go (latest stable). Standard library first; add dependencies only when they earn it.
- Suggested libraries (verify latest versions before using):
  - Logging: `log/slog` (stdlib, structured JSON)
  - Config: `gopkg.in/yaml.v3`
  - CLI: `github.com/spf13/cobra` (or stdlib `flag` if you prefer minimal)
  - Rate limiting: `golang.org/x/time/rate`
  - HTTP server: stdlib `net/http` (or `github.com/go-chi/chi/v5`)
  - Tests: stdlib `testing` (+ `github.com/stretchr/testify` optional)
  - (Advanced phase) MCP: the official or a popular Go MCP SDK — verify the current
    package name before adding it.

### Core types (define these early, in a `policy` package)

```go
// A single tool call an agent wants to make.
type ToolCall struct {
    AgentID   string            // which agent/policy set applies
    SessionID string            // groups calls for rate/spend/sequence state
    Tool      string            // e.g. "issue_refund"
    Args      map[string]any    // e.g. {"amount": 5000, "customer": 123}
    Timestamp time.Time
}

type DecisionType int
const (
    Allow DecisionType = iota
    Deny
    RequireApproval
)

type Decision struct {
    Type   DecisionType
    Reason string        // human-readable, always set (esp. for Deny/RequireApproval)
    Rule   string        // which rule fired, for auditing
}

// The engine's one job:
type Engine interface {
    Check(ctx context.Context, call ToolCall) (Decision, error)
}
```

### Policy config format (this is the contract — implement it faithfully)

```yaml
agent: "refund-bot"
policies:
  # 1. Which tools may be called at all (fail-closed: anything not in allow is denied)
  tools:
    allow: ["get_order", "issue_refund", "send_email"]
    deny:  ["delete_database", "run_shell"]      # explicit deny wins over allow

  # 2. Argument constraints on specific tools
  constraints:
    - tool: "issue_refund"
      arg:  "amount"
      max:  500                 # also support: min, equals, one_of, regex
      on_violation: require_approval   # or: deny

  # 3. Rate + spend limits (tracked per session)
  limits:
    rate:
      max_calls_per_minute: 30
    spend:
      currency: "USD"
      # which tool+arg counts as "spending money"
      meter: { tool: "issue_refund", arg: "amount" }
      max_per_session: 2000

  # 4. Sequence rules (a finite state machine over tool calls)
  sequence:
    initial: "start"
    transitions:
      - { from: "start",        tool: "get_order",    to: "order_loaded" }
      - { from: "order_loaded", tool: "issue_refund", to: "refunded" }
      - { from: "refunded",     tool: "send_email",   to: "done" }
    # a tool call whose (from-state, tool) has no valid transition is denied
```

---

## Build phases

### Phase 0 — Scaffolding
- Initialize the module (`go mod init github.com/<you>/agent-guardrail`).
- Create a clean layout: `cmd/guardrail/` (CLI/server entry), `internal/policy/`
  (engine core), `internal/config/` (YAML loading), `internal/audit/`, `examples/`.
- Add a `Makefile` (build, test, lint), a basic GitHub Actions CI running `go test ./...`,
  and a README stub. Define the core types above. No logic yet — just structure that compiles.

### Phase 1 — Engine core + tool allow/deny
- Implement `Engine.Check` for the `tools` section only: fail-closed allowlist,
  explicit denylist wins. Return `Decision` with a clear reason and the rule name.
- Load policy from YAML into typed structs (`internal/config`).
- Unit tests: allowed tool → Allow; denied tool → Deny; unlisted tool → Deny.
  Table-driven tests. This is the skeleton the rest hangs on.

### Phase 2 — Argument constraints + rate + spend limits
- Argument constraints: `max`/`min`/`equals`/`one_of`/`regex`, with `on_violation`
  (`deny` or `require_approval`). Handle type coercion for JSON numbers carefully.
- A concurrency-safe **per-session state tracker** (start in-memory, `sync.Mutex`/`RWMutex`;
  design the interface so it can later be backed by Redis).
- Rate limit per session (use `golang.org/x/time/rate`).
- Spend limit: sum the metered arg across the session; deny when the cap is exceeded.
- Tests for each, including concurrent access (run with `-race`).

### Phase 3 — Sequence enforcement (FSM)
- Build a finite state machine from the `sequence` config.
- Track each session's current state; a tool call is a transition attempt.
- If no valid transition exists from the current state for that tool → Deny with a
  reason ("cannot issue_refund before get_order").
- Tests: valid ordering passes; skipping a step is denied; out-of-order is denied.

### Phase 4 — Audit logging
- `internal/audit`: structured (JSON-lines via `slog`) record of every decision —
  timestamp, agent, session, tool, args (with a redaction hook for secrets),
  decision, reason, rule.
- Make the sink pluggable (io.Writer / interface) so it can go to stdout, a file, later a DB.
- Tests: a denied call and an allowed call both produce correct audit records.

### Phase 5 — Human-in-the-loop approval
- When a decision is `RequireApproval`, park the call in a **pending-approvals store**
  and block until a human approves/rejects (with a timeout → default deny).
- Expose approve/reject: start simple with an HTTP endpoint
  (`GET /pending`, `POST /pending/{id}/approve`, `POST /pending/{id}/reject`).
- Tests: approve → the call proceeds; reject/timeout → the call is denied.

### Phase 6 — Server + CLI + library API
- Wrap the engine as a service. Provide an **interception endpoint**: an agent (or
  proxy) POSTs a `ToolCall`; the server returns a `Decision`. (HTTP JSON first; gRPC optional.)
- Also expose a clean **library API** so Go agents can embed the engine directly:
  `guard.Check(ctx, call)` before executing a tool.
- CLI (`cmd/guardrail`): `serve` (run the server), `validate <policy.yaml>`
  (lint a policy file), `tail-audit` (pretty-print the audit log).

### Phase 7 (Advanced) — MCP proxy + output validation
- Build an **MCP proxy**: sit in front of a real MCP server, intercept `tools/call`
  requests, run them through the engine, and only forward allowed calls. This makes the
  guardrail work with real MCP-based agents with zero agent-side changes.
- Add **output validation**: inspect a tool's *result* before returning it to the agent
  (e.g. redact secrets, enforce a schema). Verify the current Go MCP SDK API first.

### Phase 8 — Demo + docs
- Ship an `examples/` folder with a deliberately reckless sample agent (one that would,
  e.g., try `run_shell "rm -rf ..."`, refund $50,000, or delete data) and a policy that
  catches each bad action.
- README with the flow diagram, a quickstart, the full config reference, and a
  before/after showing the guardrail blocking the dangerous calls.

---

## Ongoing rules for every phase
- Idiomatic Go: small interfaces, explicit errors, no premature abstraction.
- `gofmt` + `go vet` clean; run tests with `-race`.
- Every new capability ships with table-driven tests in the same PR.
- Keep the decision core pure (no I/O) so it stays fast and trivially testable.
- Update the README's config reference whenever you add a policy field.

## Stretch goals (only after Phase 8)
- Redis-backed state for multi-instance deployments.
- Prometheus metrics (allowed/denied/approval counts, decision latency).
- A `dry-run` mode that logs what *would* be blocked without enforcing.
- Policy hot-reload on file change.
- A tiny web dashboard for pending approvals and the audit trail.