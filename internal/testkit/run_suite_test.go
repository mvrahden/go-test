package testkit_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunOraclesTestSuite drives the real CLI over the markers fixture and checks
// what the oracles read back from it.
// Sequential: both methods share one built CLI and each spawns a full run.
type RunOraclesTestSuite struct {
	run testkit.Run
}

func (s *RunOraclesTestSuite) BeforeAll(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "./testdata/markers/")
	gotest.NoError(t, err)
	s.run = run
}

func (s *RunOraclesTestSuite) AfterAll(t *gotest.T) {}

func (s *RunOraclesTestSuite) TestRunPackageReportsTheRun(t *gotest.T) {
	t.When("a package with a passing and a failing method ran", func(w *gotest.T) {
		w.It("exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, s.run.Exit)
		})
		w.It("reports each method's verdict", func(it *gotest.T) {
			gotest.Equal(it, "pass", s.run.Verdict("TestMarkerTestSuite/TestOne"))
			gotest.Equal(it, "fail", s.run.Verdict("TestMarkerTestSuite/TestTwo"))
			gotest.Equal(it, "fail", s.run.Verdict("TestMarkerTestSuite"))
		})
		w.It("reports the package verdict for the empty path", func(it *gotest.T) {
			gotest.Equal(it, "fail", s.run.Verdict(""))
		})
		w.It("reports no verdict for a test that never ran", func(it *gotest.T) {
			gotest.Zero(it, s.run.Verdict("TestMarkerTestSuite/TestNine"))
		})
		w.It("keeps the stream's events", func(it *gotest.T) {
			gotest.NotEmpty(it, s.run.Events)
			gotest.Contains(it, s.run.Stdout, `"Action":"fail"`)
		})
	})

	t.When("the suite stamped its lifecycle", func(w *gotest.T) {
		w.It("reads the markers in write order", func(it *gotest.T) {
			gotest.Equal(it, []string{"beforeAll", "TestOne", "afterAll"}, s.run.Markers)
		})
		w.It("confirms a relative order that holds", func(it *gotest.T) {
			gotest.True(it, s.run.MarkerOrder("beforeAll", "TestOne", "afterAll"))
			gotest.True(it, s.run.MarkerOrder("beforeAll", "afterAll"))
		})
		w.It("rejects an order that does not hold or a marker that is absent", func(it *gotest.T) {
			gotest.False(it, s.run.MarkerOrder("afterAll", "beforeAll"))
			gotest.False(it, s.run.MarkerOrder("beforeAll", "missing"))
		})
	})
}

func (s *RunOraclesTestSuite) TestRunCLIReportsStreamsAndExit(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "version")
	gotest.NoError(t, err)
	t.When("a bare subcommand succeeds", func(w *gotest.T) {
		w.It("exits 0 with its output on stdout", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit)
			gotest.Regexp(it, "gotest", run.Stdout)
			gotest.Empty(it, run.Events)
		})
	})
	t.When("a subcommand is unknown", func(w *gotest.T) {
		bad, err := testkit.RunCLI(t.Context(), ".", "--no-such-flag")
		gotest.NoError(t, err)
		w.It("reports a non-zero exit with the complaint on stderr", func(it *gotest.T) {
			gotest.NotZero(it, bad.Exit)
			gotest.NotEmpty(it, bad.Stderr)
		})
	})
}
