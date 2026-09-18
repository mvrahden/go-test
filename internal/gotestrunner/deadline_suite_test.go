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
