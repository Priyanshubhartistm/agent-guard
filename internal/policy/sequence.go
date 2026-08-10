package policy

import (
	"sort"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

// compiledSequence is a config.Sequence pre-indexed into
// (fromState -> tool -> toState) for O(1) transition lookups. A policy
// with no transitions configured leaves sequence enforcement disabled.
type compiledSequence struct {
	enabled     bool
	initial     string
	transitions map[string]map[string]string
}

func compileSequence(s config.Sequence) compiledSequence {
	cs := compiledSequence{
		enabled:     len(s.Transitions) > 0,
		initial:     s.Initial,
		transitions: make(map[string]map[string]string),
	}
	for _, t := range s.Transitions {
		if cs.transitions[t.From] == nil {
			cs.transitions[t.From] = make(map[string]string)
		}
		cs.transitions[t.From][t.Tool] = t.To
	}
	return cs
}

// next reports the state tool would transition to from current, and
// whether that transition is valid.
func (cs compiledSequence) next(current, tool string) (string, bool) {
	to, ok := cs.transitions[current][tool]
	return to, ok
}

// allowedTools returns, in sorted order, the tools that are valid to call
// from state. Used to build a helpful Deny reason.
func (cs compiledSequence) allowedTools(state string) []string {
	m := cs.transitions[state]
	tools := make([]string, 0, len(m))
	for tool := range m {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}
