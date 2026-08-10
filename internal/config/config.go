// Package config defines the typed structures for a policy YAML file and
// (in a later phase) the loader that parses it.
package config

// Policy is the root of a policy YAML file.
type Policy struct {
	Agent    string   `yaml:"agent"`
	Policies Policies `yaml:"policies"`
}

// Policies groups every rule section that applies to an agent.
type Policies struct {
	Tools       Tools        `yaml:"tools"`
	Constraints []Constraint `yaml:"constraints"`
	Limits      Limits       `yaml:"limits"`
	Sequence    Sequence     `yaml:"sequence"`
}

// Tools is the allow/deny list of tool names a policy governs.
// Fail-closed: anything not in Allow is denied. Deny always wins over Allow.
type Tools struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// Constraint restricts the value of a single argument on a specific tool.
type Constraint struct {
	Tool        string   `yaml:"tool"`
	Arg         string   `yaml:"arg"`
	Max         *float64 `yaml:"max,omitempty"`
	Min         *float64 `yaml:"min,omitempty"`
	Equals      any      `yaml:"equals,omitempty"`
	OneOf       []any    `yaml:"one_of,omitempty"`
	Regex       string   `yaml:"regex,omitempty"`
	OnViolation string   `yaml:"on_violation"` // "deny" or "require_approval"
}

// Limits groups rate and spend limits, tracked per session.
type Limits struct {
	Rate  RateLimit  `yaml:"rate"`
	Spend SpendLimit `yaml:"spend"`
}

// RateLimit caps how many calls a session may make per minute.
type RateLimit struct {
	MaxCallsPerMinute int `yaml:"max_calls_per_minute"`
}

// SpendLimit caps the total metered amount a session may spend.
type SpendLimit struct {
	Currency      string  `yaml:"currency"`
	Meter         Meter   `yaml:"meter"`
	MaxPerSession float64 `yaml:"max_per_session"`
}

// Meter identifies which tool+arg counts as "spending money".
type Meter struct {
	Tool string `yaml:"tool"`
	Arg  string `yaml:"arg"`
}

// Sequence describes a finite state machine over tool calls.
type Sequence struct {
	Initial     string       `yaml:"initial"`
	Transitions []Transition `yaml:"transitions"`
}

// Transition is a single valid (state, tool) -> state edge.
type Transition struct {
	From string `yaml:"from"`
	Tool string `yaml:"tool"`
	To   string `yaml:"to"`
}
