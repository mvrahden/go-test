package gotestrunner_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// OutputCollectorTestSuite covers the output collector: per-package flushing in registration
// order, package-level event filtering, skipped-suite notes, formatting, the
// golden output.
// Sequential: TestOutputFormatting swaps os.Stdout to capture WritePackageSummary.
type OutputCollectorTestSuite struct{}

func (s *OutputCollectorTestSuite) TestOutputCollector(t *gotest.T) {
	pass := func(d time.Duration) gotestrunner.SuiteResult {
		return gotestrunner.SuiteResult{Stdout: []byte("PASS\n"), ExitCode: 0, Duration: d}
	}

	t.When("flushing in registration order", func(w *gotest.T) {
		w.It("buffers later packages until earlier ones complete", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.Register("example.com/c", 1)

			c.RecordResult("example.com/c", 0, pass(30*time.Millisecond))
			gotest.Empty(it, stdout.String(), "c should be buffered because a and b are not done")

			c.RecordResult("example.com/a", 0, pass(10*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/a\t0.010s\n", stdout.String(), "a should flush as the head")

			stdout.Reset()
			c.RecordResult("example.com/b", 0, pass(20*time.Millisecond))
			want := "ok  \texample.com/b\t0.020s\n" +
				"ok  \texample.com/c\t0.030s\n"
			gotest.Equal(it, want, stdout.String(), "b and c should flush together")
		})

		w.It("flushes immediately when packages complete in order", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/x", 1)
			c.Register("example.com/y", 1)

			c.RecordResult("example.com/x", 0, pass(10*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/x\t0.010s\n", stdout.String())

			stdout.Reset()
			c.RecordResult("example.com/y", 0, pass(20*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/y\t0.020s\n", stdout.String())
		})
	})

	t.When("tracking failures", func(w *gotest.T) {
		w.It("reports no failure when all pass", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{ExitCode: 0})
			gotest.False(it, c.AnyFailed())
		})

		w.It("reports failure when any suite fails", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 0})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{ExitCode: 1})
			gotest.True(it, c.AnyFailed())
		})

		w.It("registers a signal-killed binary as a failure", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/oom", 1)
			// A signal-terminated process reports -1. The worst-code comparison
			// never registered it, so an OOM-killed suite printed FAIL and the
			// run still exited 0.
			c.RecordResult("example.com/oom", 0, gotestrunner.SuiteResult{ExitCode: -1})
			gotest.True(it, c.AnyFailed())
			gotest.Equal(it, 1, c.WorstExitCode(),
				"abnormal termination must raise the exit code, not slip under it")
			gotest.Contains(it, stdout.String()+stderr.String(), "terminated by signal",
				"the binary's own output ends mid-stream, so the verdict must say what happened")
		})

		w.It("books a failed compile as a failed package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			gotestrunner.ExportBookBuildFailures(c, nil, []gotestrunner.BuildFailure{
				{Package: "example.com/broken", Err: fmt.Errorf("compile example.com/broken: exit status 1")},
			})
			gotest.True(it, c.AnyFailed())
			gotest.Equal(it, 2, c.WorstExitCode(),
				"a compile failure is exit 2, matching batch mode")
			gotest.Contains(it, stdout.String()+stderr.String(), "compile example.com/broken",
				"the failed package must be visible in the stream, not only on raw stderr")
		})
	})

	t.When("a shared teardown failure lands after the stream ended", func(w *gotest.T) {
		w.It("reaches the exit code and every artifact rendered from the stream", func(it *gotest.T) {
			result := gotestrunner.PipelineResult{CapturedJSON: []byte{}}
			gotestrunner.ExportApplyTeardownFailure(&result,
				fmt.Errorf("shared fixture teardown failed; see AfterAll errors above"))

			gotest.Equal(it, 1, result.ExitCode)

			// The captured stream is the single source spec/summary/markdown
			// artifacts derive from; an exit-code-only failure rendered "all
			// passed" beside exit 1 in a CI comment.
			events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
			gotest.NoError(it, err)
			tree := gotestspec.BuildTree(events)
			gotest.True(it, gotestspec.HasFailures(tree),
				"the failure must be derivable from the stream itself")

			var buf bytes.Buffer
			gotestspec.RenderSummary(&buf, tree, gotestspec.WithNoColor())
			gotest.Contains(it, buf.String(), "shared fixture teardown failed",
				"a rendered artifact must carry the teardown failure")
		})

		w.It("tracks worst exit code", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 1})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{ExitCode: 2})
			gotest.Equal(it, 2, c.WorstExitCode())
		})
	})

	t.When("finalize", func(w *gotest.T) {
		w.It("is a no-op for captured JSON mode", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: 1})
			stdout.Reset()
			c.Finalize([]string{"example.com/empty"})
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("JSON capture mode", func(w *gotest.T) {
		w.It("deduplicates package events across suites", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/pkg", 2)

			suite1JSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/pkg","Test":"TestA"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/pkg","Test":"TestA","Elapsed":0.01}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/pkg","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/pkg","Elapsed":0.01}`,
			}, "\n") + "\n"
			suite2JSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/pkg","Test":"TestB"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/pkg","Test":"TestB","Elapsed":0.02}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/pkg","Output":"FAIL\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/pkg","Elapsed":0.02}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/pkg", 0, gotestrunner.SuiteResult{
				Stdout: []byte(suite1JSON), ExitCode: 0, Duration: 10 * time.Millisecond,
			})
			c.RecordResult("example.com/pkg", 1, gotestrunner.SuiteResult{
				Stdout: []byte(suite2JSON), ExitCode: 1, Duration: 20 * time.Millisecond,
			})

			captured := c.CapturedJSON()
			lines := strings.Split(strings.TrimRight(string(captured), "\n"), "\n")

			// Count package-level pass/fail events (Test=="")
			var pkgVerdicts []string
			for _, line := range lines {
				var ev map[string]any
				if json.Unmarshal([]byte(line), &ev) != nil {
					continue
				}
				if ev["Test"] == nil && (ev["Action"] == "pass" || ev["Action"] == "fail") {
					pkgVerdicts = append(pkgVerdicts, ev["Action"].(string))
				}
			}

			gotest.Len(it, pkgVerdicts, 1, "should have exactly one package-level verdict, got: %v", pkgVerdicts)
			gotest.Equal(it, "fail", pkgVerdicts[0], "package should be marked as fail when any suite fails")
		})
	})
}

