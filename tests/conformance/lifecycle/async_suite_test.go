package lifecycle_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncMethodsTestSuite: an async method waits for done, for a declared
// budget at most, and no longer once nothing can call done.
type AsyncMethodsTestSuite struct{}

func (s *AsyncMethodsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// outputOf is everything the stream printed for the test at path.
func outputOf(run *testkit.Run, path string) string {
	var out strings.Builder
	for _, ev := range run.Events {
		if ev.Test == path && ev.Action == "output" {
			out.WriteString(ev.Output)
		}
	}
	return out.String()
}

// elapsedOf is the time the stream says the test at path took, -1 when it
// reported none.
func elapsedOf(run *testkit.Run, path, action string) time.Duration {
	for line := range strings.SplitSeq(run.Stdout, "\n") {
		var ev struct {
			Action  string   `json:"Action"`
			Test    string   `json:"Test"`
			Elapsed *float64 `json:"Elapsed"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Test == path && ev.Action == action && ev.Elapsed != nil {
			return time.Duration(*ev.Elapsed * float64(time.Second))
		}
	}
	return -1
}

func (s *AsyncMethodsTestSuite) TestAFatalGoroutine(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/asyncmethods/")
	gotest.NoError(t, err)
	const method = "TestAsyncTestSuite/TestFatalGoroutineAsync"
	t.When("the goroutine that owns done fails a fatal assertion and no other goroutine of the method is left", func(w *gotest.T) {
		w.It("fails the method at once, not at the end of its budget", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict(method), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestAsyncTestSuite/TestCallsDoneAsync"))
			// The method's own time, not the build before it: its budget is 20s.
			took := elapsedOf(&run, method, "fail")
			gotest.GreaterOrEqual(it, took, time.Duration(0), "stdout: %s", run.Stdout)
			gotest.Less(it, took, 10*time.Second)
		})
	})
}

type helperRow struct {
	Desc   string
	method string
	ends   string // the marker the wait ends after; "" when only the output says
	says   string // what the method's output carries besides the helper's failure
}

func (s *AsyncMethodsTestSuite) TestAnotherGoroutineFails(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcaasync/", "-run=TestHelperFailsTestSuite")
	gotest.NoError(t, err)
	// In every row a helper goroutine fails at once; the owner of done works
	// on for 700ms, or hangs past the 3s Timeout. AfterEach marks the wait's end.
	t.When("a goroutine of an async method other than the one that owns done fails a fatal assertion", func(w *gotest.T) {
		w.It("fails the method, and the wait ends only when the owner calls done, fails or outlasts the Timeout", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []helperRow{
				{Desc: "the owner calls done", method: "TestOwnerCallsDoneAsync", ends: "owner called done"},
				{Desc: "the owner fails", method: "TestOwnerFailsAsync", ends: "owner failed", says: "the owner of done fails"},
				{Desc: "the owner outlasts the Timeout", method: "TestOwnerHangsAsync", says: "TestOwnerHangsAsync: done() was not called before the test deadline"},
			}) {
				path := "TestHelperFailsTestSuite/" + row.method
				gotest.Equal(sub, "fail", run.Verdict(path), "stdout: %s", run.Stdout)
				out := outputOf(&run, path)
				gotest.Contains(sub, out, "a helper goroutine fails")
				if row.says != "" {
					gotest.Contains(sub, out, row.says)
				}
				gotest.Contains(sub, run.Markers, "AfterEach "+path)
				if row.ends != "" {
					gotest.True(sub, run.MarkerOrder(row.ends, "AfterEach "+path), "markers: %v", run.Markers)
				}
			}
		})
	})
}

func (s *AsyncMethodsTestSuite) TestAfterEachOverruns(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcaasync/", "-run=TestHookOverrunTestSuite")
	gotest.NoError(t, err)
	t.When("an async method calls done in time and its AfterEach overruns the declared Timeout", func(w *gotest.T) {
		w.It("fails the method for the overrun", func(it *gotest.T) {
			path := "TestHookOverrunTestSuite/TestCallsDoneAsync"
			gotest.Equal(it, "fail", run.Verdict(path), "stdout: %s", run.Stdout)
			gotest.Contains(it, outputOf(&run, path), "exceeded its configured Timeout of 1s")
		})
	})
}
