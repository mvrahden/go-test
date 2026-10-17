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

		w.It("registers a binary the system terminated as a failure", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/timeout", 1)
			// Windows reports the termination status itself: a suite the
			// --timeout interrupted exits 0xC000013A, which is no verdict and
			// towers over every real exit code, so the run exited with it.
			c.RecordResult("example.com/timeout", 0, gotestrunner.SuiteResult{ExitCode: 0xC000013A})
			gotest.True(it, c.AnyFailed())
			gotest.Equal(it, 1, c.WorstExitCode(),
				"a termination status must read as a failure, not become the run's exit code")
			gotest.Contains(it, stdout.String()+stderr.String(), "terminated by the system (status 0xC000013A)",
				"the status the binary never chose belongs in the output, named")
		})

		w.It("registers a binary the race detector failed as a failure", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/race", 1)
			// -race reports a race it found after the last test — in fixture
			// teardown, after m.Run — by exiting 66, outside the run's codes.
			c.RecordResult("example.com/race", 0, gotestrunner.SuiteResult{ExitCode: 66})
			gotest.True(it, c.AnyFailed())
			gotest.Equal(it, 1, c.WorstExitCode(), "66 must not become the run's exit code")
			gotest.Contains(it, stdout.String()+stderr.String(), "race detector")
		})

		w.It("books a failed compile as a failed package", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			gotestrunner.ExportBookBuildFailures(c, nil, []gotestrunner.BuildFailure{
				{Package: "example.com/broken", Err: fmt.Errorf("compile example.com/broken: exit status 1")},
			})
			gotest.True(it, c.AnyFailed())
			gotest.Equal(it, 2, c.WorstExitCode(),
				"a compile failure is exit 2")
			gotest.Contains(it, stdout.String()+stderr.String(), "compile example.com/broken",
				"the failed package must be visible in the stream, not only on raw stderr")
		})
	})

	t.When("a suite is given up before it starts", func(w *gotest.T) {
		const reason = "TestFooSuite never ran: shared fixture DBSharedFixture did not come up"

		w.It("is a failed suite in the stream, with the reason as its output", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			c.Register("example.com/pkg", 1)
			c.RecordGivenUp("example.com/pkg", 0, "TestFooSuite", reason)

			gotest.Equal(it, 1, c.WorstExitCode())
			events, err := gotestspec.ParseEvents(bytes.NewReader(c.CapturedJSON()))
			gotest.NoError(it, err)
			tree := gotestspec.BuildTree(events)
			gotest.Len(it, tree, 1)
			gotest.Equal(it, gotestspec.StatusFail, tree[0].Status)
			gotest.Len(it, tree[0].Nodes, 1)
			gotest.Equal(it, "TestFooSuite", tree[0].Nodes[0].Name)
			gotest.Equal(it, gotestspec.StatusFail, tree[0].Nodes[0].Status)
			gotest.Contains(it, strings.Join(tree[0].Nodes[0].Output, ""), reason)
		})

		w.It("is a failed package with the reason on stderr in a text run", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.Register("example.com/pkg", 1)
			c.RecordGivenUp("example.com/pkg", 0, "TestFooSuite", reason)

			gotest.Contains(it, stderr.String(), reason)
			gotest.Contains(it, stdout.String(), "FAIL\texample.com/pkg")
		})
	})

	t.When("a shared teardown failure lands after the stream ended", func(w *gotest.T) {
		teardownErr := fmt.Errorf("shared fixture teardown failed")

		w.It("reaches the exit code and every artifact rendered from the captured stream", func(it *gotest.T) {
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
			var result gotestrunner.PipelineResult
			gotestrunner.ExportApplyTeardownFailure(c, &result, teardownErr)
			result.CapturedJSON = c.CapturedJSON()

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

		w.It("reaches the live -json stream the same way", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			var result gotestrunner.PipelineResult
			gotestrunner.ExportApplyTeardownFailure(c, &result, teardownErr)

			gotest.Equal(it, 1, result.ExitCode)
			// A -json consumer reads the run from stdout alone; the booking
			// must not depend on which mode snapshots the stream.
			events, err := gotestspec.ParseEvents(bytes.NewReader(stdout.Bytes()))
			gotest.NoError(it, err)
			gotest.True(it, gotestspec.HasFailures(gotestspec.BuildTree(events)),
				"the live stream must carry the failure the exit code reports")
		})

		w.It("stays on stderr in text mode", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			var result gotestrunner.PipelineResult
			gotestrunner.ExportApplyTeardownFailure(c, &result, teardownErr)

			gotest.Equal(it, 1, result.ExitCode)
			gotest.Empty(it, stdout.String(), "text mode has no stream to book into")
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

	t.When("packages without suites are booked", func(w *gotest.T) {
		want := []streamLine{
			{Action: "start", Package: "example.com/empty"},
			{Action: "output", Package: "example.com/empty", Output: "?   \texample.com/empty\t[no test files]\n"},
			{Action: "skip", Package: "example.com/empty"},
			{Action: "start", Package: "example.com/legacy"},
			{Action: "output", Package: "example.com/legacy", Output: "?   \texample.com/legacy\t[no suites]\n"},
			{Action: "skip", Package: "example.com/legacy"},
		}

		w.It("gives each go test's skip verdict in the live stream", func(it *gotest.T) {
			var stdout, stderr bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &stderr))
			c.StdlibTestsByPkg = map[string]int{"example.com/legacy": 2}
			c.RecordNoSuite("example.com/empty", false)
			c.RecordNoSuite("example.com/legacy", false)
			gotest.Equal(it, want, streamLines(it, stdout.String()))
			gotest.Empty(it, stderr.String())
		})
		w.It("books the same verdicts into a captured run", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.StdlibTestsByPkg = map[string]int{"example.com/legacy": 2}
			c.RecordNoSuite("example.com/empty", false)
			c.RecordNoSuite("example.com/legacy", false)
			gotest.Equal(it, want, streamLines(it, string(c.CapturedJSON())))
			gotest.Empty(it, stdout.String())
		})
	})

	t.When("a package built but none of its suites run", func(w *gotest.T) {
		w.It("passes in the stream with go test's no-tests-to-run note", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.RecordNothingToRun("example.com/filtered")
			gotest.Equal(it, []streamLine{
				{Action: "start", Package: "example.com/filtered"},
				{Action: "output", Package: "example.com/filtered", Output: "ok  \texample.com/filtered\t0.000s [no tests to run]\n"},
				{Action: "pass", Package: "example.com/filtered"},
			}, streamLines(it, stdout.String()))
		})
		w.It("prints its ok line in flush order in a text run", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.SetFlushOrder([]string{"example.com/a", "example.com/filtered"})
			c.Register("example.com/a", 1)
			c.RecordNothingToRun("example.com/filtered")
			gotest.Empty(it, stdout.String(), "waits for example.com/a")
			c.RecordResult("example.com/a", 0, pass(10*time.Millisecond))
			c.Finalize()
			gotest.Equal(it, "ok  \texample.com/a\t0.010s\nok  \texample.com/filtered\t0.000s [no tests to run]\n", stdout.String())
			gotest.False(it, c.AnyFailed())
		})
	})

	t.When("a package's suites are all excluded (X_)", func(w *gotest.T) {
		w.It("opens the package before its skipped suites and passes it with no tests to run", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{"example.com/off": {"X_OffTestSuite"}})
			c.RecordNoSuite("example.com/off", true)
			gotest.Equal(it, []streamLine{
				{Action: "start", Package: "example.com/off"},
				{Action: "run", Package: "example.com/off", Test: "TestX_OffTestSuite"},
				{Action: "output", Package: "example.com/off", Test: "TestX_OffTestSuite", Output: "--- SKIP: TestX_OffTestSuite (0.00s)\n"},
				{Action: "output", Package: "example.com/off", Test: "TestX_OffTestSuite", Output: "    test suite was excluded by user\n"},
				{Action: "skip", Package: "example.com/off", Test: "TestX_OffTestSuite"},
				{Action: "output", Package: "example.com/off", Output: "ok  \texample.com/off\t0.000s [no tests to run]\n"},
				{Action: "pass", Package: "example.com/off"},
			}, streamLines(it, stdout.String()))
		})
		w.It("prints go test's line for a package with test files that runs none, not [no test files]", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.RecordNoSuite("example.com/off", true)
			c.Finalize()
			gotest.Equal(it, "ok  \texample.com/off\t0.000s [no tests to run]\n", stdout.String())
		})
	})

	t.When("a bench run's package runs no benchmark", func(w *gotest.T) {
		w.It("passes it as go test -bench does, without a note", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			gotestrunner.ExportBenchCollector(c)
			c.RecordNothingToRun("example.com/tests")
			c.Finalize()
			gotest.Equal(it, "ok  \texample.com/tests\t0.000s\n", stdout.String())
		})
	})

	t.When("the suite binaries ran no test", func(w *gotest.T) {
		ranNone := func(d time.Duration) gotestrunner.SuiteResult {
			return gotestrunner.SuiteResult{Stdout: []byte("PASS\n"), Stderr: []byte("testing: warning: no tests to run\n"), Duration: d}
		}
		w.It("notes the package as go test does, with its elapsed time", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, ranNone(2*time.Millisecond))
			c.RecordResult("example.com/a", 1, ranNone(3*time.Millisecond))
			c.Finalize()
			gotest.Equal(it, "ok  \texample.com/a\t0.005s [no tests to run]\n", stdout.String())
		})
		w.It("leaves the note off when one of them ran a test", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, ranNone(2*time.Millisecond))
			c.RecordResult("example.com/a", 1, pass(3*time.Millisecond))
			c.Finalize()
			gotest.Equal(it, "ok  \texample.com/a\t0.005s\n", stdout.String())
		})
		w.It("notes it in the stream's summary line", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{
				Stdout:   []byte(`{"Action":"output","Package":"example.com/a","Output":"testing: warning: no tests to run\n"}` + "\n"),
				Duration: 4 * time.Millisecond,
			})
			gotest.Contains(it, stdout.String(), `"Output":"ok  \texample.com/a\t0.004s [no tests to run]\n"`)
		})
	})

	t.When("packages reach the stream", func(w *gotest.T) {
		suite := func(test string) gotestrunner.SuiteResult {
			return gotestrunner.SuiteResult{Stdout: []byte(strings.Join([]string{
				`{"Time":"2024-01-01T00:00:00Z","Action":"start","Package":"example.com/a"}`,
				`{"Time":"2024-01-01T00:00:01Z","Action":"run","Package":"example.com/a","Test":"` + test + `"}`,
				`{"Time":"2024-01-01T00:00:02Z","Action":"pass","Package":"example.com/a","Test":"` + test + `"}`,
				`{"Time":"2024-01-01T00:00:02Z","Action":"pass","Package":"example.com/a"}`,
			}, "\n") + "\n")}
		}
		w.It("opens each package once, before its first event, however many suite processes report", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, suite("TestATestSuite"))
			c.RecordResult("example.com/a", 1, suite("TestBTestSuite"))
			lines := streamLines(it, stdout.String())
			starts := 0
			for _, l := range lines {
				if l.Action == "start" {
					starts++
				}
			}
			gotest.Equal(it, 1, starts)
			gotest.Equal(it, streamLine{Action: "start", Package: "example.com/a"}, lines[0])
		})
		w.It("stamps the start with the time of the package's first event", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 1)
			c.RecordResult("example.com/a", 0, suite("TestATestSuite"))
			first, _, _ := strings.Cut(stdout.String(), "\n")
			gotest.Equal(it, `{"Action":"start","Package":"example.com/a","Time":"2024-01-01T00:00:01Z"}`, first)
		})
	})

	t.When("a text run prints its verdicts", func(w *gotest.T) {
		w.It("interleaves the packages without suites in package order", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.SetFlushOrder([]string{"example.com/a", "example.com/empty", "example.com/b"})
			c.Register("example.com/a", 1)
			c.Register("example.com/b", 1)
			c.RecordNoSuite("example.com/empty", false)
			gotest.Empty(it, stdout.String(), "waits for example.com/a")
			c.RecordResult("example.com/b", 0, pass(20*time.Millisecond))
			c.RecordResult("example.com/a", 0, pass(10*time.Millisecond))
			gotest.Equal(it, "ok  \texample.com/a\t0.010s\n?   \texample.com/empty\t[no test files]\nok  \texample.com/b\t0.020s\n", stdout.String(),
				"streams each verdict as soon as the ones before it are out")
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
		w.It("opens the package, then emits run, output, output, skip events per suite", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.EmitSkippedSuites(map[string][]string{
				"example.com/pkg": {"FooSuite"},
			})

			lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
			gotest.Len(it, lines, 5)

			var start, ev0, ev1, ev2, ev3 map[string]any
			gotest.NoError(it, json.Unmarshal([]byte(lines[0]), &start))
			gotest.NoError(it, json.Unmarshal([]byte(lines[1]), &ev0))
			gotest.NoError(it, json.Unmarshal([]byte(lines[2]), &ev1))
			gotest.NoError(it, json.Unmarshal([]byte(lines[3]), &ev2))
			gotest.NoError(it, json.Unmarshal([]byte(lines[4]), &ev3))

			gotest.Equal(it, "start", start["Action"])
			gotest.Equal(it, "example.com/pkg", start["Package"])

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
			gotest.Len(it, lines, 10)

			var first, sixth map[string]any
			_ = json.Unmarshal([]byte(lines[0]), &first)
			_ = json.Unmarshal([]byte(lines[5]), &sixth)
			gotest.Equal(it, "example.com/a", first["Package"])
			gotest.Equal(it, "example.com/z", sixth["Package"])
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
				Name:   "strips the status that fixture teardown wrote after",
				input:  "--- PASS: TestFoo (0.00s)\nPASS\nreleasing the pool\n",
				expect: "--- PASS: TestFoo (0.00s)\nreleasing the pool\n",
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
			c.Finalize()

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
			c.Finalize()

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

			c.RecordNoSuite("example.com/c", false)
			c.Finalize()

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
			c.Finalize()

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
			c.Finalize()

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

			c.RecordNoSuite("example.com/c", false)
			c.Finalize()

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
			c.Finalize()

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
			c.Finalize()

			gotest.MatchSnapshot(it, normalizeJSON(stdout.String()))
		})
	})
}

