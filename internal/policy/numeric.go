package policy

import (
	"encoding/json"
	"reflect"
)

// toFloat64 coerces a constraint or call-argument value to a float64.
// Args typically arrive as JSON-decoded map[string]any, where numbers
// default to float64 but may be json.Number if the caller used
// json.Decoder.UseNumber(); Go call sites may also pass plain int/int64
// literals directly, so all common numeric kinds are handled explicitly.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// valuesEqual compares two constraint values. Numeric kinds are compared by
// value regardless of underlying Go type (e.g. int(500) == float64(500)),
// strings are compared directly, and anything else falls back to
// reflect.DeepEqual.
func valuesEqual(a, b any) bool {
	if af, aok := toFloat64(a); aok {
		if bf, bok := toFloat64(b); bok {
			return af == bf
		}
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return as == bs
		}
	}
	return reflect.DeepEqual(a, b)
}
