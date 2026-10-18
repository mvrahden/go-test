package assert

import (
	"fmt"
	"github.com/mvrahden/go-test/internal/unidiff"
	"reflect"
	"sort"
	"strings"
)

// expandThreshold is the single-line rendering length above which composite
// values (structs, maps, slices) switch to an expanded multiline form, so that
// equality failures can show a line-based diff.
const expandThreshold = 60

// FormatValueExpanded is FormatValue, except that large composite values are
// rendered one field/entry per line. Values whose formatting is already
// multiline (e.g. a GoStringer emitting newlines) pass through unchanged.
func FormatValueExpanded(v any) string {
	s := FormatValue(v)
	if len(s) <= expandThreshold || strings.ContainsRune(s, '\n') {
		return s
	}
	if ml, ok := formatMultiline(reflect.ValueOf(v)); ok {
		return ml
	}
	return s
}

// expandMaxEntries bounds expansion (and therefore the LCS diff input): beyond
// this, values stay single-line — a diff over thousands of lines is unreadable
// and the LCS matrix would be quadratic in it.
const expandMaxEntries = 64

// formatMultiline renders structs, maps, and slices/arrays with one
// field/entry per line. Map entries are sorted by formatted key for
// deterministic output. Returns ok=false for non-composite kinds and for
// values with more than expandMaxEntries entries.
func formatMultiline(rv reflect.Value) (string, bool) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		if rv.Len() > expandMaxEntries {
			return "", false
		}
	}

	var sb strings.Builder
	switch rv.Kind() {
	case reflect.Struct:
		sb.WriteString(rv.Type().String() + "{\n")
		for i := 0; i < rv.NumField(); i++ {
			if !rv.Type().Field(i).IsExported() {
				continue
			}
			fmt.Fprintf(&sb, "  %s: %#v,\n", rv.Type().Field(i).Name, rv.Field(i).Interface())
		}
		sb.WriteString("}")
		return sb.String(), true
	case reflect.Map:
		entries := make([]string, 0, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			entries = append(entries, fmt.Sprintf("  %#v: %#v,", iter.Key().Interface(), iter.Value().Interface()))
		}
		sort.Strings(entries)
		sb.WriteString(rv.Type().String() + "{\n")
		for _, e := range entries {
			sb.WriteString(e + "\n")
		}
		sb.WriteString("}")
		return sb.String(), true
	case reflect.Slice, reflect.Array:
		sb.WriteString(rv.Type().String() + "{\n")
		for i := 0; i < rv.Len(); i++ {
			fmt.Fprintf(&sb, "  %#v,\n", rv.Index(i).Interface())
		}
		sb.WriteString("}")
		return sb.String(), true
	default:
		return "", false
	}
}

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

// Diff renders the unified diff from expected to actual, hunks of changes
// with three lines of context, or "" when they are equal or both single-line
// (the message already shows both). The diff is linear in the inputs' size.
func Diff(expected, actual string) string {
	if expected == actual {
		return ""
	}
	if !strings.Contains(expected, "\n") && !strings.Contains(actual, "\n") {
		return ""
	}
	// Values are compared line by line; whether the last line ends in a
	// newline is not a difference worth a hunk.
	return unidiff.Diff("expected", "actual", []byte(withFinalNewline(expected)), []byte(withFinalNewline(actual)))
}

func withFinalNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
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
	if format, ok := msgAndArgs[0].(string); ok {
		return fmt.Sprintf(format, msgAndArgs[1:]...)
	}
	// No format to place them by: every argument, in order.
	return strings.TrimSuffix(fmt.Sprintln(msgAndArgs...), "\n")
}
