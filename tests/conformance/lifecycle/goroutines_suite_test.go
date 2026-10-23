package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoroutinesTestSuite: where the wait on a goroutine started by gotest.Go
// ends, and what it ends with.
type GoroutinesTestSuite struct{}

func (s *GoroutinesTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

func (s *GoroutinesTestSuite) TestWaits(t *gotest.T) {
	// A wait that never ends is beyond the runner's graceful stop; go test's own
	// alarm bounds it.
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/goroutines/", "-timeout=20s")
	gotest.NoError(t, err)
	t.When("BeforeAll starts a goroutine with gotest.Go that only AfterAll stops", func(w *gotest.T) {
		w.It("the suite passes: the wait comes after AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestServerTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("a goroutine started by gotest.Go fails a fatal assertion", func(w *gotest.T) {
		w.It("the wait on it fails the method there, and nothing after the wait runs", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestGoexitTestSuite/TestGoexit"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "from the goroutine")
			gotest.NotContains(it, run.Stdout, "the method went on after the wait")
		})
	})
	t.When("a goroutine started by gotest.Go skips the test", func(w *gotest.T) {
		w.It("the wait on it skips the method there, without failing it", func(it *gotest.T) {
			gotest.Equal(it, "skip", run.Verdict("TestSkipTestSuite/TestSkip"), "stdout: %s", run.Stdout)
			gotest.NotContains(it, run.Stdout, "the method went on after the skip")
		})
	})
	t.When("a goroutine started by gotest.Go leaves through runtime.Goexit without a verdict", func(w *gotest.T) {
		w.It("the wait on it fails the method, saying the goroutine exited via runtime.Goexit", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestRawGoexitTestSuite/TestRawGoexit"), "stdout: %s", run.Stdout)
			gotest.Regexp(it, `goroutines_suite_test\.go:\d+: goroutine started by gotest\.Go exited via runtime\.Goexit`, run.Stdout)
		})
	})
}

type lastGoroutineRow struct {
	Desc   string
	suite  string
	prefix string // the suite's marker prefix
}

func (s *GoroutinesTestSuite) TestSetupWaits(t *gotest.T) {
	// A wait that never ends is beyond the runner's graceful stop; go test's own
	// alarm bounds it.
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/lcagoroutines/", "-timeout=20s")
	gotest.NoError(t, err)
	t.When("BeforeAll starts goroutines with gotest.Go on When, It, Each row and NewTWithDeadline Ts that only AfterAll stops", func(w *gotest.T) {
		w.It("the suite passes: the waits come after AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestDerivedTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("a BeforeAll taking a *testing.T starts goroutines with gotest.Go on gotest.NewT(t) and NewTWithDeadline(t) that only AfterAll stops", func(w *gotest.T) {
		w.It("the suite passes: the waits come after AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestStdTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		})
	})
	t.When("a goroutine BeforeAll started starts another with gotest.Go once AfterAll stopped it", func(w *gotest.T) {
		w.It("that one is waited for too, and its panic fails the suite", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestLateTestSuite"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout+run.Stderr, "LATE-GOROUTINE-PANIC")
		})
	})
	// The goroutine sleeps twenty times the 200ms SetupTimeout.
	t.When("a goroutine BeforeAll started runs past the suite's declared SetupTimeout after AfterAll", func(w *gotest.T) {
		w.It("fails the suite with a verdict naming the wait and its SetupTimeout", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSlowTestSuite"), "stdout: %s", run.Stdout)
			gotest.Contains(it, run.Stdout, "the wait on a goroutine BeforeAll started with gotest.Go exceeded its configured SetupTimeout of 200ms")
		})
	})
	// The first goroutine ends after a second; the last fails or panics at once.
	t.When("the last of several goroutines BeforeAll started with gotest.Go fails or panics", func(w *gotest.T) {
		w.It("fails the suite and waits for every other one before the fixture is released", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []lastGoroutineRow{
				{Desc: "the last goroutine fails", suite: "TestFailLastTestSuite", prefix: "fail: "},
				{Desc: "the last goroutine panics", suite: "TestPanicLastTestSuite", prefix: "panic: "},
			}) {
				gotest.Equal(sub, "fail", run.Verdict(row.suite), "stdout: %s", run.Stdout)
				gotest.True(sub, run.MarkerOrder(row.prefix+"first goroutine ended", row.prefix+"fixture.AfterAll"), "markers: %v", run.Markers)
			}
		})
	})
}