func (s *OutputCollectorTestSuite) TestFilterPackageLevelEvents(t *gotest.T) {
	t.When("a suite produces diagnostic package output (e.g. a race report)", func(w *gotest.T) {
		w.It("keeps test-level events and diagnostic output, drops summary lines and the package verdict", func(it *gotest.T) {
			input := strings.Join([]string{
				`{"Action":"run","Package":"p","Test":"TestFoo"}`,
				`{"Action":"output","Package":"p","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}`,
				`{"Action":"output","Package":"p","Test":"TestFoo","Output":"--- PASS: TestFoo (0.00s)\n"}`,
				`{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0}`,
				`{"Action":"output","Package":"p","Output":"PASS\n"}`,
				`{"Action":"output","Package":"p","Output":"==================\n"}`,
				`{"Action":"output","Package":"p","Output":"WARNING: DATA RACE\n"}`,
				`{"Action":"output","Package":"p","Output":"Write at 0x00c by goroutine 9:\n"}`,
				`{"Action":"output","Package":"p","Output":"  pkg.TestFoo.func1()\n"}`,
				`{"Action":"output","Package":"p","Output":"      /path/to/foo_test.go:12 +0x38\n"}`,
				`{"Action":"output","Package":"p","Output":"==================\n"}`,
				`{"Action":"output","Package":"p","Output":"Found 1 data race(s)\n"}`,
				`{"Action":"output","Package":"p","Output":"FAIL\tp\t1.012s\n"}`,
				`{"Action":"fail","Package":"p","Elapsed":1.012}`,
			}, "\n")

			var buf bytes.Buffer
			gotestrunner.ExportFilterPackageLevelEvents(&buf, []byte(input))
			out := buf.String()

			// Test-level events must survive.
			gotest.Contains(it, out, `"Test":"TestFoo"`)

			// Diagnostic output must survive.
			gotest.Contains(it, out, "WARNING: DATA RACE")
			gotest.Contains(it, out, "Found 1 data race(s)")
			gotest.Contains(it, out, "foo_test.go:12")

			// Summary lines must be dropped.
			gotest.NotContains(it, out, `"Output":"PASS\n"`)
			gotest.NotContains(it, out, `"Output":"FAIL\tp\t`)

			// Package-level structural events must be dropped.
			gotest.NotContains(it, out, `"Action":"fail","Package":"p","Elapsed"`)
		})
	})

	t.When("a suite completes normally with no diagnostic output", func(w *gotest.T) {
		w.It("drops package-level structural events and the ok summary line", func(it *gotest.T) {
			input := strings.Join([]string{
				`{"Action":"start","Package":"p"}`,
				`{"Action":"run","Package":"p","Test":"TestFoo"}`,
				`{"Action":"output","Package":"p","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}`,
				`{"Action":"pass","Package":"p","Test":"TestFoo","Elapsed":0.01}`,
				`{"Action":"output","Package":"p","Output":"ok  \tp\t0.01s\n"}`,
				`{"Action":"pass","Package":"p","Elapsed":0.01}`,
			}, "\n")

			var buf bytes.Buffer
			gotestrunner.ExportFilterPackageLevelEvents(&buf, []byte(input))
			out := buf.String()

			// Test-level events survive.
			gotest.Contains(it, out, `"Test":"TestFoo"`)

			// Package-level structural events are dropped.
			gotest.NotContains(it, out, `"Action":"start"`)
			gotest.NotContains(it, out, `"ok  \\tp\\t`)
		})
	})
}

