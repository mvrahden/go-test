package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AnnotationsTestSuite covers how failures become GitHub annotations: locating
// the file and line in test output, stripping stdlib frames, and the workflow
// command format.
type AnnotationsTestSuite struct{}

func (s *AnnotationsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type annotationsCtx struct{}

func (s *AnnotationsTestSuite) BeforeEach(t *gotest.T) *annotationsCtx { return &annotationsCtx{} }

type fileLineCase struct {
	Desc    string
	input   string
	file    string
	line    int
	message string
}

func checkFileLine(t *gotest.T, tc fileLineCase) { //nolint:gocritic // hugeParam: test table row
	file, line, msg := gotestspec.ExportParseFileLine(tc.input)
	gotest.Equal(t, tc.file, file)
	gotest.Equal(t, tc.line, line)
	gotest.Equal(t, tc.message, msg)
}

func (s *AnnotationsTestSuite) TestParseFileLine(t *gotest.T, _ *annotationsCtx) {
	for sub, tc := range gotest.Each(t, []fileLineCase{
		{Desc: "standard format", input: "foo_test.go:42: expected 1, got 2", file: "foo_test.go", line: 42, message: "expected 1, got 2"},
		{Desc: "with leading whitespace", input: "    bar_test.go:15: timeout", file: "bar_test.go", line: 15, message: "timeout"},
		{Desc: "no file reference", input: "some random output"},
		{Desc: "no line number", input: "foo.go:abc: something"},
		{Desc: "nested path", input: "sub/dir/baz_test.go:99: failed", file: "sub/dir/baz_test.go", line: 99, message: "failed"},
	}) {
		checkFileLine(sub, tc)
	}
}

func (s *AnnotationsTestSuite) TestParseFileLine_StackTraceFormat(t *gotest.T, _ *annotationsCtx) {
	for sub, tc := range gotest.Each(t, []fileLineCase{
		{Desc: "race detector format with hex offset", input: "      /path/to/foo_test.go:18 +0x7b", file: "foo_test.go", line: 18},
		{Desc: "panic stack with tab indent", input: "\t/path/to/bar_test.go:12 +0x45", file: "bar_test.go", line: 12},
		{Desc: "stack frame without offset", input: "      baz_test.go:42", file: "baz_test.go", line: 42},
		{Desc: "stdlib path detected", input: "\t/usr/local/go/src/runtime/panic.go:1181 +0x18", file: "/usr/local/go/src/runtime/panic.go", line: 1181},
		{Desc: "standard format still works", input: "foo_test.go:42: expected 1, got 2", file: "foo_test.go", line: 42, message: "expected 1, got 2"},
	}) {
		checkFileLine(sub, tc)
	}
}

func (s *AnnotationsTestSuite) TestParseFirstLocation_PrefersUserCode(t *gotest.T, _ *annotationsCtx) {
	lines := []string{
		"fatal error: concurrent map writes",
		"",
		"goroutine 8 [running]:",
		"internal/runtime/maps.fatal({0x589a33?, 0x0?})",
		"\t/usr/local/go/src/runtime/panic.go:1181 +0x18",
		"example.TestConcurrentMapWrite.func1()",
		"\t/home/user/project/foo_test.go:12 +0x45",
	}

	file, line, _ := gotestspec.ExportParseFirstLocation(lines)
	gotest.Equal(t, "foo_test.go", file, "should skip stdlib")
	gotest.Equal(t, 12, line)
}

func (s *AnnotationsTestSuite) TestParseFirstLocation_FallsBackToStdlib(t *gotest.T, _ *annotationsCtx) {
	file, line, _ := gotestspec.ExportParseFirstLocation([]string{"\t/usr/local/go/src/runtime/panic.go:1181 +0x18"})
	gotest.NotEmpty(t, file, "should fall back to stdlib path when no user code found")
	gotest.Equal(t, 1181, line)
}

func (s *AnnotationsTestSuite) TestPackageDir(t *gotest.T, _ *annotationsCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc       string
		pkgPath    string
		modulePath string
		want       string
	}{
		{Desc: "strips module prefix", pkgPath: "github.com/user/repo/internal/foo", modulePath: "github.com/user/repo", want: "internal/foo"},
		{Desc: "root package", pkgPath: "github.com/user/repo", modulePath: "github.com/user/repo", want: ""},
		{Desc: "no module path", pkgPath: "github.com/user/repo/pkg", modulePath: "", want: "github.com/user/repo/pkg"},
		{Desc: "different module", pkgPath: "github.com/other/repo/pkg", modulePath: "github.com/user/repo", want: "github.com/other/repo/pkg"},
	}) {
		gotest.Equal(sub, tc.want, gotestspec.ExportPackageDir(tc.pkgPath, tc.modulePath))
	}
}

