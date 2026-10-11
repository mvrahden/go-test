package main_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// ExitZeroTestSuite drives suites whose methods call os.Exit(0): under go
// test that fails the run, so it must under every gotest mode too.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type ExitZeroTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *ExitZeroTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ExitZeroTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

const exitZeroSrc = `package testpkg

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExitTestSuite struct{}

func (s *ExitTestSuite) TestExits(t *gotest.T)       { os.Exit(0) }
func (s *ExitTestSuite) TestPasses(t *gotest.T)      {}
func (s *ExitTestSuite) BenchmarkExits(b *gotest.B) { os.Exit(0) }
`

const exitZeroPanic = "panic: unexpected call to os.Exit(0) during test"

func (s *ExitZeroTestSuite) TestOsExitZeroFailsTheRun(t *gotest.T) {
	dir := stageFixtureModule(t, s.cli.repoRoot, t.TempDir(), "exit_suite_test.go", []byte(exitZeroSrc))

	t.When("the run prints text", func(w *gotest.T) {
		stdout, stderr, code := s.cli.runSplitIn(w, dir, "./")
		gotest.NotEqual(w, 0, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		gotest.Contains(w, stderr, exitZeroPanic)
		gotest.Regexp(w, `(?m)^FAIL\s+testpkg`, stdout)
		gotest.NotContains(w, stdout, "ok  \ttestpkg")
	})

	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"json", []string{"-json", "./"}},
		{"spec", []string{"spec", "--no-color", "./"}},
		{"summary", []string{"summary", "--no-color", "./"}},
	}) {
		stdout, stderr, code := s.cli.runSplitIn(t, dir, tc.args...)
		gotest.Equal(t, 1, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		gotest.Contains(t, stdout, exitZeroPanic)
	}

	t.When("the run benchmarks", func(w *gotest.T) {
		stdout, stderr, code := s.cli.runSplitIn(w, dir, "bench", "./", "-benchtime=1x")
		gotest.NotEqual(w, 0, code, "stdout:\n%s\nstderr:\n%s", stdout, stderr)
		gotest.Contains(w, stderr, exitZeroPanic)
	})
}
