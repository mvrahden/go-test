package main_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// cliRunner is the run's shared gotest binary, run from the repo root.
type cliRunner struct {
	binary   string
	repoRoot string
}

// newCLIRunner wraps the shared binary and scrubs this process's Actions
// env, which every child CLI inherits. Call it from BeforeAll. The CI and
// snapshot switches stay: this process's snapshots read them, and cliEnv
// keeps them from the children.
func newCLIRunner(cli *gotestcli.BinarySharedFixture) cliRunner {
	testkit.ScrubActionsEnv()
	childTempDir = cli.TmpDir
	return cliRunner{binary: cli.Binary, repoRoot: cli.RepoRoot}
}

// childTempDir is the run's temp dir for child CLIs, set by newCLIRunner.
var childTempDir string

// cliEnviron is this process's environment minus the GitHub Actions
// variables and the CI and snapshot switches, so a child CLI sees only the
// flags its case gives it. Its temp dir is the run's own, so its sweep
// never reaches the real one's dirs.
func cliEnviron() []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "CI", "GOTEST_CI", "GOTEST_UPDATE_SNAPSHOTS", "GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY", "GITHUB_WORKSPACE":
			continue
		}
		// Windows spells these in any case.
		if childTempDir != "" && slices.ContainsFunc([]string{"TMPDIR", "TMP", "TEMP"}, func(v string) bool { return strings.EqualFold(v, name) }) {
			continue
		}
		env = append(env, kv)
	}
	if childTempDir != "" {
		env = append(env, "TMPDIR="+childTempDir, "TMP="+childTempDir, "TEMP="+childTempDir)
	}
	return env
}

// cliEnv is cliEnviron for a CLI started in dir, then extra. It carries PWD
// itself: exec sets it from cmd.Dir only for a nil cmd.Env.
func cliEnv(dir string, extra ...string) []string {
	return append(append(cliEnviron(), "PWD="+dir), extra...)
}

// run returns the binary's combined stdout+stderr output.
func (r cliRunner) run(t *gotest.T, args ...string) string {
	out, _ := r.runExit(t, args...)
	return out
}

// runExit returns the combined output along with the exit code.
func (r cliRunner) runExit(t *gotest.T, args ...string) (string, int) {
	return r.runEnv(t, nil, args...)
}

// runEnv is runExit with extra environment entries.
func (r cliRunner) runEnv(t *gotest.T, env []string, args ...string) (string, int) {
	return runGotestIn(t, r.binary, r.repoRoot, env, args...)
}

// runSplit returns stdout and stderr apart, with the exit code: what a
// command renders and what it complains about are different claims.
func (r cliRunner) runSplit(t *gotest.T, args ...string) (stdout, stderr string, code int) {
	return r.runSplitIn(t, r.repoRoot, args...)
}

// runSplitIn is runSplit from dir.
func (r cliRunner) runSplitIn(t *gotest.T, dir string, args ...string) (stdout, stderr string, code int) {
	cmd := exec.Command(r.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	cmd.Env = cliEnv(dir)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exitErr *exec.ExitError
	gotest.True(t, err == nil || errors.As(err, &exitErr), "running gotest binary: %v\n%s", err, errs.String())
	return out.String(), errs.String(), cmd.ProcessState.ExitCode()
}

// specTableEntries extracts `name` from rows shaped `| `name` | ... |` within the
// doc section starting at heading until the next heading of the same level.
func specTableEntries(doc, heading string) map[string]bool {
	return specTableEntriesUntil(doc, heading, "\n### ")
}

// specTableEntriesUntil is specTableEntries with an explicit section
// terminator, for sections delimited by a different heading level.
func specTableEntriesUntil(doc, heading, next string) map[string]bool {
	start := strings.Index(doc, heading)
	if start < 0 {
		return nil
	}
	section := doc[start:]
	if end := strings.Index(section[len(heading):], next); end >= 0 {
		section = section[:len(heading)+end]
	}
	entries := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `([^`=<]+)").FindAllStringSubmatch(section, -1) {
		entries[strings.TrimSpace(m[1])] = true
	}
	return entries
}

// stageFixtureModule writes one fixture source into its own module under
// dir, with a go.work that pairs it with the checkout under test. Staging
// inside the repository (examples/ used to host it) leaks into the editor's
// discovery snapshot: the extension watches the tree and indexes the package
// before the test removes it.
func stageFixtureModule(t *gotest.T, repoRoot, dir, filename string, src []byte) string {
	gotest.NoError(t, testkit.WriteModule(repoRoot, dir, "testpkg"))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, filename), src, 0o600))
	return dir
}
