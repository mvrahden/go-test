package main_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/config"
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
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline}
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
	// The document opens with the version the same binary reports: a git
	// checkout build carries a VCS pseudo-version, so the test process's own
	// resolved version is not the one to compare against.
	info, err := exec.Command(s.cli.binary, "version").Output() //nolint:gosec // G204: controlled binary with fixed args
	gotest.NoError(t, err)
	fields := strings.Fields(string(info))
	gotest.GreaterOrEqual(t, len(fields), 2, "gotest version output: %q", info)
	gotest.Regexp(t, `^\{"version":"`+regexp.QuoteMeta(fields[1])+`",`, line)

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

// A fixture whose AfterAll fails leaves what it held behind; prepare must say
// so with its exit code, as a run does.
func (s *PrepareCLITestSuite) TestPrepareReportsATeardownFailure(t *gotest.T) {
	cmd := exec.Command(s.cli.binary, "prepare", "./tests/canary/testdata/teardownfailing/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.cli.repoRoot
	cmd.Env = append(os.Environ(), "GOTEST_CI=0")
	var stderr strings.Builder
	cmd.Stderr = &stderr
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
	select {
	case <-lines:
	case <-time.After(2 * time.Minute):
		_ = tree.Kill()
		gotest.Fail(t, "prepare printed nothing within two minutes")
	}

	gotest.NoError(t, testkit.Interrupt(cmd, tree))
	err = cmd.Wait()
	tree.Release()
	wait()

	t.When("the shared fixture's AfterAll fails on shutdown", func(w *gotest.T) {
		w.It("exits 1 and names the teardown failure on stderr", func(it *gotest.T) {
			var exitErr *exec.ExitError
			gotest.True(it, errors.As(err, &exitErr), "prepare exited 0 although the teardown failed; stderr: %s", stderr.String())
			gotest.Equal(it, 1, exitErr.ExitCode())
			gotest.Contains(it, stderr.String(), "would not release")
		})
	})
}

// PrepareOptionsTestSuite: prepare builds the same overlay a run would, so
// the project's fuzz.harvest setting decides seed harvesting here too.
type PrepareOptionsTestSuite struct{}

func (s *PrepareOptionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *PrepareOptionsTestSuite) TestHarvestFollowsTheConfig(t *gotest.T) {
	off := false
	t.When("the config says nothing", func(w *gotest.T) {
		w.It("harvests", func(it *gotest.T) {
			gotest.True(it, ExportPrepareHarvestSeeds(config.ProjectConfig{}))
		})
	})
	t.When("fuzz.harvest is false", func(w *gotest.T) {
		w.It("does not harvest", func(it *gotest.T) {
			gotest.False(it, ExportPrepareHarvestSeeds(config.ProjectConfig{Fuzz: config.FuzzConfig{Harvest: &off}}))
		})
	})
}
