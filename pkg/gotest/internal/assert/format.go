package assert

import (
	"fmt"
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

// diff renders a minimal unified diff between two multiline strings.
// Returns "" if values are identical or if both strings are single-line.
// Uses - for removed lines (expected), + for added lines (actual),
// space for common lines.
func Diff(expected, actual string) string {
	if expected == actual {
		return ""
	}
	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")
	if len(expectedLines) == 1 && len(actualLines) == 1 {
		return ""
	}

	lcs := longestCommonSubsequence(expectedLines, actualLines)

	var sb strings.Builder
	i, j, k := 0, 0, 0
	for k < len(lcs) {
		for i < len(expectedLines) && expectedLines[i] != lcs[k] {
			sb.WriteString("- ")
			sb.WriteString(expectedLines[i])
			sb.WriteByte('\n')
			i++
		}
		for j < len(actualLines) && actualLines[j] != lcs[k] {
			sb.WriteString("+ ")
			sb.WriteString(actualLines[j])
			sb.WriteByte('\n')
			j++
		}
		sb.WriteString("  ")
		sb.WriteString(lcs[k])
		sb.WriteByte('\n')
		i++
		j++
		k++
	}
	// remaining lines after LCS
	for i < len(expectedLines) {
		sb.WriteString("- ")
		sb.WriteString(expectedLines[i])
		sb.WriteByte('\n')
		i++
	}
	for j < len(actualLines) {
		sb.WriteString("+ ")
		sb.WriteString(actualLines[j])
		sb.WriteByte('\n')
		j++
	}
	return sb.String()
}

// longestCommonSubsequence computes the LCS of two string slices.
func longestCommonSubsequence(a, b []string) []string {
	m, n := len(a), len(b)
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			switch {
			case a[i-1] == b[j-1]:
				dp[i][j] = dp[i-1][j-1] + 1
			case dp[i-1][j] >= dp[i][j-1]:
				dp[i][j] = dp[i-1][j]
			default:
				dp[i][j] = dp[i][j-1]
			}
		}
	}
	lcs := make([]string, 0, dp[m][n])
	i, j := m, n
	for i > 0 && j > 0 {
		switch {
		case a[i-1] == b[j-1]:
			lcs = append(lcs, a[i-1])
			i--
			j--
		case dp[i-1][j] >= dp[i][j-1]:
			i--
		default:
			j--
		}
	}
	// reverse
	for l, r := 0, len(lcs)-1; l < r; l, r = l+1, r-1 {
		lcs[l], lcs[r] = lcs[r], lcs[l]
	}
	return lcs
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
	return fmt.Sprintf("%v", msgAndArgs[0])
}