func (s *OutputCollectorTestSuite) TestIsPackageSummaryLine(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Name  string
		input string
		want  bool
	}{
		{"bare PASS", "PASS\n", true},
		{"bare FAIL", "FAIL\n", true},
		{"ok package summary", "ok  \tpkg\t0.5s\n", true},
		{"FAIL package summary", "FAIL\tpkg\t1.2s\n", true},
		{"no test files summary", "?   \tpkg\t[no test files]\n", true},
		{"no suites summary", "?   \tpkg\t[no suites]\n", true},
		{"race separator", "==================\n", false},
		{"race warning", "WARNING: DATA RACE\n", false},
		{"race detector count", "Found 1 data race(s)\n", false},
		{"panic message", "panic: boom\n", false},
		{"goroutine trace header", "goroutine 1 [running]:\n", false},
	}) {
		got := gotestrunner.ExportIsPackageSummaryLine(tc.input)
		gotest.Equal(sub, tc.want, got)
	}
}

func (s *OutputCollectorTestSuite) TestEmitSkippedSuites(t *gotest.T) {
	t.When("text mode", func(w *gotest.T) {
		w.It("produces no output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/a": {"SkippedSuite"},
			})
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("JSON streaming mode", func(w *gotest.T) {
		w.It("emits run, output, output, skip events per suite", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/pkg": {"FooSuite"},
			})

			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			gotest.Len(it, lines, 4)

			var ev0, ev1, ev2, ev3 map[string]any
			gotest.NoError(it, json.Unmarshal([]byte(lines[0]), &ev0))
			gotest.NoError(it, json.Unmarshal([]byte(lines[1]), &ev1))
			gotest.NoError(it, json.Unmarshal([]byte(lines[2]), &ev2))
			gotest.NoError(it, json.Unmarshal([]byte(lines[3]), &ev3))

			gotest.Equal(it, "run", ev0["Action"])
			gotest.Equal(it, "TestFooSuite", ev0["Test"])
			gotest.Equal(it, "example.com/pkg", ev0["Package"])

			gotest.Equal(it, "output", ev1["Action"])
			gotest.Contains(it, ev1["Output"].(string), "SKIP")

			gotest.Equal(it, "output", ev2["Action"])
			gotest.Contains(it, ev2["Output"].(string), "excluded by user")

			gotest.Equal(it, "skip", ev3["Action"])
			gotest.Equal(it, "TestFooSuite", ev3["Test"])
		})

		w.It("sorts packages for deterministic output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/z": {"ZSuite"},
				"example.com/a": {"ASuite"},
			})

			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			gotest.Len(it, lines, 8)

			var first, fifth map[string]any
			_ = json.Unmarshal([]byte(lines[0]), &first)
			_ = json.Unmarshal([]byte(lines[4]), &fifth)
			gotest.Equal(it, "example.com/a", first["Package"])
			gotest.Equal(it, "example.com/z", fifth["Package"])
		})
	})

	t.When("empty map", func(w *gotest.T) {
		w.It("produces no output", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{})
			gotest.Empty(it, stdout.String())
		})

		w.It("handles nil map", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(nil)
			gotest.Empty(it, stdout.String())
		})
	})
}

