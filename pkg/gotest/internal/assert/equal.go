package assert

import (
	"fmt"
	"reflect"
	"strings"
)

// CheckEqual returns "" if expected and actual are deeply equal.
// Otherwise it returns a formatted error string describing the mismatch,
// including a diff section when both formatted values are multiline.
func CheckEqual(expected, actual any) string {
	if reflect.DeepEqual(expected, actual) {
		return ""
	}

	fmtExpected := FormatValue(expected)
	fmtActual := FormatValue(actual)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Equal failed:\n")
	fmt.Fprintf(&sb, "  expected: %s\n", fmtExpected)
	fmt.Fprintf(&sb, "  actual:   %s", fmtActual)

	d := diff(fmtExpected, fmtActual)
	if d != "" {
		fmt.Fprintf(&sb, "\n  diff:\n")
		for _, line := range strings.Split(strings.TrimRight(d, "\n"), "\n") {
			fmt.Fprintf(&sb, "    %s\n", line)
		}
	}

	return sb.String()
}

// CheckNotEqual returns "" if expected and actual are NOT deeply equal.
// Otherwise it returns a formatted error string indicating both values are equal.
func CheckNotEqual(expected, actual any) string {
	if !reflect.DeepEqual(expected, actual) {
		return ""
	}

	return fmt.Sprintf("NotEqual failed:\n  both are: %s", FormatValue(actual))
}
