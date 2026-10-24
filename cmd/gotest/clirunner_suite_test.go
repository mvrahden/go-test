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

func (s *CLIRunnerTestSuite) AfterEach(t *gotest.T) {
	childTempDir = ""
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

// A child CLI's temp dir is the run's own, under every name for it: its
// sweep never reaches the host's.
func (s *CLIRunnerTestSuite) TestAChildCLISeesTheRunsTempDir(t *gotest.T) {
	runTmp, dir := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", "/host/tmpdir")
	t.Setenv("Tmp", "/host/tmp") // Windows spells it in any case
	newCLIRunner(&gotestcli.BinarySharedFixture{TmpDir: runTmp})

	temp := map[string][]string{}
	for _, kv := range cliEnv(dir) {
		name, value, _ := strings.Cut(kv, "=")
		if slices.ContainsFunc([]string{"TMPDIR", "TMP", "TEMP"}, func(v string) bool { return strings.EqualFold(v, name) }) {
			temp[name] = append(temp[name], value)
		}
	}
	gotest.Equal(t, map[string][]string{"TMPDIR": {runTmp}, "TMP": {runTmp}, "TEMP": {runTmp}}, temp)
}