func (s *OutputCollectorTestSuite) TestOutputFormatting(t *gotest.T) {
	t.When("stripping trailing status", func(w *gotest.T) {
		for sub, tc := range gotest.Each(w, []struct {
			Name   string
			input  string
			expect string
		}{
			{
				Name:   "strips trailing PASS",
				input:  "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\nPASS\n",
				expect: "=== RUN   TestFoo\n--- PASS: TestFoo (0.00s)\n",
			},
			{
				Name:   "strips trailing FAIL",
				input:  "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\nFAIL\n",
				expect: "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\n",
			},
			{
				Name:   "strips trailing PASS with extra newlines",
				input:  "line1\nline2\nPASS\n\n\n",
				expect: "line1\nline2\n",
			},
			{
				Name:   "preserves non-status last line",
				input:  "line1\nline2\nsome output\n",
				expect: "line1\nline2\nsome output\n",
			},
			{
				Name:   "only PASS returns nil",
				input:  "PASS\n",
				expect: "",
			},
			{
				Name:   "no newlines returns nil",
				input:  "PASS",
				expect: "",
			},
		}) {
			got := gotestrunner.StripTrailingStatus([]byte(tc.input))
			if tc.expect == "" {
				gotest.Empty(sub, got, "expected nil, got %q", got)
			} else {
				gotest.Equal(sub, tc.expect, string(got))
			}
		}
	})

	t.When("writing package summary", func(w *gotest.T) {
		w.When("verbose mode", func(w2 *gotest.T) {
			for sub, tc := range gotest.Each(w2, []struct {
				Name     string
				pkg      string
				failed   bool
				duration time.Duration
				expect   string
			}{
				{
					Name:     "passing package includes PASS prefix",
					pkg:      "example.com/pkg",
					failed:   false,
					duration: 1234 * time.Millisecond,
					expect:   "PASS\nok  \texample.com/pkg\t1.234s\n",
				},
				{
					Name:     "failing package includes FAIL prefix",
					pkg:      "example.com/pkg",
					failed:   true,
					duration: 567 * time.Millisecond,
					expect:   "FAIL\nFAIL\texample.com/pkg\t0.567s\n",
				},
			}) {
				got := capturePackageSummary(tc.pkg, tc.failed, tc.duration, true)
				gotest.Equal(sub, tc.expect, got)
			}
		})

		w.When("non-verbose mode", func(w2 *gotest.T) {
			for sub, tc := range gotest.Each(w2, []struct {
				Name     string
				pkg      string
				failed   bool
				duration time.Duration
				expect   string
			}{
				{
					Name:     "passing package omits PASS prefix",
					pkg:      "example.com/pkg",
					failed:   false,
					duration: 1234 * time.Millisecond,
					expect:   "ok  \texample.com/pkg\t1.234s\n",
				},
				{
					Name:     "failing package still includes FAIL prefix",
					pkg:      "example.com/pkg",
					failed:   true,
					duration: 567 * time.Millisecond,
					expect:   "FAIL\nFAIL\texample.com/pkg\t0.567s\n",
				},
			}) {
				got := capturePackageSummary(tc.pkg, tc.failed, tc.duration, false)
				gotest.Equal(sub, tc.expect, got)
			}
		})
	})

}

