package main_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// cliRunner is the built gotest binary, run from the repo root. Suites that
// drive a subcommand end to end build one in BeforeAll.
type cliRunner struct {
	binary   string
	repoRoot string
}

func newCLIRunner(t *gotest.T) cliRunner {
	absRoot, err := filepath.Abs("../..")
	gotest.NoError(t, err)
	scrubActionsEnv()
	return cliRunner{binary: buildGotestBinary(t, absRoot, t.TempDir()), repoRoot: absRoot}
}

// scrubActionsEnv removes the GitHub Actions variables from this process.
// Every child CLI inherits its environment; under CI each one would
// otherwise append to the job's real step summary.
func scrubActionsEnv() {
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("GITHUB_STEP_SUMMARY")
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

// runEnv is runExit with extra environment entries. A nil env keeps
// cmd.Env nil, which is when exec sets the child's PWD from cmd.Dir; an
// explicit env has to carry that itself.
func (r cliRunner) runEnv(t *gotest.T, env []string, args ...string) (string, int) {
	return runGotestIn(t, r.binary, r.repoRoot, env, args...)
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
	goMod := "module testpkg\n\ngo 1.25.0\n\nrequire github.com/mvrahden/go-test v0.0.0-00010101000000-000000000000\n\nreplace github.com/mvrahden/go-test => " + repoRoot + "\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600))
	work := "go 1.25.0\n\nuse (\n\t.\n\t" + repoRoot + "\n)\n"
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.work"), []byte(work), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, filename), src, 0o600))
	return dir
}
