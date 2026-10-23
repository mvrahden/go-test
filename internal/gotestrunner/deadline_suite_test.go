package gotestrunner_test

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// DeadlineAttributionTestSuite covers naming the tests an expired --timeout cut
// short, and booking each as failed so every renderer shows which one hung.
type DeadlineAttributionTestSuite struct{}

func (s *DeadlineAttributionTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const deadlineStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestHang"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestHang/it_waits"}
{"Action":"run","Package":"example.com/other","Test":"TestParseTestSuite"}
{"Time":"2026-09-13T10:00:02Z","Action":"pass","Package":"example.com/other","Test":"TestParseTestSuite","Elapsed":2}
{"Action":"run","Package":"example.com/other","Test":"FuzzParseTestSuite_FuzzRoundTrip"}
{"Action":"run","Package":"example.com/other","Test":"FuzzParseTestSuite_FuzzRoundTrip/seed#0"}
{"Time":"2026-09-13T10:00:20Z","Action":"fail","Package":"example.com/pkg","Elapsed":20}
`

func (s *DeadlineAttributionTestSuite) TestRunningUnitsReadTheStream(t *gotest.T) {
	t.It("names a started method and fuzz wrapper without a verdict, not their behaviors, seeds or busy suites", func(it *gotest.T) {
		gotest.Equal(it, []unit{
			{Pkg: censusPkg, Path: "TestCartTestSuite/TestHang"},
			{Pkg: "example.com/other", Path: "FuzzParseTestSuite_FuzzRoundTrip"},
		}, gotestrunner.ExportRunning(deadlineStream))
	})

	t.It("reads lines split across writes", func(it *gotest.T) {
		whole := gotestrunner.ExportRunning(deadlineStream)
		for _, at := range []int{7, 180, 333} {
			gotest.Equal(it, whole, gotestrunner.ExportRunning(deadlineStream[:at], deadlineStream[at:]))
		}
	})

	t.It("names nothing in a stream that settled every unit", func(it *gotest.T) {
		gotest.Empty(it, gotestrunner.ExportRunning(censusGreenStream))
	})

	t.It("names a benchmark without a result line, never its wrapper, which reports no verdict", func(it *gotest.T) {
		gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkStarted"}}, gotestrunner.ExportRunning(censusBenchStream))
	})

	t.It("names a suite itself when none of its units is running", func(it *gotest.T) {
		gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite"}}, gotestrunner.ExportRunning(`{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
`))
	})

	t.It("names a method that started again after a verdict", func(it *gotest.T) {
		gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite/TestAdd"}}, gotestrunner.ExportRunning(`{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
`))
	})
}

const deadlineBooked = `{"Action":"output","Package":"example.com/pkg","Test":"TestCartTestSuite/TestHang","Output":"    gotest: --timeout expired while this test was running\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestHang"}
{"Action":"output","Package":"example.com/other","Test":"FuzzParseTestSuite_FuzzRoundTrip","Output":"    gotest: --timeout expired while this test was running\n"}
{"Action":"fail","Package":"example.com/other","Test":"FuzzParseTestSuite_FuzzRoundTrip"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Time":"2026-09-13T10:00:02Z","Action":"fail","Package":"example.com/other","Test":"TestParseTestSuite","Elapsed":2}
{"Time":"2026-09-13T10:00:20Z","Action":"fail","Package":"example.com/pkg","Elapsed":20}
{"Action":"fail","Package":"example.com/other"}
`

// cutShortStream is a method the converter failed when the binary ended under
// it: a fail with an elapsed time, but no result line of its own.
const cutShortStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"output","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Output":"=== RUN   TestCartTestSuite/TestAdd\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Elapsed":9.7}
`

func (s *DeadlineAttributionTestSuite) TestAMethodTheConverterFailedIsStillNamed(t *gotest.T) {
	t.When("the stream fails a method without a result line of its own", func(w *gotest.T) {
		w.It("names the method, not its suite, as what the deadline cut short", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite/TestAdd"}}, gotestrunner.ExportRunning(cutShortStream))
		})
		w.It("books the timeout as the method's output and fails its suite, without a second fail for the method", func(it *gotest.T) {
			running, booked := gotestrunner.ExportBookDeadline(gotestrunner.RunStreamJSON, cutShortStream, context.DeadlineExceeded)
			gotest.Len(it, running, 1)
			gotest.Contains(it, booked, `"Test":"TestCartTestSuite/TestAdd","Output":"    gotest: --timeout expired while this test was running`)
			gotest.NotContains(it, booked, `{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}`)
			gotest.Contains(it, booked, `{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite"}`)
		})
		w.It("treats a method with its own result line as settled", func(it *gotest.T) {
			settled := cutShortStream[:len(cutShortStream)-len(`{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Elapsed":9.7}`+"\n")] +
				`{"Action":"output","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Output":"--- FAIL: TestCartTestSuite/TestAdd (9.70s)\n"}` + "\n" +
				`{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Elapsed":9.7}` + "\n"
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite"}}, gotestrunner.ExportRunning(settled))
		})
	})
}

func (s *DeadlineAttributionTestSuite) TestACrashBeforeTheDeadlineIsNotBlamedOnIt(t *gotest.T) {
	target := func(name string) gotestrunner.SuiteTarget {
		return gotestrunner.SuiteTarget{SuiteSpec: gotestrunner.SuiteSpec{Package: censusPkg, SuiteName: name}}
	}
	hang := `{"Action":"run","Package":"example.com/pkg","Test":"TestHangTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestHangTestSuite/TestWait"}
`
	t.When("one suite crashed on its own and another was still running at the deadline", func(w *gotest.T) {
		var stdout, stderr bytes.Buffer
		c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&stdout, &stderr))
		c.Register(censusPkg, 2)
		c.RecordResult(censusPkg, 0, gotestrunner.SuiteResult{Target: target("TestCartTestSuite"), Stdout: []byte(cutShortStream), ExitCode: 2})
		c.RecordResult(censusPkg, 1, gotestrunner.SuiteResult{Target: target("TestHangTestSuite"), Stdout: []byte(hang), ExitCode: 1, CutShort: true})
		before := stdout.Len()
		running := gotestrunner.ExportBookDeadlineOn(c, context.DeadlineExceeded)
		booked := stdout.String()[before:]

		w.It("names only what the deadline cut short", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestHangTestSuite/TestWait"}}, running)
			gotest.NotContains(it, booked, `"Test":"TestCartTestSuite/TestAdd","Output":"    gotest: --timeout expired`)
		})
	})

	t.When("a method's output starts with dashes that are not a result line", func(w *gotest.T) {
		stream := `{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd"}
{"Action":"output","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Output":"--- 5 allocs/op\n"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestCartTestSuite/TestAdd","Elapsed":9.7}
`
		w.It("still names the method the binary ended under", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite/TestAdd"}}, gotestrunner.ExportRunning(stream))
		})
	})
}

