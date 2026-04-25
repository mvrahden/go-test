package assert

import (
	"testing"
)

func Test_FormatValue_Nil(t *testing.T) {
	got := FormatValue(nil)
	if got != "<nil>" {
		t.Fatalf("FormatValue(nil) = %q; want %q", got, "<nil>")
	}
}

func Test_FormatValue_NilPointer(t *testing.T) {
	var p *string
	got := FormatValue(p)
	want := "(*string)(nil)"
	if got != want {
		t.Fatalf("FormatValue((*string)(nil)) = %q; want %q", got, want)
	}
}

func Test_FormatValue_NonNilPointer(t *testing.T) {
	s := "hello"
	got := FormatValue(&s)
	want := `"hello"`
	if got != want {
		t.Fatalf("FormatValue(&s) = %q; want %q", got, want)
	}
}

func Test_FormatValue_NonNilPointerInt(t *testing.T) {
	n := 42
	got := FormatValue(&n)
	want := "42"
	if got != want {
		t.Fatalf("FormatValue(&n) = %q; want %q", got, want)
	}
}

func Test_FormatValue_PlainValues(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{v: true, want: "true"},
		{v: false, want: "false"},
		{v: 42, want: "42"},
		{v: 3.14, want: "3.14"},
		{v: "hello", want: `"hello"`},
		{v: []int{1, 2, 3}, want: "[]int{1, 2, 3}"},
		{v: struct{ A int }{A: 1}, want: "struct { A int }{A:1}"},
	}
	for _, tc := range tests {
		got := FormatValue(tc.v)
		if got != tc.want {
			t.Errorf("FormatValue(%v) = %q; want %q", tc.v, got, tc.want)
		}
	}
}

func Test_diff_IdenticalStrings(t *testing.T) {
	got := diff("hello", "hello")
	if got != "" {
		t.Fatalf("diff(identical) = %q; want empty string", got)
	}
}

func Test_diff_BothSingleLine(t *testing.T) {
	got := diff("foo", "bar")
	if got != "" {
		t.Fatalf("diff(both single-line) = %q; want empty string (no diff for single-line)", got)
	}
}

func Test_diff_MultilineIdentical(t *testing.T) {
	s := "line1\nline2\nline3"
	got := diff(s, s)
	if got != "" {
		t.Fatalf("diff(identical multiline) = %q; want empty string", got)
	}
}

func Test_diff_MultilineAddedLine(t *testing.T) {
	expected := "line1\nline2"
	actual := "line1\nline2\nline3"
	got := diff(expected, actual)
	if got == "" {
		t.Fatal("diff(multiline with added line) returned empty; want non-empty diff")
	}
	// "line3" should appear as added
	if !containsSubstring(got, "+ line3") {
		t.Errorf("diff output %q missing '+ line3'", got)
	}
	// common lines should appear with leading space
	if !containsSubstring(got, "  line1") {
		t.Errorf("diff output %q missing '  line1'", got)
	}
	if !containsSubstring(got, "  line2") {
		t.Errorf("diff output %q missing '  line2'", got)
	}
}

func Test_diff_MultilineRemovedLine(t *testing.T) {
	expected := "line1\nline2\nline3"
	actual := "line1\nline3"
	got := diff(expected, actual)
	if got == "" {
		t.Fatal("diff(multiline with removed line) returned empty; want non-empty diff")
	}
	// "line2" should appear as removed
	if !containsSubstring(got, "- line2") {
		t.Errorf("diff output %q missing '- line2'", got)
	}
}

func Test_diff_MultilineChangedLine(t *testing.T) {
	expected := "line1\nlineA\nline3"
	actual := "line1\nlineB\nline3"
	got := diff(expected, actual)
	if got == "" {
		t.Fatal("diff(multiline changed line) returned empty; want non-empty diff")
	}
	if !containsSubstring(got, "- lineA") {
		t.Errorf("diff output %q missing '- lineA'", got)
	}
	if !containsSubstring(got, "+ lineB") {
		t.Errorf("diff output %q missing '+ lineB'", got)
	}
}

func Test_FormatMessage_Empty(t *testing.T) {
	got := FormatMessage(nil)
	if got != "" {
		t.Fatalf("FormatMessage(nil) = %q; want %q", got, "")
	}
	got = FormatMessage([]any{})
	if got != "" {
		t.Fatalf("FormatMessage([]) = %q; want %q", got, "")
	}
}

func Test_FormatMessage_SingleString(t *testing.T) {
	got := FormatMessage([]any{"hello world"})
	want := "hello world"
	if got != want {
		t.Fatalf("FormatMessage([string]) = %q; want %q", got, want)
	}
}

func Test_FormatMessage_SingleNonString(t *testing.T) {
	got := FormatMessage([]any{42})
	want := "42"
	if got != want {
		t.Fatalf("FormatMessage([int]) = %q; want %q", got, want)
	}
}

func Test_FormatMessage_FormatString(t *testing.T) {
	got := FormatMessage([]any{"hello %s, you are %d years old", "Alice", 30})
	want := "hello Alice, you are 30 years old"
	if got != want {
		t.Fatalf("FormatMessage([format, args...]) = %q; want %q", got, want)
	}
}

func Test_FormatMessage_FormatStringNoArgs(t *testing.T) {
	// First element is a string but no extra args — treated as plain string
	got := FormatMessage([]any{"plain message"})
	want := "plain message"
	if got != want {
		t.Fatalf("FormatMessage([plain string]) = %q; want %q", got, want)
	}
}

// containsSubstring reports whether s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
