//go:build unix

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RelayFallbackTestSuite: a suite the stop line cannot reach is stopped by a
// Ctrl-C to its process group, which the converter beside it never gets.
// Sequential: it builds the CLI once and every method drives a live process.
type RelayFallbackTestSuite struct {
	cli      string
	wrappers string
}

func (s *RelayFallbackTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute, SetupTimeout: 45 * time.Second}
}

func (s *RelayFallbackTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	s.wrappers = t.TempDir()
	for name, script := range map[string]string{
		// The shell keeps the channel open and says nothing on it; the suite
		// runs with descriptor 3 closed.
		"hold3.sh": "#!/bin/sh\n\"$@\" 3>&-\n",
		// The shell says hello for the suite, then runs it with the channel
		// closed at its end. It closes descriptor 3 with an exec of its own:
		// macOS's /bin/sh, bash 3.2, keeps a copy of a descriptor closed on
		// an exec that also runs a command, and the command inherits it.
		"hello3.sh": "#!/bin/sh\nprintf 'hello 1 %d\\n' $$ >&3\nexec 3>&-\nexec \"$@\"\n",
	} {
		gotest.NoError(t, os.WriteFile(filepath.Join(s.wrappers, name), []byte(script), 0o700)) //nolint:gosec // G306: the wrapper must be executable
	}
}

// AfterAll has nothing to release: the binary and the wrappers live in temp dirs.
func (s *RelayFallbackTestSuite) AfterAll(t *gotest.T) {}

// relayStopped runs lcrrelay's HoldTestSuite under the -exec wrapper, stops
// it with a SIGINT to the CLI's process group once its method holds, as a
// terminal's Ctrl-C does, and reads back what the run left.
func (s *RelayFallbackTestSuite) relayStopped(t *gotest.T, wrapper string) testkit.Run {
	markerDir := t.TempDir()
	cmd := exec.Command(s.cli, "-json", "-exec", filepath.Join(s.wrappers, wrapper), "-run=^TestHoldTestSuite$", "../testdata/lcrrelay/") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir, "running", "child started")
	signalled(t, cmd, tree, syscall.Kill(-cmd.Process.Pid, syscall.SIGINT))
	return ended(cmd, tree, markerDir, 30*time.Second)
}

// relayVerdicts asserts that the stream holds every method's verdict, the
// methods not started skipped with the reason run stopping.
func relayVerdicts(t *gotest.T, run *testkit.Run) {
	gotest.Equal(t, "pass", run.Verdict("TestHoldTestSuite/TestHolds"), "stream:\n%s", run.Stdout)
	for _, method := range []string{"TestSecond", "TestThird"} {
		path := "TestHoldTestSuite/" + method
		gotest.Equal(t, "skip", run.Verdict(path), "verdict of %s:\n%s", path, run.Stdout)
		gotest.Contains(t, outputOf(run, path), "run stopping")
	}
	gotest.Equal(t, "fail", run.Verdict("-"), "stream:\n%s", run.Stdout)
}

func (s *RelayFallbackTestSuite) TestAWrapperThatClosesDescriptor3BehindIt(t *gotest.T) {
	run := s.relayStopped(t, "hold3.sh")

	t.When("a run is stopped while its suite, run by an -exec wrapper that closes descriptor 3, has said no hello", func(w *gotest.T) {
		w.It("interrupts the suite's process group with SIGINT", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "suite interrupt")
			// The process the test started is in that group.
			gotest.Contains(it, run.Markers, "child SIGINT")
			gotest.NotContains(it, run.Markers, "child SIGTERM")
		})
		w.It("leaves the converter running, the suite's output converted once", func(it *gotest.T) {
			gotest.NotContains(it, run.Stderr, "the converter ended early")
			gotest.Contains(it, run.Stderr, "gotest: suite binary terminated by signal")
		})
		w.It("ends the stream whole, the methods not started skipped with the reason run stopping", func(it *gotest.T) {
			relayVerdicts(it, &run)
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
		})
	})
}

func (s *RelayFallbackTestSuite) TestAStopLineThatCannotBeWritten(t *gotest.T) {
	run := s.relayStopped(t, "hello3.sh")

	t.When("a run is stopped while the suite's channel, after its hello, is closed at the suite's end", func(w *gotest.T) {
		w.It("interrupts the suite's process group with SIGINT and runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.Equal(it, 130, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Markers, "suite interrupt")
			gotest.Contains(it, run.Markers, "child SIGINT")
			gotest.True(it, run.MarkerOrder("context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", run.Markers)
			relayVerdicts(it, &run)
		})
	})
}