// ev is one stream line for a test of the example package.
func ev(action, test string, output ...string) string {
	line := `{"Action":"` + action + `","Package":"example.com/pkg","Test":"` + test + `"`
	if len(output) > 0 {
		line += `,"Output":"` + output[0] + `\n"`
	}
	return line + "}\n"
}

func (s *DeadlineAttributionTestSuite) TestWhatCountsAsAResultLine(t *gotest.T) {
	const add = "TestCartTestSuite/TestAdd"
	failedUnder := func(output string) string {
		return ev("run", "TestCartTestSuite") + ev("run", add) + ev("output", add, output) + ev("fail", add)
	}
	t.When("a method prints its own result line before the stream fails it", func(w *gotest.T) {
		for sub, line := range gotest.Each(w, []string{
			"--- PASS: " + add + " (0.00s)",
			"--- FAIL: " + add + " (9.70s)",
			"--- SKIP: " + add + " (0.00s)",
			"    --- FAIL: " + add + " (9.70s)",
		}) {
			gotest.Equal(sub, []unit{{Pkg: censusPkg, Path: "TestCartTestSuite"}}, gotestrunner.ExportRunning(failedUnder(line)))
		}
	})
	t.When("a method prints a line shaped like another test's result", func(w *gotest.T) {
		for sub, line := range gotest.Each(w, []string{
			"--- PASS: TestCartTestSuite/TestAddMore (0.00s)",
			"--- FAIL: " + add + "/it_adds (0.00s)",
			"--- PASS: " + add,
			"--- PASS: in the log",
		}) {
			gotest.Equal(sub, []unit{{Pkg: censusPkg, Path: add}}, gotestrunner.ExportRunning(failedUnder(line)))
		}
	})
}

func (s *DeadlineAttributionTestSuite) TestARunAgainStartsWithoutAResult(t *gotest.T) {
	const add = "TestCartTestSuite/TestAdd"
	stream := ev("run", "TestCartTestSuite") + ev("run", add) + ev("output", add, "--- PASS: "+add+" (0.00s)") + ev("pass", add) +
		ev("output", "TestCartTestSuite", "--- PASS: TestCartTestSuite (0.00s)") + ev("pass", "TestCartTestSuite") +
		ev("run", "TestCartTestSuite") + ev("run", add) + ev("fail", add)
	t.When("the stream fails a method's second run without a result line of its own", func(w *gotest.T) {
		w.It("names the method, whose first run's result does not settle the second", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: add}}, gotestrunner.ExportRunning(stream))
		})
	})
	t.When("a suite's output has been recorded", func(w *gotest.T) {
		c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
		c.Register(censusPkg, 1)
		c.RecordResult(censusPkg, 0, gotestrunner.SuiteResult{Stdout: []byte(stream[:strings.LastIndex(stream, ev("run", "TestCartTestSuite"))])})
		w.It("keeps none of its result lines", func(it *gotest.T) {
			gotest.Zero(it, gotestrunner.ExportResulted(c))
		})
	})
}

