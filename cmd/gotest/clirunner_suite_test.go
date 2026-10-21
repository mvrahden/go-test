package main_test

import (
	"os"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// CLIRunnerTestSuite pins where the CI and snapshot switches are scrubbed:
// from the child CLIs only. This process keeps them, since its own
// snapshots read them.
// Sequential: Setenv.
type CLIRunnerTestSuite struct{}

var cliSwitches = []string{"CI", "GOTEST_CI", "GOTEST_UPDATE_SNAPSHOTS"}

func (s *CLIRunnerTestSuite) BeforeEach(t *gotest.T) {
	for _, name := range cliSwitches {
		t.Setenv(name, "1")
	}
	newCLIRunner(&gotestcli.BinarySharedFixture{})
}

func (s *CLIRunnerTestSuite) TestThisProcessKeepsItsSwitches(t *gotest.T) {
	for _, name := range cliSwitches {
		gotest.Equal(t, "1", os.Getenv(name), "%s after newCLIRunner", name)
	}
}

func (s *CLIRunnerTestSuite) TestAChildCLISeesNone(t *gotest.T) {
	dir := t.TempDir()
	env := cliEnv(dir, "EXTRA=1")
	var inherited []string
	for _, kv := range env {
		if name, _, _ := strings.Cut(kv, "="); slices.Contains(cliSwitches, name) {
			inherited = append(inherited, name)
		}
	}
	gotest.Empty(t, inherited, "the child would inherit them")
	gotest.Contains(t, env, "PWD="+dir)
	gotest.Equal(t, "EXTRA=1", env[len(env)-1])
}
