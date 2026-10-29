package cli_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLINewcomerTestSuite: the first things a newcomer meets say what to do
// next, and a broken config never hides the help that explains it.
type CLINewcomerTestSuite struct{}

func (s *CLINewcomerTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLINewcomerTestSuite) TestABrokenConfigStillHelps(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, ".gotest.yml"), []byte("lint: [not: valid\n"), 0o600))
	t.When(".gotest.yml does not parse", func(w *gotest.T) {
		w.It("still prints version and help, and a run reports the config error with exit 2", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "version still prints", args: []string{"version"}, exit: 0, stdoutHas: "gotest"},
				{Desc: "help config still prints", args: []string{"help", "config"}, exit: 0, stdoutHas: ".gotest.yml"},
				{Desc: "a run reports the config error", args: []string{"./..."}, exit: 2, stderrHas: ".gotest.yml"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), dir, row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.stdoutHas != "" {
					gotest.Contains(sub, run.Stdout, row.stdoutHas)
				}
				if row.stderrHas != "" {
					gotest.Contains(sub, run.Stderr, row.stderrHas)
				}
			}
		})
	})
}

func (s *CLINewcomerTestSuite) TestAMistypedCommandIsSuggested(t *gotest.T) {
	t.When("the first word is almost a subcommand or is a help topic", func(w *gotest.T) {
		w.It("suggests the command meant, and runs any other word as a package pattern", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "sumary suggests summary", args: []string{"sumary", "../testdata/passing/"}, exit: 2, stderrHas: `did you mean "summary"`},
				{Desc: "test suggests dropping the word", args: []string{"test", "../testdata/passing/"}, exit: 2, stderrHas: "gotest ../testdata/passing/"},
				{Desc: "a package named like nothing runs as a pattern", args: []string{"../testdata/passing/"}, exit: 0},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				if row.stderrHas != "" {
					gotest.Contains(sub, run.Stderr, row.stderrHas)
				}
			}
		})
	})
}

func (s *CLINewcomerTestSuite) TestAModuleWithoutGotestIsToldHow(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fresh\n\ngo 1.26.0\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "fresh_suite_test.go"), []byte(`package fresh_test

import "github.com/mvrahden/go-test/pkg/gotest"

type FreshTestSuite struct{}

func (s *FreshTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }
`), 0o600))
	t.When("a suite imports gotest in a module that does not require it", func(w *gotest.T) {
		w.It("names the go get -tool command to run", func(it *gotest.T) {
			run, err := testkit.RunCLIWithEnv(it.Context(), dir, []string{"GOWORK=off", "GOFLAGS="}, ".")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "go get -tool github.com/mvrahden/go-test/cmd/gotest")
		})
	})
}

func (s *CLINewcomerTestSuite) TestStdlibTestsAreNotedInEveryMode(t *gotest.T) {
	t.When("a package has only stdlib tests", func(w *gotest.T) {
		w.It("notes them on stderr in the plain, -json, summary and spec modes", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []flagRow{
				{Desc: "the plain run notes them on stderr", args: []string{"../testdata/stdlibonly/"}, exit: 0, stderrHas: "stdlib test"},
				{Desc: "the -json run notes them on stderr", args: []string{"-json", "../testdata/stdlibonly/"}, exit: 0, stderrHas: "stdlib test"},
				{Desc: "summary notes them on stderr", args: []string{"summary", "../testdata/stdlibonly/"}, exit: 0, stderrHas: "stdlib test"},
				{Desc: "spec notes them on stderr", args: []string{"spec", "../testdata/stdlibonly/"}, exit: 0, stderrHas: "stdlib test"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, row.exit, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(sub, run.Stderr, row.stderrHas)
			}
		})
	})
}
