package shutdown_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ShutdownTestSuite proves that a run which ends early still tears its
// shared fixtures down: an interrupt exits 130 and a FailFast trip skips the
// remaining methods, and the fixture's AfterAll runs either way. Exclusive:
// it spawns full CLI runs and times a signal.
// Sequential: each test times a full CLI run against the wall clock.
//
//nolint:lifecycle-pair // BeforeAll's binary and module live under t.TempDir(), which the framework removes
type ShutdownTestSuite struct {
	binary string
	module string
}

func (s *ShutdownTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	cfg.Timeout = 2 * time.Minute
	cfg.SetupTimeout = 2 * time.Minute
	return cfg
}

func (s *ShutdownTestSuite) BeforeAll(t *gotest.T) {
	repoRoot, err := filepath.Abs("../../..")
	gotest.NoError(t, err)

	name := "gotest"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	s.binary = filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", s.binary, "./cmd/gotest") //nolint:gosec // G204: go tool with controlled arguments
	build.Dir = repoRoot
	out, err := build.CombinedOutput()
	gotest.NoError(t, err, "build gotest: %s", out)

	s.module = stageModule(t, repoRoot, filepath.Join(repoRoot, "tests", "sharedfixture", "testdata", "shutdownmod"), t.TempDir())
}

// stageModule copies the fixture module into dir, points its replace
// directive at the checkout under test, and pairs the two in a go.work.
func stageModule(t *gotest.T, repoRoot, src, dir string) string {
	gotest.NoError(t, filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		data, err := os.ReadFile(path) //nolint:gosec // G122: copies the repo's own fixture into a fresh temp dir
		if err != nil {
			return err
		}
		if d.Name() == "go.mod" {
			data = []byte(strings.ReplaceAll(string(data), "REPO_ROOT", repoRoot))
		}
		return os.WriteFile(target, data, 0o600)
	}))
	work := "go 1.25.0\n\nuse (\n\t.\n\t" + repoRoot + "\n)\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.work"), []byte(work), 0o600))
	return dir
}

// start launches the CLI over one package of the staged module with the
// marker directory in its environment.
func (s *ShutdownTestSuite) start(t *gotest.T, markers, pkg string) *exec.Cmd {
	return s.startWith(t, markers, nil, "./"+pkg+"/")
}

// startWith launches the CLI with the given arguments and extra environment.
func (s *ShutdownTestSuite) startWith(t *gotest.T, markers string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.module
	cmd.Env = append(append(os.Environ(), "GOTEST_SHUTDOWN_DIR="+markers, "GOTEST_CI=0"), env...)
	gotest.NoError(t, cmd.Start())
	return cmd
}

// interruptWhen sends an interrupt to cmd once the marker file appears.
func interruptWhen(t *gotest.T, cmd *exec.Cmd, markers, name string) {
	gotest.Eventually(t, 90*time.Second, 100*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, marker(markers, name), "%s never appeared", name)
	})
	gotest.NoError(t, cmd.Process.Signal(os.Interrupt))
}

func exitCode(t *gotest.T, cmd *exec.Cmd) int {
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	gotest.True(t, errors.As(err, &exitErr), "wait: %v", err)
	return exitErr.ExitCode()
}

func marker(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func (s *ShutdownTestSuite) TestInterrupt(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("no way to deliver an interrupt to a child process on Windows")
	}
	markers := t.TempDir()
	cmd := s.start(t, markers, "slow")
	interruptWhen(t, cmd, markers, "test-running")
	code := exitCode(t, cmd)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-up"), "the fixture never started")
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

func (s *ShutdownTestSuite) TestFailFastTrip(t *gotest.T) {
	markers := t.TempDir()
	code := exitCode(t, s.start(t, markers, "failfast"))

	t.It("exits 1 and skips the methods after the failure", func(it *gotest.T) {
		gotest.Equal(it, 1, code)
		gotest.False(it, marker(markers, "second-ran"), "the method after the failure ran")
	})
	t.It("tears the shared fixture down", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

// The spec, summary and bench commands run the batch pipeline, not the
// streaming one; an interrupt must mean 130 on both.
func (s *ShutdownTestSuite) TestInterruptDuringSpec(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("no way to deliver an interrupt to a child process on Windows")
	}
	markers := t.TempDir()
	cmd := s.startWith(t, markers, nil, "spec", "./slow/")
	interruptWhen(t, cmd, markers, "test-running")
	code := exitCode(t, cmd)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

// Once every verdict is in, an interrupt has nothing left to cut short: a
// green run that is interrupted while its fixture tears down is still green.
func (s *ShutdownTestSuite) TestInterruptDuringTeardown(t *gotest.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("no way to deliver an interrupt to a child process on Windows")
	}
	markers := t.TempDir()
	cmd := s.startWith(t, markers, []string{"GOTEST_SHUTDOWN_TEARDOWN_SLEEP=3s"}, "./quick/")
	interruptWhen(t, cmd, markers, "fixture-tearing-down")
	code := exitCode(t, cmd)

	t.It("keeps the suites' verdict", func(it *gotest.T) {
		gotest.Equal(it, 0, code)
	})
	t.It("lets the teardown finish", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not finish")
	})
}