func (s *AnnotationsTestSuite) TestCollectAnnotations(t *gotest.T, _ *annotationsCtx) {
	packages := []*gotestspec.Package{{
		Path: "github.com/user/repo/pkg/foo",
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "Good", Status: gotestspec.StatusPass},
			{
				Kind:     gotestspec.KindTest,
				Display:  "Bad",
				Status:   gotestspec.StatusFail,
				Duration: 12 * time.Millisecond,
				Output:   []string{"    foo_test.go:42: expected 1, got 2\n"},
			},
		},
	}}

	annotations := gotestspec.CollectAnnotations(packages, "github.com/user/repo")

	gotest.Len(t, annotations, 1)
	gotest.Equal(t, "pkg/foo/foo_test.go", annotations[0].File)
	gotest.Equal(t, 42, annotations[0].Line)
	gotest.Equal(t, "Bad", annotations[0].Title)
}

func (s *AnnotationsTestSuite) TestCollectAnnotations_NoFileReference(t *gotest.T, _ *annotationsCtx) {
	packages := []*gotestspec.Package{{
		Path: "p",
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindTest,
			Display:  "Broken",
			Status:   gotestspec.StatusFail,
			Duration: time.Millisecond,
			Output:   []string{"panic: runtime error\n"},
		}},
	}}

	gotest.Empty(t, gotestspec.CollectAnnotations(packages, ""), "no annotation for output without file:line")
}

