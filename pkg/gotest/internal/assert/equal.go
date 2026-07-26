package assert

import (
	"fmt"
	"reflect"
	"strings"
)

// CheckEqual returns "" if expected and actual are deeply equal.
// Otherwise it returns a formatted error string describing the mismatch,
// including a line diff when the rendered values are multiline — large
// composite values are expanded to one field/entry per line for this purpose.
func CheckEqual(expected, actual any) string {
	if reflect.DeepEqual(expected, actual) {
		return ""
	}

	fmtExpected := FormatValueExpanded(expected)
	fmtActual := FormatValueExpanded(actual)
	if fmtExpected == fmtActual {
		// The values differ (DeepEqual said so) but the expanded rendering
		// hides it — e.g. only unexported fields differ, which expansion
		// cannot read. Fall back to %#v, which can.
		fmtExpected = FormatValue(expected)
		fmtActual = FormatValue(actual)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Equal failed:\n")
	fmt.Fprintf(&sb, "  expected: %s\n", fmtExpected)
	fmt.Fprintf(&sb, "  actual:   %s", fmtActual)

	if d := Diff(fmtExpected, fmtActual); d != "" {
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
