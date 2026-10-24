package lifecycle_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AsyncMethodsTestSuite: an async method waits for done, for a declared
// budget at most, and no longer once nothing can call done. One run of the
// lcaasync package serves every method.
type AsyncMethodsTestSuite struct {
	run testkit.Run
}

func (s *AsyncMethodsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: setup ~62s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, SetupTimeout: 3 * time.Minute}
}

func (s *AsyncMethodsTestSuite) BeforeAll(t *gotest.T) {
	var err error
	s.run, err = testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcaasync/")
	gotest.NoError(t, err)
}

// AfterAll has nothing to release: the run's markers live in a temp dir.
func (s *AsyncMethodsTestSuite) AfterAll(t *gotest.T) {}

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
	run := s.run
	const method = "TestFatalTestSuite/TestFatalGoroutineAsync"
	t.When("the goroutine that owns done fails a fatal assertion and no other goroutine of the method is left", func(w *gotest.T) {
		w.It("fails the method at once, not at the end of its budget", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict(method), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestFatalTestSuite/TestCallsDoneAsync"))
			// The method's own time, not the build before it: its budget is 20s.
			took := elapsedOf(&run, method, "fail")
			gotest.GreaterOrEqual(it, took, time.Duration(0), "stdout: %s", run.Stdout)
			gotest.Less(it, took, 10*time.Second)
		})
	})
}

type helperRow struct {
	Desc string
	path string
	ends string   // the marker the wait ends after; "" when only the output says
	says []string // what the method's output carries
}

func (s *AsyncMethodsTestSuite) TestAnotherGoroutineFails(t *gotest.T) {
	run := s.run
	// In every row a goroutine other than the owner of done fails at once;
	// the owner works on for 700ms, or hangs past the Timeout. AfterEach
	// marks the wait's end.
	helper := "a helper goroutine fails"
	t.When("a goroutine of an async method other than the one that owns done fails a fatal assertion", func(w *gotest.T) {
		w.It("fails the method, and the wait ends only when the owner calls done, fails or outlasts the Timeout", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []helperRow{
				{Desc: "the owner calls done", path: "TestHelperFailsTestSuite/TestOwnerCallsDoneAsync", ends: "owner called done", says: []string{helper}},
				{Desc: "the owner fails", path: "TestHelperFailsTestSuite/TestOwnerFailsAsync", ends: "owner failed", says: []string{helper, "the owner of done fails"}},
				{Desc: "the owner outlasts the Timeout", path: "TestHelperFailsTestSuite/TestOwnerHangsAsync", says: []string{helper, "TestOwnerHangsAsync failed, and its wait for done() lasted until the test deadline: a goroutine it started was still running"}},
				{Desc: "the owner is a job of a pool BeforeAll started", path: "TestOwnersTestSuite/TestPoolOwnerFailsAsync", ends: "pool owner failed", says: []string{helper, "the pool owner fails late"}},
				{Desc: "the owner is a time.AfterFunc callback", path: "TestOwnersTestSuite/TestAfterFuncOwnerAsync", ends: "AfterFunc owner called done", says: []string{helper}},
				{Desc: "the owner fails while a goroutine BeforeEach started works on", path: "TestBeforeEachGoroutineTestSuite/TestOwnerFailsAsync", ends: "BeforeEach's goroutine failed", says: []string{"the owner of done fails", "BeforeEach's goroutine fails late"}},
			}) {
				gotest.Equal(sub, "fail", run.Verdict(row.path), "stdout: %s", run.Stdout)
				out := outputOf(&run, row.path)
				for _, says := range row.says {
					gotest.Contains(sub, out, says)
				}
				gotest.Contains(sub, run.Markers, "AfterEach "+row.path)
				if row.ends != "" {
					gotest.True(sub, run.MarkerOrder(row.ends, "AfterEach "+row.path), "markers: %v", run.Markers)
				}
			}
		})
	})
}

func (s *AsyncMethodsTestSuite) TestAGoGoroutinePanics(t *gotest.T) {
	run := s.run
	const method = "TestPanicTestSuite/TestGoPanicsAsync"
	t.When("a goroutine gotest.Go started panics while an async method waits for done", func(w *gotest.T) {
		w.It("ends the wait at once, runs AfterEach and reports the panic with the stack from where it happened", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict(method), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Markers, "AfterEach "+method)
			gotest.Contains(it, run.Stdout, "the owner of done panics")
			gotest.Contains(it, run.Stdout, "[recovered from goroutine started by gotest.Go]")
			gotest.Regexp(it, `owners_suite_test\.go:\d+`, run.Stdout)
			// The method's own time, not the build before it: its budget is 20s.
			took := elapsedOf(&run, method, "fail")
			gotest.GreaterOrEqual(it, took, time.Duration(0), "stdout: %s", run.Stdout)
			gotest.Less(it, took, 10*time.Second)
		})
	})
}

func (s *AsyncMethodsTestSuite) TestAParallelSiblingsGoroutine(t *gotest.T) {
	run := s.run
	t.When("the owner of a parallel async method's done fails while a goroutine of its sibling's, whose creators ended, runs on", func(w *gotest.T) {
		w.It("ends the failed method's wait without waiting for the sibling's goroutine", func(it *gotest.T) {
			failed := "TestParallelTestSuite/TestOwnerFailsAsync"
			gotest.Equal(it, "fail", run.Verdict(failed), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestParallelTestSuite/TestOrphanCallsDoneAsync"), "stdout: %s", run.Stdout)
			gotest.True(it, run.MarkerOrder("AfterEach "+failed, "orphan called done"), "markers: %v", run.Markers)
		})
	})
}

func (s *AsyncMethodsTestSuite) TestAfterEachOverruns(t *gotest.T) {
	run := s.run
	t.When("an async method calls done in time and its AfterEach overruns the declared Timeout", func(w *gotest.T) {
		w.It("fails the method for the overrun", func(it *gotest.T) {
			path := "TestHookOverrunTestSuite/TestCallsDoneAsync"
			gotest.Equal(it, "fail", run.Verdict(path), "stdout: %s", run.Stdout)
			gotest.Contains(it, outputOf(&run, path), "AfterEach exceeded its configured Timeout of 1s")
		})
	})
}
