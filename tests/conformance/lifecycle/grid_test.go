package lifecycle_test

import (
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// gridRow is one cell of the hook × failure-mode grid: the run of the hooks
// package with one hook told to misbehave, and what that run must show.
type gridRow struct {
	Desc     string
	run      *rowRun
	ran      []string          // the exact marker sequence; nil leaves it unchecked
	verdicts map[string]string // test path → last verdict
	exit     int
	said     string // a pattern the run's test output matches; empty leaves it unchecked
}

// rowRun is a run of the hooks package that started when its row was written.
type rowRun struct {
	done chan struct{}
	run  testkit.Run
	err  error
}

// gridSlots caps the hooks package runs in flight across the grid.
var gridSlots = make(chan struct{}, 4)

// startRow starts running the hooks package with the hook at told to
// misbehave as mode says. No row's run depends on another's, so a table's
// runs go at once, and each row waits only for its own.
func startRow(t *gotest.T, at, mode string) *rowRun {
	dir, err := os.MkdirTemp(t.TempDir(), "row-")
	gotest.NoError(t, err)
	r := &rowRun{done: make(chan struct{})}
	ctx := t.Context()
	gotest.Go(t, func() {
		defer close(r.done)
		gridSlots <- struct{}{}
		defer func() { <-gridSlots }()
		r.run, r.err = testkit.RunPackageWithEnv(ctx, dir, []string{"HOOKS_AT=" + at, "HOOKS_MODE=" + mode}, "../testdata/hooks/")
	})
	return r
}

// checkGrid holds a row's run to the row.
func checkGrid(t *gotest.T, row *gridRow) {
	<-row.run.done
	gotest.NoError(t, row.run.err)
	run := &row.run.run
	gotest.Equal(t, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	if row.ran != nil {
		gotest.Equal(t, row.ran, run.Markers)
	}
	for path, want := range row.verdicts {
		gotest.Equal(t, want, run.Verdict(path), "verdict of %s", path)
	}
	if row.said != "" {
		var out strings.Builder
		for _, ev := range run.Events {
			out.WriteString(ev.Output)
		}
		gotest.Regexp(t, row.said, out.String())
	}
}

// lineOf is the line of the hooks package's file that contains text.
func lineOf(t *gotest.T, file, text string) string {
	src, err := os.ReadFile("../testdata/hooks/" + file)
	gotest.NoError(t, err)
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, text) {
			return strconv.Itoa(i + 1)
		}
	}
	gotest.Fail(t, "%s has no line with %q", file, text)
	return ""
}

// overrunAt matches the overrun verdict of phase, located at the line of file
// that contains text.
func overrunAt(t *gotest.T, file, text, phase string) string {
	return `(?m)^\s+` + regexp.QuoteMeta(file) + `:` + lineOf(t, file, text) + `: ` + regexp.QuoteMeta(phase) + `exceeded its configured Timeout of 300ms`
}

// failedAt matches msg, located at the line of file that contains text.
func failedAt(t *gotest.T, file, text, msg string) string {
	return `(?m)^\s+` + regexp.QuoteMeta(file) + `:` + lineOf(t, file, text) + `: ` + regexp.QuoteMeta(msg)
}

// panicAt matches a panic's message and its stack's frame at the line of
// file that contains text.
func panicAt(t *gotest.T, file, text, msg string) string {
	return `(?s)panic: ` + regexp.QuoteMeta(msg) + `.*testdata/hooks/` + regexp.QuoteMeta(file) + `:` + lineOf(t, file, text) + ` `
}

// Marker sequences of the hooks package: a clean run, and the hooks of one method.
var (
	suite       = "TestGridTestSuite"
	body        = suite + "/TestBody"
	next        = suite + "/TestNext"
	cleanRun    = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	bodySkipped = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "suite.AfterEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// After a fixture BeforeEach that overran only the fixture's AfterEach
	// runs of that method.
	suiteHooksSkipped = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "fixture.AfterEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	// A panic ends the suite after the method's own teardown, as under go
	// test: the next method never starts and has no verdict.
	suiteEndsAfterBody    = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "body", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	suiteEndsBeforeBody   = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.BeforeEach", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
	suiteEndsAtBeforeEach = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "suite.AfterAll", "fixture.AfterAll"}
	// A fixture BeforeEach that failed opened nothing to release: no hook of
	// that method runs after it.
	methodNeverOpened = []string{"fixture.BeforeAll", "suite.BeforeAll", "fixture.BeforeEach", "fixture.BeforeEach", "suite.BeforeEach", "next", "suite.AfterEach", "fixture.AfterEach", "suite.AfterAll", "fixture.AfterAll"}
)

// withMarkerAfter is markers with marker inserted after the first after.
func withMarkerAfter(markers []string, after, marker string) []string {
	i := slices.Index(markers, after) + 1
	return slices.Insert(slices.Clone(markers), i, marker)
}
