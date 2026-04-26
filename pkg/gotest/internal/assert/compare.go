package assert

import (
	"cmp"
	"fmt"
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
