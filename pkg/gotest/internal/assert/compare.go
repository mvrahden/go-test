package assert

import (
	"cmp"
	"fmt"
	"math"
	"reflect"
)

// CheckGreater returns "" if a > b.
// Otherwise it returns a formatted error string.
func CheckGreater[T cmp.Ordered](a, b T) string {
	if cmp.Compare(a, b) > 0 {
		return ""
	}
	return fmt.Sprintf("Greater failed:\n  %s is not greater than %s", FormatValue(a), FormatValue(b))
}

// CheckGreaterOrEqual returns "" if a >= b.
// Otherwise it returns a formatted error string.
func CheckGreaterOrEqual[T cmp.Ordered](a, b T) string {
	if cmp.Compare(a, b) >= 0 {
		return ""
	}
	return fmt.Sprintf("GreaterOrEqual failed:\n  %s is not greater than or equal to %s", FormatValue(a), FormatValue(b))
}

// CheckLess returns "" if a < b.
// Otherwise it returns a formatted error string.
func CheckLess[T cmp.Ordered](a, b T) string {
	if cmp.Compare(a, b) < 0 {
		return ""
	}
	return fmt.Sprintf("Less failed:\n  %s is not less than %s", FormatValue(a), FormatValue(b))
}

// CheckLessOrEqual returns "" if a <= b.
// Otherwise it returns a formatted error string.
func CheckLessOrEqual[T cmp.Ordered](a, b T) string {
	if cmp.Compare(a, b) <= 0 {
		return ""
	}
	return fmt.Sprintf("LessOrEqual failed:\n  %s is not less than or equal to %s", FormatValue(a), FormatValue(b))
}

// Numeric is every type InDelta measures.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// CheckInDelta returns "" if expected and actual differ by at most delta.
// Otherwise it returns a formatted error string. Integers are measured
// exactly, never through float64.
func CheckInDelta[V Numeric](expected, actual V, delta float64) string {
	if math.IsNaN(delta) {
		return "InDelta failed:\n  delta is NaN; no distance is within it"
	}
	var diff any
	within := false
	switch e := reflect.ValueOf(expected); {
	case e.CanInt():
		d := intDistance(e.Int(), reflect.ValueOf(actual).Int())
		diff, within = d, uintWithin(d, delta)
	case e.CanUint():
		d := uintDistance(e.Uint(), reflect.ValueOf(actual).Uint())
		diff, within = d, uintWithin(d, delta)
	default:
		ef, af := e.Float(), reflect.ValueOf(actual).Float()
		if math.IsInf(ef, 0) && ef == af {
			return "" // the same infinity, whose difference has no value
		}
		d := math.Abs(ef - af)
		diff, within = d, d <= delta
	}
	if !within {
		return fmt.Sprintf("InDelta failed:\n  |%v - %v| = %v exceeds delta %v", expected, actual, diff, delta)
	}
	return ""
}

// intDistance is |a - b|, which always fits in a uint64.
func intDistance(a, b int64) uint64 {
	if a < b {
		a, b = b, a
	}
	return uint64(a) - uint64(b) //nolint:gosec // G115: wraps on purpose; the difference of the wrapped values is exact
}

func uintDistance(a, b uint64) uint64 {
	if a < b {
		return b - a
	}
	return a - b
}

// uintWithin reports d <= delta without rounding d to a float64.
func uintWithin(d uint64, delta float64) bool {
	switch {
	case delta < 0:
		return false
	case delta >= 1<<64:
		return true
	}
	return d <= uint64(delta) // truncation: an integer is at most delta when at most its floor
}
