package main_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// WatchCLITestSuite drives "gotest watch" through the built binary over an
// absolute path: the first cycle ends with a watch-end event and the
// interrupt ends the command with the interrupt's exit code.
// Sequential: it spawns a watch and waits on a signal.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type WatchCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *WatchCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline}
}

func (s *WatchCLITestSuite) BeforeAll(t *gotest.T) { s.cli = newCLIRunner(s.CLI) }

func (s *WatchCLITestSuite) TestWatchOverAnAbsolutePath(t *gotest.T) {
	pkg, err := filepath.Abs(filepath.Join(s.cli.repoRoot, "tests", "conformance", "testdata", "passing"))
	gotest.NoError(t, err)
	cmd := exec.Command(s.cli.binary, "watch", "-json", pkg) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.cli.repoRoot
	cmd.Env = append(cliEnviron(), "GOTEST_CI=0")
	var stderr lockedBuilder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)

	seen := make(chan string, 64)
	wait := gotest.Go(t, func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			seen <- sc.Text()
		}
		close(seen)
	})
	var start, end string
	deadline := time.After(2 * time.Minute)
	for end == "" {
		select {
		case line, ok := <-seen:
			gotest.True(t, ok, "watch ended before its first cycle completed; stderr: %s", stderr.String())
			switch {
			case strings.Contains(line, `"Action":"watch-start"`):
				start = line
			case strings.Contains(line, `"Action":"watch-end"`):
				end = line
			}
		case <-deadline:
			_ = tree.Kill()
			gotest.Fail(t, "no watch-end within two minutes; stderr: %s", stderr.String())
		}
	}

	gotest.NoError(t, testkit.Interrupt(cmd, tree))
	err = cmd.Wait()
	tree.Release()
	wait()

	t.When("the first cycle over an absolute path completes", func(w *gotest.T) {
		w.It("announced the cycle's start and end with a green exit", func(it *gotest.T) {
			var started struct {
				Package string `json:"Package"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(start), &started), "watch-start line: %s", start)
			gotest.Equal(it, pkg, started.Package)
			gotest.Contains(it, end, `"Exit":0`)
		})
	})
	t.When("the watch is interrupted", func(w *gotest.T) {
		w.It("exits 130", func(it *gotest.T) {
			var exitErr *exec.ExitError
			gotest.True(it, errors.As(err, &exitErr), "watch exited 0 on interrupt")
			gotest.Equal(it, 130, exitErr.ExitCode())
		})
	})
}

// lockedBuilder is a strings.Builder a command may write while the test reads it.
type lockedBuilder struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuilder) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuilder) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