func (s *AnnotationsTestSuite) TestCollectAnnotations_FiltersLogNoiseBeforeAssertion(t *gotest.T, _ *annotationsCtx) {
	packages := []*gotestspec.Package{{
		Path: "github.com/user/repo/pkg/foo",
		Nodes: []*gotestspec.Node{{
			Kind:     gotestspec.KindTest,
			Display:  "Slow",
			Status:   gotestspec.StatusFail,
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

	annotations := gotestspec.CollectAnnotations(packages, "github.com/user/repo")
	gotest.Len(t, annotations, 1)

	msg := annotations[0].Message
	gotest.NotContains(t, msg, "INFO", "log noise must not reach the annotation")
	gotest.Contains(t, msg, "    last failure:", "relative indentation preserved")
	gotest.Contains(t, msg, "        expected: true", "nested indentation preserved")
}

func (s *AnnotationsTestSuite) TestCollectAnnotations_PackageDiagnostic(t *gotest.T, _ *annotationsCtx) {
	packages := []*gotestspec.Package{{
		Path:   "github.com/user/repo/pkg/foo",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{
			{Kind: gotestspec.KindTest, Display: "TestFoo", Status: gotestspec.StatusPass},
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

	annotations := gotestspec.CollectAnnotations(packages, "github.com/user/repo")

	gotest.Len(t, annotations, 1)
	gotest.Equal(t, "pkg/foo/foo_test.go", annotations[0].File)
	gotest.Equal(t, 12, annotations[0].Line)
}

func (s *AnnotationsTestSuite) TestWriteGitHubAnnotations(t *gotest.T, _ *annotationsCtx) {
	annotations := []gotestspec.Annotation{
		{File: "pkg/foo/foo_test.go", Line: 42, Title: "TestFoo / validates input", Message: "expected 1, got 2"},
		{File: "pkg/bar/bar_test.go", Line: 0, Title: "TestBar", Message: "timeout"},
	}

	var buf bytes.Buffer
	gotestspec.WriteGitHubAnnotations(&buf, annotations)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	gotest.Len(t, lines, 2)
	gotest.Contains(t, lines[0], "::error file=pkg/foo/foo_test.go,line=42")
	gotest.Contains(t, lines[0], "title=TestFoo / validates input")
	gotest.Contains(t, lines[0], "::expected 1, got 2")
	gotest.NotContains(t, lines[1], "line=", "no line when line=0")
}

func (s *AnnotationsTestSuite) TestWriteGitHubAnnotations_TruncatesLongMessage(t *gotest.T, _ *annotationsCtx) {
	annotations := []gotestspec.Annotation{{File: "test.go", Line: 1, Title: "T", Message: strings.Repeat("x", 2000)}}

	var buf bytes.Buffer
	gotestspec.WriteGitHubAnnotations(&buf, annotations)
	out := buf.String()

	gotest.LessOrEqual(t, len(out), 1200, "annotation should truncate long messages")
	gotest.Contains(t, out, "...", "truncated message should end with ...")
}

func (s *AnnotationsTestSuite) TestWriteGitHubAnnotations_IncludesColumn(t *gotest.T, _ *annotationsCtx) {
	annotations := []gotestspec.Annotation{
		{File: "internal/foo/suite_test.go", Line: 42, Col: 7, Title: "fail-guard", Message: "assertion result ignored"},
		{File: "internal/bar/suite_test.go", Line: 9, Title: "testify", Message: "testify import"},
	}

	var buf bytes.Buffer
	gotestspec.WriteGitHubAnnotations(&buf, annotations)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	gotest.Len(t, lines, 2)
	gotest.Contains(t, lines[0], "::error file=internal/foo/suite_test.go,line=42,col=7,title=fail-guard::")
	gotest.NotContains(t, lines[1], "col=", "no col when col=0")
}

func (s *AnnotationsTestSuite) TestStripStdlibFrames(t *gotest.T, _ *annotationsCtx) {
	t.When("a simple two-goroutine race", func(w *gotest.T) {
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

		got := gotestspec.ExportStripStdlibFrames(input)

		w.It("drops stdlib function names, file refs and _testmain.go", func(it *gotest.T) {
			gotest.NotContains(it, got, "testing.tRunner")
			gotest.NotContains(it, got, "testing.go:")
			gotest.NotContains(it, got, "_testmain.go")
		})

		w.It("keeps user frames, the diagnostic summary and goroutine headers", func(it *gotest.T) {
			gotest.Contains(it, got, "racetest.TestRaceAfterReturn()")
			gotest.Contains(it, got, "race_test.go:14")
			gotest.Contains(it, got, "Found 1 data race(s)")
			gotest.Contains(it, got, "Goroutine 9 (running) created at:")
		})
	})

	t.When("a multi-goroutine race with http and runtime frames", func(w *gotest.T) {
		// Realistic race between an HTTP handler goroutine and a background
		// writer: 4 goroutine sections, deep stdlib tails from net/http,
		// runtime, and testing, starting from the first user-code reference.
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

		got := gotestspec.ExportStripStdlibFrames(input)

		w.It("keeps every user frame", func(it *gotest.T) {
			for _, want := range []string{
				"handler.(*API).HandleUpdate.func1()", "api.go:89",
				"cache.(*Store).Get()", "store.go:32",
				"handler.(*API).HandleRead()", "handler.(*API).HandleUpdate()",
			} {
				gotest.Contains(it, got, want)
			}
		})

		w.It("keeps the goroutine headers", func(it *gotest.T) {
			gotest.Contains(it, got, "Previous read at 0x00c0001a4000 by goroutine 14:")
			gotest.Contains(it, got, "Goroutine 12 (running) created at:")
			gotest.Contains(it, got, "Goroutine 14 (running) created at:")
		})

		w.It("strips testing, runtime and net/http frames", func(it *gotest.T) {
			for _, noise := range []string{
				"testing.tRunner", "testing.(*T).Run.gowrap1", "runtime.mapaccess1_faststr",
				"net/http.HandlerFunc.ServeHTTP", "net/http/httptest", "net/http.(*conn).serve", "net/http.(*Server).Serve",
			} {
				gotest.NotContains(it, got, noise)
			}
		})

		w.It("fits the 1024-char annotation budget after removing more than half", func(it *gotest.T) {
			gotest.GreaterOrEqual(it, len(input), 1024, "precondition: raw input exceeds the annotation limit")
			gotest.LessOrEqual(it, len(got), 1024)
			gotest.LessOrEqual(it, len(got), len(input)/2)
		})
	})
}

func (s *AnnotationsTestSuite) TestIsStdlibFile(t *gotest.T, _ *annotationsCtx) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		want bool
	}{
		{Desc: "/usr/local/go/src/runtime/panic.go", want: true},
		{Desc: "/usr/local/go/src/testing/testing.go", want: true},
		{Desc: "/usr/local/go/src/internal/race/race.go", want: true},
		{Desc: "/usr/lib/go-1.22/src/runtime/proc.go", want: true},
		{Desc: "/opt/homebrew/opt/go/libexec/src/sync/mutex.go", want: true},
		{Desc: "/usr/local/Cellar/go/1.22.0/libexec/src/testing/testing.go", want: true},
		{Desc: "/snap/go/3584/src/runtime/panic.go", want: true},
		{Desc: "/home/user/sdk/go1.22.0/src/runtime/panic.go", want: true},
		{Desc: "C:/Program Files/Go/src/runtime/panic.go", want: true},
		{Desc: "C:/Program Files/Go/src/testing/testing.go", want: true},
		{Desc: "/home/user/project/foo_test.go", want: false},
		{Desc: "/home/user/go/src/github.com/user/repo/file.go", want: false},
		{Desc: "foo_test.go", want: false},
		{Desc: "sub/dir/bar_test.go", want: false},
		{Desc: "/tmp/gotest/race_test.go", want: false},
	}) {
		gotest.Equal(sub, tc.want, gotestspec.ExportIsStdlibFile(tc.Desc))
	}
}
