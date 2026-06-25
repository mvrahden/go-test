package gotestspec //nolint:stdlib-test

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseFileLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		file    string
		line    int
		message string
	}{
		{
			name:    "standard format",
			input:   "foo_test.go:42: expected 1, got 2",
			file:    "foo_test.go",
			line:    42,
			message: "expected 1, got 2",
		},
		{
			name:    "with leading whitespace",
			input:   "    bar_test.go:15: timeout",
			file:    "bar_test.go",
			line:    15,
			message: "timeout",
		},
		{
			name:  "no file reference",
			input: "some random output",
		},
		{
			name:  "no line number",
			input: "foo.go:abc: something",
		},
		{
			name:    "nested path",
			input:   "sub/dir/baz_test.go:99: failed",
			file:    "sub/dir/baz_test.go",
			line:    99,
			message: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, line, msg := parseFileLine(tt.input)
			if file != tt.file {
				t.Errorf("file = %q, want %q", file, tt.file)
			}
			if line != tt.line {
				t.Errorf("line = %d, want %d", line, tt.line)
			}
			if msg != tt.message {
				t.Errorf("message = %q, want %q", msg, tt.message)
			}
		})
	}
}

func TestPackageDir(t *testing.T) {
	tests := []struct {
		name       string
		pkgPath    string
		modulePath string
		want       string
	}{
		{
			name:       "strips module prefix",
			pkgPath:    "github.com/user/repo/internal/foo",
			modulePath: "github.com/user/repo",
			want:       "internal/foo",
		},
		{
			name:       "root package",
			pkgPath:    "github.com/user/repo",
			modulePath: "github.com/user/repo",
			want:       "",
		},
		{
			name:       "no module path",
			pkgPath:    "github.com/user/repo/pkg",
			modulePath: "",
			want:       "github.com/user/repo/pkg",
		},
		{
			name:       "different module",
			pkgPath:    "github.com/other/repo/pkg",
			modulePath: "github.com/user/repo",
			want:       "github.com/other/repo/pkg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := packageDir(tt.pkgPath, tt.modulePath)
			if got != tt.want {
				t.Errorf("packageDir(%q, %q) = %q, want %q", tt.pkgPath, tt.modulePath, got, tt.want)
			}
		})
	}
}

func TestCollectAnnotations(t *testing.T) {
	packages := []*Package{{
		Path: "github.com/user/repo/pkg/foo",
		Nodes: []*Node{
			{Kind: KindTest, Display: "Good", Status: StatusPass},
			{
				Kind:     KindTest,
				Display:  "Bad",
				Status:   StatusFail,
				Duration: 12 * time.Millisecond,
				Output:   []string{"    foo_test.go:42: expected 1, got 2\n"},
			},
		},
	}}

	annotations := CollectAnnotations(packages, "github.com/user/repo")

	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	a := annotations[0]
	if a.File != "pkg/foo/foo_test.go" {
		t.Errorf("file = %q, want %q", a.File, "pkg/foo/foo_test.go")
	}
	if a.Line != 42 {
		t.Errorf("line = %d, want 42", a.Line)
	}
	if a.Title != "Bad" {
		t.Errorf("title = %q, want %q", a.Title, "Bad")
	}
}

func TestCollectAnnotations_NoFileReference(t *testing.T) {
	packages := []*Package{{
		Path: "p",
		Nodes: []*Node{{
			Kind:     KindTest,
			Display:  "Broken",
			Status:   StatusFail,
			Duration: time.Millisecond,
			Output:   []string{"panic: runtime error\n"},
		}},
	}}

	annotations := CollectAnnotations(packages, "")
	if len(annotations) != 0 {
		t.Errorf("expected 0 annotations for output without file:line, got %d", len(annotations))
	}
}

func TestWriteGitHubAnnotations(t *testing.T) {
	annotations := []Annotation{
		{
			File:    "pkg/foo/foo_test.go",
			Line:    42,
			Title:   "TestFoo / validates input",
			Message: "expected 1, got 2",
		},
		{
			File:    "pkg/bar/bar_test.go",
			Line:    0,
			Title:   "TestBar",
			Message: "timeout",
		},
	}

	var buf bytes.Buffer
	WriteGitHubAnnotations(&buf, annotations)
	out := buf.String()

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 annotation lines, got %d: %s", len(lines), out)
	}

	if !strings.Contains(lines[0], "::error file=pkg/foo/foo_test.go,line=42") {
		t.Errorf("first annotation wrong format: %s", lines[0])
	}
	if !strings.Contains(lines[0], "title=TestFoo / validates input") {
		t.Errorf("first annotation missing title: %s", lines[0])
	}
	if !strings.Contains(lines[0], "::expected 1, got 2") {
		t.Errorf("first annotation missing message: %s", lines[0])
	}

	if strings.Contains(lines[1], "line=") {
		t.Errorf("second annotation should not have line when line=0: %s", lines[1])
	}
}

func TestWriteGitHubAnnotations_TruncatesLongMessage(t *testing.T) {
	long := strings.Repeat("x", 2000)
	annotations := []Annotation{{
		File:    "test.go",
		Line:    1,
		Title:   "T",
		Message: long,
	}}

	var buf bytes.Buffer
	WriteGitHubAnnotations(&buf, annotations)
	out := buf.String()

	if len(out) > 1200 {
		t.Errorf("annotation should truncate long messages, got %d chars", len(out))
	}
	if !strings.Contains(out, "...") {
		t.Error("truncated message should end with ...")
	}
}
