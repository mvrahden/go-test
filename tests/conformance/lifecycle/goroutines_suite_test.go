package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoroutinesTestSuite: where the wait on a goroutine started by gotest.Go
// ends, and what it ends with.
type GoroutinesTestSuite struct{}

func (s *GoroutinesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
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
	t.When("BeforeAll starts a goroutine with gotest.Go on a T from NewTWithDeadline that only AfterAll stops", func(w *gotest.T) {
		w.It("the suite passes: the wait comes after AfterAll", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestDerivedTestSuite"), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
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
