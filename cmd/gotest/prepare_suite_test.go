package main_test

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PrepareCLITestSuite drives "gotest prepare" through the built binary: it
// starts the shared fixtures, prints one JSON line an editor can act on,
// blocks, and tears down on the shutdown signal. Sequential: it spawns a
// full prepare and waits on a signal.
//
//nolint:lifecycle-pair // BeforeAll's binary lives under t.TempDir(), which the framework removes automatically
type PrepareCLITestSuite struct {
	binary   string
	repoRoot string
}

func (s *PrepareCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *PrepareCLITestSuite) BeforeAll(t *gotest.T) {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.repoRoot = absRoot
	s.binary = buildGotestBinary(t, absRoot, t.TempDir())
}

type prepareOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile"`
}

func (s *PrepareCLITestSuite) TestPrepare(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("no way to deliver a shutdown signal to a child process on Windows")
	}
	cmd := exec.Command(s.binary, "prepare", "./tests/sharedfixture/standalone/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.repoRoot
	cmd.Env = append(os.Environ(), "GOTEST_CI=0")
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	gotest.NoError(t, cmd.Start())

	lines := make(chan string, 1)
	wait := gotest.Go(t, func() {
		sc := bufio.NewScanner(stdout)
		if sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	})

	var line string
	select {
	case line = <-lines:
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		gotest.Fail(t, "prepare printed nothing within two minutes")
	}

	var out prepareOutput
	gotest.NoError(t, json.Unmarshal([]byte(line), &out), "line: %s", line)

	t.It("names the overlay, the work dir and a state file that exists while it blocks", func(it *gotest.T) {
		gotest.NotEmpty(it, out.OverlayFile)
		gotest.NotEmpty(it, out.Dir)
		gotest.NotEmpty(it, out.StateFile)
		_, err := os.Stat(out.StateFile)
		gotest.NoError(it, err)
	})

	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
	err = cmd.Wait()
	wait()

	t.It("tears down and exits 0 on the shutdown signal", func(it *gotest.T) {
		gotest.NoError(it, err)
	})
}
