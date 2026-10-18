package lifecycle_test

import (
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// gridRow is one cell of the hook × failure-mode grid: which hook goes wrong
// and how, and what the run must then show.
type gridRow struct {
	Desc     string
	at, mode string
	ran      []string          // the exact marker sequence; nil leaves it unchecked
	verdicts map[string]string // test path → last verdict
	exit     int
}

// runGrid runs the hooks fixture package with one hook told to misbehave.
func runGrid(t *gotest.T, at, mode string) testkit.Run {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"HOOKS_AT=" + at, "HOOKS_MODE=" + mode}, "../testdata/hooks/")
	gotest.NoError(t, err)
	return run
}

// checkGrid holds a run to its row.
func checkGrid(t *gotest.T, row gridRow, run *testkit.Run) {
	gotest.Equal(t, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	if row.ran != nil {
		gotest.Equal(t, row.ran, run.Markers)
	}
	for path, want := range row.verdicts {
		gotest.Equal(t, want, run.Verdict(path), "verdict of %s", path)
	}
}

// Marker sequences of the hooks package: a clean run, and the hooks of one method.
var (
	suite       = "TestGridTestSuite"
	body        = suite + "/TestBody"
	next        = suite + "/TestNext"
	cleanRun    = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	bodySkipped = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// A panic ends the suite after the method's own teardown, as under go
	// test: the next method never starts and has no verdict.
	suiteEndsAfterBody  = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	suiteEndsBeforeBody = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
)