// streamLine is the part of a stream event the collector's verdicts fix.
type streamLine struct {
	Action, Package, Test, Output string
}

func streamLines(t *gotest.T, stream string) []streamLine {
	var lines []streamLine
	for line := range strings.Lines(stream) {
		var ev streamLine
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev), line)
		lines = append(lines, ev)
	}
	return lines
}

// A binary stopped from outside ends its own stream mid-way; the note that
// says so must be in the stream for -json consumers, not only on stderr.
func (s *OutputCollectorTestSuite) TestATerminatedBinaryIsNotedInTheStream(t *gotest.T) {
	var stdout, stderr bytes.Buffer
	c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &stderr))
	c.Register("example.com/a", 1)
	c.RecordResult("example.com/a", 0, gotestrunner.SuiteResult{ExitCode: -1, Duration: time.Millisecond})
	t.When("a suite binary reports a signal status", func(w *gotest.T) {
		w.It("writes the note as an output event of its package", func(it *gotest.T) {
			gotest.Regexp(it, `"Action":"output".*"Output":"gotest: suite binary terminated by signal\\n".*"Package":"example.com/a"`, stdout.String())
			gotest.Contains(it, stderr.String(), "gotest: suite binary terminated")
		})
	})
}

// Suites of one package run as concurrent processes; the package's time is
// the clock they occupied together, never the sum of their durations.
func (s *OutputCollectorTestSuite) TestPackageTimeIsTheSpanOfItsSuites(t *gotest.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := gotestrunner.SuiteResult{Stdout: []byte("PASS\n"), Started: t0, Duration: 30 * time.Millisecond}
	second := gotestrunner.SuiteResult{Stdout: []byte("PASS\n"), Started: t0.Add(10 * time.Millisecond), Duration: 10 * time.Millisecond}

	t.When("two suites overlapped", func(w *gotest.T) {
		w.It("prints the span in the text summary", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, first)
			c.RecordResult("example.com/a", 1, second)
			gotest.Equal(it, "ok  \texample.com/a\t0.030s\n", stdout.String())
		})
		w.It("reports the span as the package's Elapsed in the stream", func(it *gotest.T) {
			var stdout bytes.Buffer
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &bytes.Buffer{}))
			c.Register("example.com/a", 2)
			c.RecordResult("example.com/a", 0, first)
			c.RecordResult("example.com/a", 1, second)
			gotest.Contains(it, stdout.String(), `"Action":"pass","Elapsed":0.03,`)
		})
	})
}