func (s *DeadlineAttributionTestSuite) TestATestWaitingForAParallelSlotIsNotRunning(t *gotest.T) {
	queued := ev("run", "TestParTestSuite") +
		ev("run", "TestParTestSuite/TestA") + ev("pause", "TestParTestSuite/TestA") +
		ev("run", "TestParTestSuite/TestB") + ev("pause", "TestParTestSuite/TestB") +
		ev("run", "TestParTestSuite/TestC") + ev("pause", "TestParTestSuite/TestC") +
		ev("cont", "TestParTestSuite/TestA")
	t.When("parallel methods paused and only one continued", func(w *gotest.T) {
		w.It("names only the method that continued", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestParTestSuite/TestA"}}, gotestrunner.ExportRunning(queued))
		})
		w.It("names only that method when the stream fails every method without a result", func(it *gotest.T) {
			failed := queued + ev("fail", "TestParTestSuite/TestA") + ev("fail", "TestParTestSuite/TestB") + ev("fail", "TestParTestSuite/TestC")
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestParTestSuite/TestA"}}, gotestrunner.ExportRunning(failed))
		})
	})
	t.When("every method paused and none continued", func(w *gotest.T) {
		w.It("names the suite, whose own body is what runs", func(it *gotest.T) {
			gotest.Equal(it, []unit{{Pkg: censusPkg, Path: "TestParTestSuite"}}, gotestrunner.ExportRunning(queued[:strings.LastIndex(queued, `{"Action":"cont"`)]))
		})
	})
}

