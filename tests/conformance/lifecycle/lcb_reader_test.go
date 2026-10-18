//go:build linux || darwin

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// StreamReaderTestSuite: a suite under the runner whose stream nobody reads
// any more stops and tears down by itself; outside the runner it does not.
type StreamReaderTestSuite struct {
	cli    string
	plain  string // the reader package's test binary, built by plain go test
	staged string
}

func (s *StreamReaderTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// BeforeAll builds the CLI, and the reader package's test binary the way go
// test builds it, from a module of its own with its harness generated on disk.
func (s *StreamReaderTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	s.staged = t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "lcbreader"), s.staged, "example.com/reader")
	gotest.NoError(t, err)
	generate := exec.Command(s.cli, "generate", "./lcbreader/") //nolint:gosec // G204: binary built by this suite
	generate.Dir = s.staged
	out, err := generate.CombinedOutput()
	gotest.NoError(t, err, string(out))
	s.plain = filepath.Join(s.staged, "reader.test")
	build := exec.Command("go", "test", "-c", "-o", s.plain, "./lcbreader/")
	build.Dir = s.staged
	out, err = build.CombinedOutput()
	gotest.NoError(t, err, string(out))
}

// AfterAll has nothing to release: the binaries and the module live in temp dirs.
func (s *StreamReaderTestSuite) AfterAll(t *gotest.T) {}

func (s *StreamReaderTestSuite) TestTheRunnerAndConverterDie(t *gotest.T) {
	markerDir := t.TempDir()
	// The wrapper stays between the converter and the suite, and outlives both.
	wrap := wrapper(t, t.TempDir(), `echo "converter=$PPID" >> "$GOTEST_CANARY_DIR/markers"
"$@"
`)
	cmd := commandOf(s.cli, "-json", "-exec", wrap, "../testdata/stopping/")
	cmd.Env = testkit.ChildEnv(markerDir)
	tree := started(t, cmd, markerDir, "running", "converter=", "pid=")
	converter, err := strconv.Atoi(markerValue(awaitMarkers(markerDir, 0), "converter"))
	gotest.NoError(t, err)
	signalled(t, cmd, tree, syscall.Kill(cmd.Process.Pid, syscall.SIGKILL))
	signalled(t, cmd, tree, syscall.Kill(converter, syscall.SIGKILL))
	run := ended(cmd, tree, markerDir, 30*time.Second)
	markers := awaitMarkers(markerDir, 60*time.Second, "fixture.AfterAll")
	pid, _ := strconv.Atoi(markerValue(markers, "pid"))

	t.When("the runner and its converter are killed under -json while an -exec wrapper waits on the suite", func(w *gotest.T) {
		w.It("ends the holding method's context", func(it *gotest.T) {
			gotest.Contains(it, markers, "context ended", "stderr: %s", run.Stderr)
		})
		w.It("the methods that have not started never run", func(it *gotest.T) {
			gotest.NotContains(it, markers, "second")
			gotest.NotContains(it, markers, "third")
		})
		w.It("runs AfterAll before the fixture is released", func(it *gotest.T) {
			gotest.True(it, testkit.MarkerOrderOf(markers, "context ended", "suite.AfterAll", "fixture.AfterAll"), "markers: %v", markers)
		})
		w.It("leaves no suite process behind", func(it *gotest.T) {
			gotest.NotZero(it, pid, "markers: %v", markers)
			gotest.True(it, gone(pid), "the suite process %d outlived the run", pid)
		})
	})
}

func (s *StreamReaderTestSuite) TestAPlainRunLosesItsReader(t *gotest.T) {
	markerDir := t.TempDir()
	r, w, err := os.Pipe()
	gotest.NoError(t, err)
	cmd := exec.Command(s.plain, "-test.run=^TestReaderTestSuite$") //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(markerDir)
	cmd.Stdout = w
	gotest.NoError(t, cmd.Start())
	// Nobody reads the stream from here on.
	_ = w.Close()
	_ = r.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	markers, _ := testkit.ReadMarkers(filepath.Join(markerDir, testkit.MarkersFile))

	t.When("a suite run by its test binary outside the gotest runner loses the reader of its stdout", func(w *gotest.T) {
		w.It("the holding method runs on: its context does not end", func(it *gotest.T) {
			gotest.Contains(it, markers, "not stopped")
			gotest.NotContains(it, markers, "context ended")
		})
	})
}
