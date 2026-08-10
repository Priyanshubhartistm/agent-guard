package policy

import (
	"fmt"
	"regexp"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/config"
)

// violationAction is what to do when a compiledConstraint is violated.
type violationAction int

const (
	violationDeny violationAction = iota
	violationRequireApproval
)

// compiledConstraint is a config.Constraint that has been validated and
// pre-compiled (e.g. its regex) so Check never fails or pays compilation
// cost on the hot path.
type compiledConstraint struct {
	arg         string
	max         *float64
	min         *float64
	equals      any
	oneOf       []any
	regex       *regexp.Regexp
	onViolation violationAction
}

// compileConstraint validates a config.Constraint and prepares it for fast,
// repeated evaluation. It returns an error for an unknown on_violation
// value or an invalid regex, so a bad policy fails at load time rather than
// on the hot path.
func compileConstraint(c config.Constraint) (compiledConstraint, error) {
	cc := compiledConstraint{
		arg:    c.Arg,
		max:    c.Max,
		min:    c.Min,
		equals: c.Equals,
		oneOf:  c.OneOf,
	}

	switch c.OnViolation {
	case "deny":
		cc.onViolation = violationDeny
	case "require_approval":
		cc.onViolation = violationRequireApproval
	default:
		return compiledConstraint{}, fmt.Errorf("on_violation must be %q or %q, got %q", "deny", "require_approval", c.OnViolation)
	}

	if c.Regex != "" {
		re, err := regexp.Compile(c.Regex)
		if err != nil {
			return compiledConstraint{}, fmt.Errorf("invalid regex %q: %w", c.Regex, err)
		}
		cc.regex = re
	}

	return cc, nil
}

// evaluate reports whether args violates the constraint and, if so, a
// human-readable detail explaining why.
//
// A missing arg is not a violation: there is no value to check against the
// bound, so the constraint simply does not apply to that call.
func (c compiledConstraint) evaluate(args map[string]any) (violated bool, detail string) {
	value, present := args[c.arg]
	if !present {
		return false, ""
	}

	if c.max != nil {
		f, ok := toFloat64(value)
		if !ok {
			return true, fmt.Sprintf("must be numeric to check against max %v, got %v", *c.max, value)
		}
		if f > *c.max {
			return true, fmt.Sprintf("value %v exceeds max %v", value, *c.max)
		}
	}

	if c.min != nil {
		f, ok := toFloat64(value)
		if !ok {
			return true, fmt.Sprintf("must be numeric to check against min %v, got %v", *c.min, value)
		}
		if f < *c.min {
			return true, fmt.Sprintf("value %v is below min %v", value, *c.min)
		}
	}

	if c.equals != nil && !valuesEqual(value, c.equals) {
		return true, fmt.Sprintf("value %v does not equal required value %v", value, c.equals)
	}

	if len(c.oneOf) > 0 {
		match := false
		for _, want := range c.oneOf {
			if valuesEqual(value, want) {
				match = true
				break
			}
		}
		if !match {
			return true, fmt.Sprintf("value %v is not one of %v", value, c.oneOf)
		}
	}

	if c.regex != nil {
		s := fmt.Sprint(value)
		if !c.regex.MatchString(s) {
			return true, fmt.Sprintf("value %q does not match pattern %q", s, c.regex.String())
		}
	}

	return false, ""
}
