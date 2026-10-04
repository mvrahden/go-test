package canary_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// TestMainTestSuite runs a generated fixture package under plain go test,
// where every suite shares one process and nothing but m.Run returning says
// the last of them is done.
type TestMainTestSuite struct {
	binary string
}

func (s *TestMainTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *TestMainTestSuite) BeforeAll(t *gotest.T) {
	root, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	s.binary, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	out, err := exec.Command(s.binary, "generate", "./testdata/testmain/").CombinedOutput() //nolint:gosec // G204: controlled binary with fixed args
	gotest.NoError(t, err, string(out))
}

func (s *TestMainTestSuite) AfterAll(t *gotest.T) {
	generated, _ := filepath.Glob("testdata/testmain/gotest_p*suite_test.go")
	for _, f := range generated {
		_ = os.Remove(f)
	}
}

// goTest runs go test on the package and returns its exit code, its output and
// the fixture's setup and teardown lines.
func goTest(t *gotest.T, env []string, args ...string) (code int, out string, events []string) {
	log := filepath.Join(t.TempDir(), "events")
	cmd := exec.Command("go", append(append([]string{"test", "-count=1"}, args...), "./testdata/testmain/")...) //nolint:gosec // G204: fixed args
	cmd.Env = append(append(os.Environ(), "TESTMAIN_LOG="+log), env...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		exit := gotest.ErrorAs[*exec.ExitError](t, err)
		code = exit.ExitCode()
	}
	if b, err := os.ReadFile(log); err == nil {
		events = strings.Fields(string(b))
	}
	return code, string(raw), events
}

func count(events []string, kind string) int {
	n := 0
	for _, e := range events {
		if e == kind {
			n++
		}
	}
	return n
}

// Each variant of the package owns its fixture: two setups, two teardowns,
// each teardown after every suite of its variant ran.
func (s *TestMainTestSuite) TestEverySuiteSeesTheFixtureUntilTheLastHasRun(t *gotest.T) {
	for _, flags := range [][]string{nil, {"-count=2", "-shuffle=on"}} {
		code, out, events := goTest(t, nil, flags...)
		gotest.Equal(t, 0, code, "go test %v\n%s", flags, out)
		gotest.Equal(t, 2, count(events, "setup"), "go test %v: %q", flags, events)
		gotest.Equal(t, 2, count(events, "teardown"), "go test %v: %q", flags, events)
		gotest.Equal(t, "setup", events[0], "go test %v: %q", flags, events)
		gotest.Equal(t, "teardown", events[len(events)-1], "go test %v: %q", flags, events)
	}
}

func (s *TestMainTestSuite) TestARunThatSelectsNoBoundSuiteSetsNothingUp(t *gotest.T) {
	for _, flags := range [][]string{{"-run", "TestUnboundTestSuite"}, {"-list", "."}} {
		code, out, events := goTest(t, nil, flags...)
		gotest.Equal(t, 0, code, "go test %v\n%s", flags, out)
		gotest.Empty(t, events, "go test %v", flags)
	}
}

// testing ends the process after a panicking test's cleanups; m.Run never
// returns, so the fixture is released in those cleanups.
func (s *TestMainTestSuite) TestAPanickingTestStillReleasesTheFixture(t *gotest.T) {
	code, out, events := goTest(t, []string{"TESTMAIN_PANIC=1"}, "-run", "TestPanickingTestSuite")
	gotest.NotEqual(t, 0, code, "exit 0 after a panic\n%s", out)
	gotest.Contains(t, out, "panic: method panic")
	gotest.Equal(t, []string{"setup", "teardown"}, events)
}

// The failure belongs to the package: no test passed only to be failed for
// what a fixture did after it.
func (s *TestMainTestSuite) TestATeardownFailureFailsThePackage(t *gotest.T) {
	code, out, _ := goTest(t, []string{"TESTMAIN_FAIL_TEARDOWN=1"}, "-v")
	gotest.Equal(t, 1, code, out)
	gotest.Contains(t, out, "\nFAIL: fixture teardown failed\n")
	gotest.NotContains(t, out, "--- FAIL", "a test was failed for the fixture's teardown")
}
