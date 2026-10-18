package lifecycle_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// started starts cmd as a CLI run whose streams are kept, and returns once
// every marker in wants is in markerDir. A run that never gets there is
// killed before the test fails, so nothing outlives it.
func started(t *gotest.T, cmd *exec.Cmd, markerDir string, wants ...string) *proctree.Tree {
	cmd.Stdout, cmd.Stderr = new(strings.Builder), new(strings.Builder)
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if !slices.ContainsFunc(wants, func(w string) bool { return !slices.ContainsFunc(got, prefixed(w)) }) {
			return tree
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			gotest.Fail(t, "the run never reached %v:\nmarkers: %v\n%s%s", wants, got, cmd.Stdout, cmd.Stderr) //nolint:fail-guard // the tree is killed before the test fails
		}
	}
}

// prefixed matches a marker equal to want, or starting with it when want
// ends in "=".
func prefixed(want string) func(string) bool {
	return func(m string) bool {
		if strings.HasSuffix(want, "=") {
			return strings.HasPrefix(m, want)
		}
		return m == want
	}
}

// markerValue is the value of the first key=value marker with key.
func markerValue(markers []string, key string) string {
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, key+"="); ok {
			return v
		}
	}
	return ""
}

// awaitMarkers waits up to within for every marker in wants and reports the
// markers it read last.
func awaitMarkers(markerDir string, within time.Duration, wants ...string) []string {
	markers := filepath.Join(markerDir, testkit.MarkersFile)
	for deadline := time.Now().Add(within); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if !slices.ContainsFunc(wants, func(w string) bool { return !slices.Contains(got, w) }) || time.Now().After(deadline) {
			return got
		}
	}
}

// ended waits up to within for the CLI and reads back what it left; one
// still running then is killed first, and the run's Exit is -2.
func ended(cmd *exec.Cmd, tree *proctree.Tree, markerDir string, within time.Duration) testkit.Run {
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	hung := false
	select {
	case <-done:
	case <-time.After(within):
		hung = true
		_ = tree.Kill()
		<-done
	}
	tree.Release()
	run := testkit.Run{Exit: -2}
	if !hung && cmd.ProcessState != nil {
		run.Exit = cmd.ProcessState.ExitCode()
	}
	run.Stdout = cmd.Stdout.(*strings.Builder).String()
	run.Stderr = cmd.Stderr.(*strings.Builder).String()
	run.Events = testkit.ParseEvents([]byte(run.Stdout))
	run.Markers, _ = testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))
	return run
}
