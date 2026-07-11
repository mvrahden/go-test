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

func TestParseFileLine_StackTraceFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		file    string
		line    int
		message string
	}{
		{
			name:  "race detector format with hex offset",
			input: "      /path/to/foo_test.go:18 +0x7b",
			file:  "foo_test.go",
			line:  18,
		},
		{
			name:  "panic stack with tab indent",
			input: "\t/path/to/bar_test.go:12 +0x45",
			file:  "bar_test.go",
			line:  12,
		},
		{
			name:  "stack frame without offset",
			input: "      baz_test.go:42",
			file:  "baz_test.go",
			line:  42,
		},
		{
			name:  "stdlib path detected",
			input: "\t/usr/local/go/src/runtime/panic.go:1181 +0x18",
			file:  "/usr/local/go/src/runtime/panic.go",
			line:  1181,
		},
		{
			name:    "standard format still works",
			input:   "foo_test.go:42: expected 1, got 2",
			file:    "foo_test.go",
			line:    42,
			message: "expected 1, got 2",
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

func TestParseFirstLocation_PrefersUserCode(t *testing.T) {
	lines := []string{
		"fatal error: concurrent map writes",
		"",
		"goroutine 8 [running]:",
		"internal/runtime/maps.fatal({0x589a33?, 0x0?})",
		"\t/usr/local/go/src/runtime/panic.go:1181 +0x18",
		"example.TestConcurrentMapWrite.func1()",
		"\t/home/user/project/foo_test.go:12 +0x45",
	}

	file, line, _ := parseFirstLocation(lines)
	if file != "foo_test.go" {
		t.Errorf("file = %q, want foo_test.go (should skip stdlib)", file)
	}
	if line != 12 {
		t.Errorf("line = %d, want 12", line)
	}
}

func TestParseFirstLocation_FallsBackToStdlib(t *testing.T) {
	lines := []string{
		"\t/usr/local/go/src/runtime/panic.go:1181 +0x18",
	}

	file, line, _ := parseFirstLocation(lines)
	if file == "" {
		t.Error("should fall back to stdlib path when no user code found")
	}
	if line != 1181 {
		t.Errorf("line = %d, want 1181", line)
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

func TestCollectAnnotations_FiltersLogNoiseBeforeAssertion(t *testing.T) {
	packages := []*Package{{
		Path: "github.com/user/repo/pkg/foo",
		Nodes: []*Node{{
			Kind:     KindTest,
			Display:  "Slow",
			Status:   StatusFail,
			Duration: 10 * time.Second,
			Output: []string{
				"    2026/06/25 18:10:03 INFO staging params.json bytes=2\n",
				"    2026/06/25 18:10:03 INFO file stored successfully\n",
				"    helpers.go:37: foo_test.go:74: Eventually failed after 10s:\n",
				"        last failure:\n",
				"        helpers.go:47: True failed:\n",
				"            expected: true\n",
				"            actual:   false\n",
			},
		}},
	}}

	annotations := CollectAnnotations(packages, "github.com/user/repo")
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	a := annotations[0]
	if strings.Contains(a.Message, "INFO") {
		t.Errorf("annotation message should not contain log noise, got:\n%s", a.Message)
	}
	if !strings.Contains(a.Message, "    last failure:") {
		t.Errorf("annotation message should preserve relative indentation, got:\n%s", a.Message)
	}
	if !strings.Contains(a.Message, "        expected: true") {
		t.Errorf("annotation message should preserve nested indentation, got:\n%s", a.Message)
	}
}

func TestCollectAnnotations_PackageDiagnostic(t *testing.T) {
	packages := []*Package{{
		Path:   "github.com/user/repo/pkg/foo",
		Status: StatusFail,
		Nodes: []*Node{
			{Kind: KindTest, Display: "TestFoo", Status: StatusPass},
		},
		Output: []string{
			"==================\n",
			"WARNING: DATA RACE\n",
			"Write at 0x00c by goroutine 9:\n",
			"  pkg.TestFoo.func1()\n",
			"      /home/user/repo/pkg/foo/foo_test.go:12 +0x38\n",
			"==================\n",
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
	if a.Line != 12 {
		t.Errorf("line = %d, want 12", a.Line)
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

func TestStripStdlibFrames(t *testing.T) {
	t.Run("simple two-goroutine race", func(t *testing.T) {
		input := strings.Join([]string{
			"Previous read at 0x00c by goroutine 8:",
			"  racetest.TestRaceAfterReturn()",
			"      /tmp/race_test.go:14 +0xae",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"  testing.(*T).Run.gowrap1()",
			"      /usr/local/go/src/testing/testing.go:2101 +0x38",
			"Goroutine 9 (running) created at:",
			"  racetest.TestRaceAfterReturn()",
			"      /tmp/race_test.go:10 +0xa4",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"Goroutine 8 (finished) created at:",
			"  testing.(*T).Run()",
			"      /usr/local/go/src/testing/testing.go:2101 +0xb12",
			"  main.main()",
			"      _testmain.go:46 +0x164",
			"==================",
			"Found 1 data race(s)",
		}, "\n")

		got := stripStdlibFrames(input)

		if strings.Contains(got, "testing.tRunner") {
			t.Errorf("should strip stdlib function names, got:\n%s", got)
		}
		if strings.Contains(got, "testing.go:") {
			t.Errorf("should strip stdlib file refs, got:\n%s", got)
		}
		if strings.Contains(got, "_testmain.go") {
			t.Errorf("should strip _testmain.go, got:\n%s", got)
		}
		if !strings.Contains(got, "racetest.TestRaceAfterReturn()") {
			t.Errorf("should keep user function names, got:\n%s", got)
		}
		if !strings.Contains(got, "race_test.go:14") {
			t.Errorf("should keep user file refs, got:\n%s", got)
		}
		if !strings.Contains(got, "Found 1 data race(s)") {
			t.Errorf("should keep diagnostic summary, got:\n%s", got)
		}
		if !strings.Contains(got, "Goroutine 9 (running) created at:") {
			t.Errorf("should keep goroutine headers, got:\n%s", got)
		}
	})

	t.Run("multi-goroutine race with http and runtime frames", func(t *testing.T) {
		// Realistic race between an HTTP handler goroutine and a background
		// writer — 4 goroutine sections, deep stdlib tails from net/http,
		// runtime, and testing. This message simulates what gatherMessage
		// produces starting from the first user-code file reference.
		input := strings.Join([]string{
			"  github.com/user/repo/internal/handler.(*API).HandleUpdate.func1()",
			"      /home/user/repo/internal/handler/api.go:89 +0x124",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"  testing.(*T).Run.gowrap1()",
			"      /usr/local/go/src/testing/testing.go:2101 +0x38",
			"",
			"Previous read at 0x00c0001a4000 by goroutine 14:",
			"  runtime.mapaccess1_faststr()",
			"      /usr/local/go/src/runtime/map_faststr.go:13 +0x0",
			"  github.com/user/repo/internal/cache.(*Store).Get()",
			"      /home/user/repo/internal/cache/store.go:32 +0x6c",
			"  github.com/user/repo/internal/handler.(*API).HandleRead()",
			"      /home/user/repo/internal/handler/api.go:56 +0x98",
			"  net/http.HandlerFunc.ServeHTTP()",
			"      /usr/local/go/src/net/http/server.go:2166 +0x44",
			"  net/http/httptest.(*Server).wrapHandler.func1()",
			"      /usr/local/go/src/net/http/httptest/server.go:194 +0xb8",
			"  net/http.serverHandler.ServeHTTP()",
			"      /usr/local/go/src/net/http/server.go:3142 +0x258",
			"  net/http.(*conn).serve()",
			"      /usr/local/go/src/net/http/server.go:2044 +0x11b4",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"",
			"Goroutine 12 (running) created at:",
			"  github.com/user/repo/internal/handler.(*API).HandleUpdate()",
			"      /home/user/repo/internal/handler/api.go:85 +0x110",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"  testing.(*T).Run.gowrap1()",
			"      /usr/local/go/src/testing/testing.go:2101 +0x38",
			"",
			"Goroutine 14 (running) created at:",
			"  net/http.(*Server).Serve()",
			"      /usr/local/go/src/net/http/server.go:3285 +0x584",
			"  net/http/httptest.(*Server).goServe.func1()",
			"      /usr/local/go/src/net/http/httptest/server.go:180 +0xac",
			"  testing.tRunner()",
			"      /usr/local/go/src/testing/testing.go:2036 +0x21c",
			"  testing.(*T).Run.gowrap1()",
			"      /usr/local/go/src/testing/testing.go:2101 +0x38",
			"==================",
			"Found 1 data race(s)",
		}, "\n")

		got := stripStdlibFrames(input)

		// Verify user code is preserved
		if !strings.Contains(got, "handler.(*API).HandleUpdate.func1()") {
			t.Errorf("should keep user function (write site), got:\n%s", got)
		}
		if !strings.Contains(got, "api.go:89") {
			t.Errorf("should keep user file ref (write site), got:\n%s", got)
		}
		if !strings.Contains(got, "cache.(*Store).Get()") {
			t.Errorf("should keep user function (read site), got:\n%s", got)
		}
		if !strings.Contains(got, "store.go:32") {
			t.Errorf("should keep user file ref (read site), got:\n%s", got)
		}
		if !strings.Contains(got, "handler.(*API).HandleRead()") {
			t.Errorf("should keep user function (read caller), got:\n%s", got)
		}
		if !strings.Contains(got, "handler.(*API).HandleUpdate()") {
			t.Errorf("should keep user function (goroutine creation), got:\n%s", got)
		}

		// Verify goroutine headers preserved
		if !strings.Contains(got, "Previous read at 0x00c0001a4000 by goroutine 14:") {
			t.Errorf("should keep goroutine read header, got:\n%s", got)
		}
		if !strings.Contains(got, "Goroutine 12 (running) created at:") {
			t.Errorf("should keep goroutine 12 header, got:\n%s", got)
		}
		if !strings.Contains(got, "Goroutine 14 (running) created at:") {
			t.Errorf("should keep goroutine 14 header, got:\n%s", got)
		}

		// Verify stdlib noise is stripped
		if strings.Contains(got, "testing.tRunner") {
			t.Errorf("should strip testing.tRunner, got:\n%s", got)
		}
		if strings.Contains(got, "testing.(*T).Run.gowrap1") {
			t.Errorf("should strip testing.(*T).Run.gowrap1, got:\n%s", got)
		}
		if strings.Contains(got, "runtime.mapaccess1_faststr") {
			t.Errorf("should strip runtime map access, got:\n%s", got)
		}
		if strings.Contains(got, "net/http.HandlerFunc.ServeHTTP") {
			t.Errorf("should strip net/http frames, got:\n%s", got)
		}
		if strings.Contains(got, "net/http/httptest") {
			t.Errorf("should strip httptest frames, got:\n%s", got)
		}
		if strings.Contains(got, "net/http.(*conn).serve") {
			t.Errorf("should strip net/http.(*conn).serve, got:\n%s", got)
		}
		if strings.Contains(got, "net/http.(*Server).Serve") {
			t.Errorf("should strip net/http.(*Server).Serve, got:\n%s", got)
		}

		// Verify significant size reduction: raw input exceeds 1024 annotation
		// limit, stripped output fits comfortably within it.
		if len(input) < 1024 {
			t.Errorf("test precondition: raw input should exceed annotation limit (got %d)", len(input))
		}
		if len(got) > 1024 {
			t.Errorf("stripped message should fit within annotation budget (got %d chars):\n%s", len(got), got)
		}
		if len(got) > len(input)/2 {
			t.Errorf("stripping should remove >50%% of content (input=%d, output=%d)", len(input), len(got))
		}
	})
}

func TestIsStdlibFile(t *testing.T) {
	tests := []struct {
		file string
		want bool
	}{
		{"/usr/local/go/src/runtime/panic.go", true},
		{"/usr/local/go/src/testing/testing.go", true},
		{"/usr/local/go/src/internal/race/race.go", true},
		{"/usr/lib/go-1.22/src/runtime/proc.go", true},
		{"/opt/homebrew/opt/go/libexec/src/sync/mutex.go", true},
		{"/usr/local/Cellar/go/1.22.0/libexec/src/testing/testing.go", true},
		{"/snap/go/3584/src/runtime/panic.go", true},
		{"/home/user/sdk/go1.22.0/src/runtime/panic.go", true},
		{"C:/Program Files/Go/src/runtime/panic.go", true},
		{"C:/Program Files/Go/src/testing/testing.go", true},
		{"/home/user/project/foo_test.go", false},
		{"/home/user/go/src/github.com/user/repo/file.go", false},
		{"foo_test.go", false},
		{"sub/dir/bar_test.go", false},
		{"/tmp/gotest/race_test.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := isStdlibFile(tt.file)
			if got != tt.want {
				t.Errorf("isStdlibFile(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
