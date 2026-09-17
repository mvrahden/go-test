package main_test

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// PrepareCLITestSuite drives "gotest prepare" through the built binary: it
// starts the shared fixtures, prints one JSON line an editor can act on,
// blocks, and tears down on the shutdown signal.
// Sequential: it spawns a full prepare and waits on a signal.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type PrepareCLITestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *PrepareCLITestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Timeout = 3 * time.Minute
	return cfg
}

func (s *PrepareCLITestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type prepareOutput struct {
	OverlayFile string `json:"overlayFile"`
	Dir         string `json:"dir"`
	StateFile   string `json:"stateFile"`
}

func (s *PrepareCLITestSuite) TestPrepare(t *gotest.T) {
	cmd := exec.Command(s.cli.binary, "prepare", "./tests/sharedfixture/standalone/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.cli.repoRoot
	cmd.Env = append(os.Environ(), "GOTEST_CI=0")
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	tree, err := testkit.StartCLI(cmd)
	gotest.NoError(t, err)

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
		_ = tree.Kill()
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

	gotest.NoError(t, testkit.Interrupt(cmd, tree))
	err = cmd.Wait()
	tree.Release()
	wait()

	t.It("tears down and exits 0 on the shutdown signal", func(it *gotest.T) {
		gotest.NoError(it, err)
	})
}