func (s *DeadlineAttributionTestSuite) TestATextBenchRunNamesTheHungBenchmark(t *gotest.T) {
	const header = "goos: linux\ngoarch: amd64\npkg: example.com/pkg\ncpu: Some CPU\n"
	name := func(stdout string) []unit {
		c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&bytes.Buffer{}, &bytes.Buffer{}))
		c.Register(censusPkg, 1)
		c.RecordResult(censusPkg, 0, gotestrunner.SuiteResult{
			Target: gotestrunner.SuiteTarget{SuiteSpec: gotestrunner.SuiteSpec{Package: censusPkg, SuiteName: "CacheTestSuite"}, Bench: true},
			Stdout: []byte(header + stdout), ExitCode: 1, CutShort: true,
		})
		return gotestrunner.ExportBookDeadlineOn(c, context.DeadlineExceeded)
	}
	const (
		suite  = "BenchmarkCacheTestSuite\n"
		fast   = "BenchmarkCacheTestSuite/BenchmarkFast\n"
		result = "BenchmarkCacheTestSuite/BenchmarkFast-16         \t     100\t         1.600 ns/op\t       0 B/op\t       0 allocs/op\n"
		hang   = "BenchmarkCacheTestSuite/BenchmarkHang\n"
	)
	method := []unit{{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite/BenchmarkHang"}}
	whole := []unit{{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite"}}
	t.When("the deadline ends a benchmark that printed its name and no result", func(w *gotest.T) {
		for sub, stdout := range gotest.Each(w, []string{
			suite + fast + result + hang,
			suite + hang,
			suite + fast + strings.Replace(result, "-16 ", "    ", 1) + hang,
			suite + fast + result + result + hang,
			suite + hang + "BenchmarkCacheTestSuite/BenchmarkHang/one\nBenchmarkCacheTestSuite/BenchmarkHang/one-16 \t 100\t 1.5 ns/op\nBenchmarkCacheTestSuite/BenchmarkHang/two\n",
			suite + hang + "    cache_test.go:12: --- PASS: noise\n",
		}) {
			gotest.Equal(sub, method, name(stdout))
		}
	})
	t.When("every benchmark that printed its name has a result", func(w *gotest.T) {
		for sub, stdout := range gotest.Each(w, []string{
			suite + fast + result,
			suite,
			"",
			suite + hang + "--- FAIL: BenchmarkCacheTestSuite/BenchmarkHang\n",
			suite + hang + "--- SKIP: BenchmarkCacheTestSuite/BenchmarkHang\n",
		}) {
			gotest.Equal(sub, whole, name(stdout))
		}
	})
}

func (s *DeadlineAttributionTestSuite) TestRunningUnitsAreBookedIntoTheStream(t *gotest.T) {
	t.When("the deadline cut the run short", func(w *gotest.T) {
		for sub, mode := range gotest.Each(w, []gotestrunner.RunMode{gotestrunner.RunCaptureJSON, gotestrunner.RunStreamJSON}) {
			running, booked := gotestrunner.ExportBookDeadline(mode, deadlineStream, context.DeadlineExceeded)
			gotest.Len(sub, running, 2)
			gotest.Equal(sub, deadlineBooked, booked,
				"each running unit fails with the timeout as its output, then its suite, verdict or not, and its package, keeping their time")
		}

		w.It("turns the tree red at the hung method, its suite and its package", func(it *gotest.T) {
			_, booked := gotestrunner.ExportBookDeadline(gotestrunner.RunCaptureJSON, deadlineStream, context.DeadlineExceeded)
			pkg := censusTree(it, deadlineStream+booked)
			gotest.NotZero(it, pkg)
			gotest.Equal(it, gotestspec.StatusFail, pkg.Status)
			gotest.Equal(it, 20*time.Second, pkg.Duration)
			var suite, hang, add *gotestspec.Node
			for _, n := range pkg.Nodes {
				if n.Name == "TestCartTestSuite" {
					suite = n
				}
				for _, m := range n.Children {
					switch m.Name {
					case "TestHang":
						hang = m
					case "TestAdd":
						add = m
					}
				}
			}
			gotest.NotZero(it, suite)
			gotest.Equal(it, gotestspec.StatusFail, suite.Status)
			gotest.NotZero(it, hang)
			gotest.Equal(it, gotestspec.StatusFail, hang.Status)
			gotest.Contains(it, strings.Join(hang.Output, ""), "gotest: --timeout expired while this test was running")
			gotest.NotZero(it, add)
			gotest.Equal(it, gotestspec.StatusPass, add.Status)
		})
	})

	t.When("no deadline cut the run short", func(w *gotest.T) {
		for sub, err := range gotest.Each(w, []error{nil, context.Canceled}) {
			running, booked := gotestrunner.ExportBookDeadline(gotestrunner.RunCaptureJSON, deadlineStream, err)
			gotest.Empty(sub, running)
			gotest.Empty(sub, booked)
		}
	})

	t.When("the text run has no stream", func(w *gotest.T) {
		var stdout, stderr bytes.Buffer
		c := gotestrunner.NewOutputCollector(gotestrunner.RunBatchText, false, gotestrunner.WithWriters(&stdout, &stderr))
		c.Register(censusPkg, 3)
		target := func(name string, bench bool) gotestrunner.SuiteTarget {
			return gotestrunner.SuiteTarget{SuiteSpec: gotestrunner.SuiteSpec{Package: censusPkg, SuiteName: name}, Bench: bench}
		}
		c.RecordResult(censusPkg, 0, gotestrunner.SuiteResult{Target: target("TestCartTestSuite", false)})
		c.RecordResult(censusPkg, 1, gotestrunner.SuiteResult{Target: target("TestHangTestSuite", false), ExitCode: 1, CutShort: true})
		c.RecordResult(censusPkg, 2, gotestrunner.SuiteResult{Target: target("CacheTestSuite", true), ExitCode: 1, CutShort: true})
		running := gotestrunner.ExportBookDeadlineOn(c, context.DeadlineExceeded)

		w.It("names the suites whose process the deadline signalled", func(it *gotest.T) {
			gotest.Equal(it, []unit{
				{Pkg: censusPkg, Path: "TestHangTestSuite"},
				{Pkg: censusPkg, Path: "BenchmarkCacheTestSuite"},
			}, running)
		})
	})
}

func (s *DeadlineAttributionTestSuite) TestUnitNames(t *gotest.T) {
	units := func(n int) []unit {
		out := make([]unit, n)
		for i := range out {
			out[i] = unit{Pkg: censusPkg, Path: "TestXTestSuite/TestM" + string(rune('a'+i))}
		}
		return out
	}

	t.It("joins each unit as package and path", func(it *gotest.T) {
		gotest.Equal(it, "example.com/pkg TestXTestSuite/TestMa, example.com/pkg TestXTestSuite/TestMb", gotestrunner.ExportUnitNames(units(2)))
	})

	t.It("names five and counts the rest", func(it *gotest.T) {
		gotest.Equal(it, "example.com/pkg TestXTestSuite/TestMa, example.com/pkg TestXTestSuite/TestMb, example.com/pkg TestXTestSuite/TestMc, example.com/pkg TestXTestSuite/TestMd, example.com/pkg TestXTestSuite/TestMe, … 2 more", gotestrunner.ExportUnitNames(units(7)))
	})
}