func (s *OutputCollectorTestSuite) TestOutputGolden(t *gotest.T) {
	t.When("text non-verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("single failing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/fail", 1)
			c.RecordResult("example.com/fail", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("--- FAIL: TestBad (0.00s)\n    bad_test.go:5: assertion failed\nFAIL\n"),
				Stderr:   []byte(""),
				ExitCode: 1,
				Duration: 100 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("multi-package mixed with no-test-files", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))

			// Package A: 2 suites — first passes, second fails → package fails
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			})
			c.RecordResult("example.com/a", 1, gotestrunner.SuiteResult{
				Stdout:   []byte("--- FAIL: TestBad (0.00s)\n    bad_test.go:5: nope\nFAIL\n"),
				ExitCode: 1,
				Duration: 200 * time.Millisecond,
			})

			// Package B: 1 suite — passes
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("PASS\n"),
				ExitCode: 0,
				Duration: 20 * time.Millisecond,
			})

			c.Finalize([]string{"example.com/c"})

			gotest.MatchSnapshot(it, stdout.String())
		})
	})

	t.When("text verbose", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/ok", 1)
			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestOK\n--- PASS: TestOK (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("single failing package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/fail", 1)
			c.RecordResult("example.com/fail", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestBad\n--- FAIL: TestBad (0.00s)\n    bad_test.go:5: assertion failed\nFAIL\n"),
				ExitCode: 1,
				Duration: 100 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, stdout.String())
		})

		w.It("multi-package mixed with no-test-files", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, true, gotestrunner.WithWriters(&stdout, &stderr))

			// Package A: 2 suites — first passes, second fails
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestGoodA\n--- PASS: TestGoodA (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 100 * time.Millisecond,
			})
			c.RecordResult("example.com/a", 1, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestBadA\n--- FAIL: TestBadA (0.00s)\n    bad_test.go:5: nope\nFAIL\n"),
				ExitCode: 1,
				Duration: 200 * time.Millisecond,
			})

			// Package B: 1 suite — passes
			c.Register("example.com/b", 1)
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout:   []byte("=== RUN   TestGoodB\n--- PASS: TestGoodB (0.00s)\nPASS\n"),
				ExitCode: 0,
				Duration: 20 * time.Millisecond,
			})

			c.Finalize([]string{"example.com/c"})

			gotest.MatchSnapshot(it, stdout.String())
		})
	})

	t.When("json streaming", func(w *gotest.T) {
		w.It("single passing package", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/ok", 1)

			suiteJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"start","Package":"example.com/ok"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/ok","Test":"TestFoo"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/ok","Test":"TestFoo","Output":"=== RUN   TestFoo\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/ok","Test":"TestFoo","Elapsed":0.001}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/ok","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/ok","Elapsed":0.05}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/ok", 0, gotestrunner.SuiteResult{
				Stdout:   []byte(suiteJSON),
				ExitCode: 0,
				Duration: 50 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, normalizeJSON(stdout.String()))
		})

		w.It("multi-package mixed", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)

			passJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/a","Test":"TestA"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/a","Test":"TestA","Elapsed":0.001}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/a","Output":"PASS\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"pass","Package":"example.com/a","Elapsed":0.01}`,
			}, "\n") + "\n"

			failJSON := strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"example.com/b","Test":"TestB"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/b","Test":"TestB","Elapsed":0.002}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"output","Package":"example.com/b","Output":"FAIL\n"}`,
				`{"Time":"2024-01-01T00:00:00Z","Action":"fail","Package":"example.com/b","Elapsed":0.02}`,
			}, "\n") + "\n"

			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout: []byte(passJSON), ExitCode: 0, Duration: 10 * time.Millisecond,
			})
			c.RecordResult("example.com/b", 0, gotestrunner.SuiteResult{
				Stdout: []byte(failJSON), ExitCode: 1, Duration: 20 * time.Millisecond,
			})
			c.Finalize(nil)

			gotest.MatchSnapshot(it, normalizeJSON(stdout.String()))
		})
	})
}
