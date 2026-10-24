//go:build unix

package testkit_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func (s *RunOraclesTestSuite) TestACancelledRunTearsItsFixturesDown(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	module := t.TempDir()
	_, err = testkit.StagePackage(root, "./testdata/stopshared", module, "stopmod")
	gotest.NoError(t, err)
	markersDir := t.TempDir()
	markers := filepath.Join(markersDir, testkit.MarkersFile)

	ctx, cancel := context.WithCancel(t.Context())
	// Cancelled once the suite holds, with its fixture up.
	wait := gotest.Go(t, func() {
		for ctx.Err() == nil {
			got, _ := testkit.ReadMarkers(markers)
			if slices.Contains(got, "suite holds") {
				cancel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	_, runErr := testkit.RunCLIWithEnv(ctx, module, []string{"GOTEST_CANARY_DIR=" + markersDir}, "-json", "./stopshared/")
	cancel()
	wait()
	got, _ := testkit.ReadMarkers(markers)
	t.When("the context of a run whose shared fixture is up ends", func(w *gotest.T) {
		w.It("returns the run", func(it *gotest.T) {
			gotest.NoError(it, runErr)
			gotest.Contains(it, got, "suite holds")
		})
		w.It("returns once the fixture's 3s AfterAll ran to its end", func(it *gotest.T) {
			gotest.Contains(it, got, "fixture released", "markers: %v", got)
		})
		w.It("leaves no fixture process behind", func(it *gotest.T) {
			gotest.True(it, processGone(it, got), "markers: %v", got)
		})
	})
}

// processGone reports whether no process with the pid stamped as "pid=<n>"
// is left.
func processGone(t *gotest.T, markers []string) bool {
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

func (s *RunOraclesTestSuite) TestOutputHeldPastTheExitIsAnError(t *gotest.T) {
	t.When("a run exits while a process it left holds its output open", func(w *gotest.T) {
		for row, tC := range gotest.Each(w, []struct {
			Desc   string
			script string
			exit   int
		}{
			{"cleanly", "sleep 2 & exit 0", 0},
			{"with a failure", "sleep 2 & exit 3", 3},
		}) {
			// The "CLI" exits at once and leaves a process holding its output.
			started := time.Now()
			run, err := testkit.ExportRunBinary(row.Context(), "/bin/sh", 300*time.Millisecond, "-c", tC.script)
			took := time.Since(started)
			gotest.Equal(row, tC.exit, run.Exit, "returns the exit code")
			gotest.Less(row, took, 1500*time.Millisecond, "returns once the grace is over")
			gotest.ErrorContains(row, err, "still held open", "says the output was held")
		}
	})
}
