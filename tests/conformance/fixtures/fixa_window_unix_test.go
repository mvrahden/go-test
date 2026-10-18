//go:build unix

package fixtures_test

import (
	"errors"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// fixtureProcessGone reports whether no process with the pid a fixture stamped as
// "pid=<n>" is left.
func fixtureProcessGone(t *gotest.T, markers []string) bool {
	for _, m := range markers {
		if pid, ok := strings.CutPrefix(m, "pid="); ok {
			n, err := strconv.Atoi(pid)
			gotest.NoError(t, err)
			return errors.Is(syscall.Kill(n, 0), syscall.ESRCH)
		}
	}
	gotest.Fail(t, "no pid marker in %v", markers)
	return false
}

func (s *FixtureWindowTestSuite) TestAStoppedRunEndsTheWait(t *gotest.T) {
	began := time.Now()
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"FIXA_UNBOUNDED=1"}, "../testdata/fixatail/", "--timeout=20s")
	elapsed := time.Since(began)
	gotest.NoError(t, err)
	t.When("the run's --timeout expires while a shared fixture started for the exclusive tail hangs with no Timeout", func(w *gotest.T) {
		w.It("the wait ends with the run and the run fails", func(it *gotest.T) {
			gotest.Contains(it, run.Stderr, "shared fixture start cancelled")
			gotest.Equal(it, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Less(it, elapsed, 4*time.Minute)
		})
		w.It("leaves no fixture process behind", func(it *gotest.T) {
			gotest.True(it, fixtureProcessGone(it, run.Markers), "markers: %v", run.Markers)
		})
	})
}
