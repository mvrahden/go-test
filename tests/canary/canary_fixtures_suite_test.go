package canary_test //nolint:fail-guard

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// fixtureRun is one CLI run over a fixture package: its exit code, its
// verdict rows in golden form, and what it printed on stdout and stderr.
type fixtureRun struct {
	exit           int
	rows           []string
	stdout, stderr string
}

// runFixture runs the CLI in -json mode over pkg with dir as its marker dir.
func (s *CanaryTestSuite) runFixture(t *gotest.T, pkg, dir string) fixtureRun {
	cmd := exec.Command(s.binary, "-json", "./testdata/"+pkg+"/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := fixtureRun{}
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "run %s: %v", pkg, err)
		}
		r.exit = exitErr.ExitCode()
	}
	r.stdout, r.stderr = stdout.String(), stderr.String()
	seen := map[string]bool{}
	for _, ev := range testkit.ParseEvents(stdout.Bytes()) {
		if ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip" {
			continue
		}
		test := ev.Test
		if test == "" {
			test = "-"
		}
		seen[pkg+" "+ev.Action+" "+test] = true
	}
	for row := range seen {
		r.rows = append(r.rows, row)
	}
	sort.Strings(r.rows)
	return r
}

// checkFixture runs pkg and compares its exit code and rows with the golden.
func (s *CanaryTestSuite) checkFixture(t *gotest.T, pkg, dir string) fixtureRun {
	want, ok := s.expected[pkg]
	if !ok {
		fatalf(t, "no golden entry for %s", pkg)
	}
	r := s.runFixture(t, pkg, dir)
	if r.exit != want.exit {
		t.Errorf("%s: exit code %d, want %d\n%s", pkg, r.exit, want.exit, r.stderr)
	}
	if strings.Join(r.rows, "\n") != strings.Join(want.rows, "\n") {
		t.Errorf("%s: verdicts differ from the golden list\n got:\n  %s\n want:\n  %s",
			pkg, strings.Join(r.rows, "\n  "), strings.Join(want.rows, "\n  "))
	}
	return r
}

// An error a fixture AfterEach returns is reported at the hook's own file,
// not inside the runtime's.
func (s *CanaryTestSuite) TestAFixtureAfterEachErrorIsReportedAtTheHook(t *gotest.T) {
	r := s.checkFixture(t, "fixtureafterfail", t.TempDir())
	if !strings.Contains(r.stdout, "fixture.go:") {
		t.Errorf("fixtureafterfail: the failure is not reported at the hook's file")
	}
	if strings.Contains(r.stdout, "methods.go:") {
		t.Errorf("fixtureafterfail: the failure is reported inside the runtime's file")
	}
}

// A fixture whose AfterAll fails between two setup attempts fails the run and
// reports that AfterAll's error, though its second attempt comes up: a package
// fixture and a shared one alike.
func (s *CanaryTestSuite) TestAFailedReleaseBetweenAttemptsFailsTheRun(t *gotest.T) {
	for _, c := range []struct{ pkg, says string }{
		{"packagerelease", "the package release leaked"},
		{"sharedrelease", "the shared release leaked"},
	} {
		r := s.checkFixture(t, c.pkg, t.TempDir())
		if !strings.Contains(r.stdout+r.stderr, c.says) {
			t.Errorf("%s: the run does not report the release's error %q", c.pkg, c.says)
		}
	}
}

// A shared fixture with a 2s Timeout and one retry whose attempts and the
// release between them each take 1.6s is waited for: the wait counts the
// release.
func (s *CanaryTestSuite) TestTheSetupWaitCountsTheReleaseBetweenAttempts(t *gotest.T) {
	s.checkFixture(t, "slowretry", t.TempDir())
}

// A shared fixture that fails its last attempt fails the run, though the suite
// binding it would pass without it.
func (s *CanaryTestSuite) TestAFailedLastAttemptFailsTheRun(t *gotest.T) {
	s.checkFixture(t, "lastattempt", t.TempDir())
}

// A run stopped while a shared fixture's dependency sets up runs no AfterAll
// for the fixture, whose BeforeAll never ran, and the dependency's AfterAll.
func (s *CanaryTestSuite) TestASetupCancelledBeforeBeforeAllReleasesNothing(t *gotest.T) {
	dir := t.TempDir()
	cmd := exec.Command(s.binary, "-json", "./testdata/cancelbeforesetup/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(dir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	if err != nil {
		fatalf(t, "start: %v", err)
	}
	markers := filepath.Join(dir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "hold.BeforeAll") {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			fatalf(t, "cancelbeforesetup: the dependency never began its setup:\n%s", out.String())
		}
	}
	if err := testkit.Interrupt(cmd, tree); err != nil {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		fatalf(t, "cancelbeforesetup: signal: %v", err)
	}
	_ = cmd.Wait()
	tree.Release()
	got, _ := testkit.ReadMarkers(markers)
	if want := []string{"hold.BeforeAll", "hold.AfterAll"}; !slices.Equal(got, want) {
		t.Errorf("cancelbeforesetup: markers %v, want %v\n%s", got, want, out.String())
	}
}
