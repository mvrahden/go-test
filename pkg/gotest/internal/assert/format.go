package assert

import (
	"fmt"
	"reflect"
	"strings"
)

// FormatValue formats a Go value for display in error messages.
// nil → "<nil>", non-nil pointer → dereference and format the pointee,
// nil pointer → "(*Type)(nil)", otherwise fmt.Sprintf("%#v", v).
func FormatValue(v any) string {
	if v == nil {
		return "<nil>"
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Sprintf("(*%s)(nil)", rv.Type().Elem().Name())
		}
		return fmt.Sprintf("%#v", rv.Elem().Interface())
	}
	return fmt.Sprintf("%#v", v)
}

// Diff renders the line changes from expected to actual as hunks with three
// lines of context, each line marked ' ', '-' or '+' and the hunks
// separated by a line "…". It is "" when they are equal or both single-line
// (the message already shows both). The diff is linear in the inputs' size.
func Diff(expected, actual string) string {
	if expected == actual {
		return ""
	}
	if !strings.Contains(expected, "\n") && !strings.Contains(actual, "\n") {
		return ""
	}
	var out strings.Builder
	for k, h := range lineHunks(strings.Split(expected, "\n"), strings.Split(actual, "\n")) {
		if k > 0 {
			out.WriteString("…\n")
		}
		for _, l := range h.Lines {
			for _, m := range markLine("", l.Op, chomp(l.Text), l.Text) {
				out.WriteString(m + "\n")
			}
		}
	}
	return out.String()
}

// DiffSection is Diff under a "diff (-expected +actual):" heading, its
// lines indented under it, or "" when Diff is.
func DiffSection(expected, actual string) string {
	d := Diff(expected, actual)
	if d == "" {
		return ""
	}
	return "  diff (-expected +actual):\n" + indentLines(strings.TrimSuffix(d, "\n"), "    ", "    ")
}

// indentLines puts first before s's first line and rest before the others.
func indentLines(s, first, rest string) string {
	return first + strings.ReplaceAll(s, "\n", "\n"+rest)
}

// FormatMessage formats optional user messages for error output.
// Empty → "". Single string → return it.
// First string + rest → fmt.Sprintf(first, rest...).
func FormatMessage(msgAndArgs []any) string {
	if len(msgAndArgs) == 0 {
		return ""
	}
	if len(msgAndArgs) == 1 {
		if s, ok := msgAndArgs[0].(string); ok {
			return s
		}
		return fmt.Sprintf("%v", msgAndArgs[0])
	}
	// Any string kind is a format, named ones included.
	if format := reflect.ValueOf(msgAndArgs[0]); format.Kind() == reflect.String {
		return fmt.Sprintf(format.String(), msgAndArgs[1:]...)
	}
	// No format to place them by: every argument, in order.
	return strings.TrimSuffix(fmt.Sprintln(msgAndArgs...), "\n")
}
