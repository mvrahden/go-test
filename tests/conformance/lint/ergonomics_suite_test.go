package lint_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterErgonomicsTestSuite: nolint comments say what they mean, a finding
// is about what the repository holds, and a refused flag says why.
type LinterErgonomicsTestSuite struct{}

func (s *LinterErgonomicsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const nolintPkg = "../testdata/lintergnolint/"

func (s *LinterErgonomicsTestSuite) TestNolint(t *gotest.T) {
	t.When("a nolint comment names no rule", func(w *gotest.T) {
		w.It("a bare nolint comment suppresses every rule on its line", func(it *gotest.T) {
			gotest.Empty(it, onLine(ruleFindings(it, ".", nolintPkg), "nolint_test.go:5:"))
		})
	})
	t.When("a nolint comment names all", func(w *gotest.T) {
		w.It("nolint:all suppresses every rule on its line", func(it *gotest.T) {
			gotest.Empty(it, onLine(ruleFindings(it, ".", nolintPkg), "nolint_test.go:7:"))
		})
	})
	t.When("a nolint comment names a rule lint does not have", func(w *gotest.T) {
		w.It("a nolint comment naming an unknown rule is a finding", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{
				"nolint-unknown nolint_test.go:9:35: unknown rule not-a-rule in nolint comment",
				"stdlib-test nolint_test.go:9:1: stdlib test TestUnknown — consider using a gotest suite",
			}, onLine(ruleFindings(it, ".", nolintPkg), "nolint_test.go:9:"))
		})
		w.It("the comment naming the unknown rule cannot suppress that finding", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{
				"nolint-unknown nolint_test.go:11:41: unknown rule not-a-rule in nolint comment",
			}, onLine(ruleFindings(it, ".", nolintPkg), "nolint_test.go:11:"))
		})
	})
	t.When("a nolint comment names another linter's rule", func(w *gotest.T) {
		w.It("lint leaves the rule to that linter and suppresses none of its own", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{
				"stdlib-test nolint_test.go:13:1: stdlib test TestForeign — consider using a gotest suite",
			}, onLine(ruleFindings(it, ".", nolintPkg), "nolint_test.go:13:"))
		})
	})
}

func (s *LinterErgonomicsTestSuite) TestGeneratedFile(t *gotest.T) {
	const finding = "generated-file gotest_psuite_test.go:1:1: generated file gotest_psuite_test.go should not be checked into version control"
	t.When("a generated file is on disk", func(w *gotest.T) {
		w.It("an untracked generated file is not a finding", func(it *gotest.T) {
			dir := stageGenerated(it, "init")
			gotest.NotContains(it, generatedFindings(it, dir), finding)
		})
		w.It("a generated file git tracks is a finding", func(it *gotest.T) {
			dir := stageGenerated(it, "init", "add")
			gotest.Contains(it, generatedFindings(it, dir), finding)
		})
		w.It("outside a git repository a generated file is a finding", func(it *gotest.T) {
			dir := stageGenerated(it)
			gotest.Contains(it, generatedFindings(it, dir), finding)
		})
		w.It("where git cannot read its repository, the finding says git cannot tell whether it is tracked", func(it *gotest.T) {
			dir := stageGenerated(it, "init")
			index := filepath.Join(filepath.Dir(dir), ".git", "index")
			gotest.NoError(it, os.WriteFile(index, []byte("not an index"), 0o600))
			got := generatedFindings(it, dir)
			gotest.NotContains(it, got, finding)
			gotest.Len(it, onLine(got, "gotest_psuite_test.go:1:1"), 1, got)
			gotest.Regexp(it, `^generated-file gotest_psuite_test\.go:1:1: generated file gotest_psuite_test\.go may be checked into version control: git cannot tell \(.*index`, onLine(got, "gotest_psuite_test.go:1:1")[0])
		})
	})
}

func (s *LinterErgonomicsTestSuite) TestSkipFlags(t *gotest.T) {
	t.When("the command line skips an integrity rule", func(w *gotest.T) {
		w.It("skipping an integrity rule on the command line says why it cannot be skipped and exits 2", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "lint", "-skip-focus", nolintPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, "FAIL: integrity lint rule in -skip-focus: \"focus\" can only be suppressed per line with //nolint\n", run.Stderr)
		})
	})
}

// onLine is the findings whose position starts with fileLine.
func onLine(findings []string, fileLine string) []string {
	var got []string
	for _, f := range findings {
		if _, pos, _ := strings.Cut(f, " "); strings.HasPrefix(pos, fileLine) {
			got = append(got, f)
		}
	}
	return got
}

// stageGenerated stages the lintergen package beside a generated file and
// runs each git subcommand on it: "init" makes the module a repository, "add"
// stages the generated file. It returns the package directory.
func stageGenerated(t *gotest.T, git ...string) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "lintergen"), t.TempDir(), "lintergenmod")
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(filepath.Join(pkg, "gotest_psuite_test.go"), []byte("package lintergen\n"), 0o600))
	for _, sub := range git {
		args := []string{sub}
		if sub == "add" {
			args = append(args, "gotest_psuite_test.go")
		}
		cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // G204: git with the suite's own arguments
		cmd.Dir = filepath.Dir(pkg)
		if sub == "add" {
			cmd.Dir = pkg
		}
		out, err := cmd.CombinedOutput()
		gotest.NoError(t, err, "git %s: %s", sub, out)
	}
	return pkg
}

// generatedFindings lints the staged package with git kept from looking
// above its module, as "<rule> <file>:<line>:<col>: <message>".
func generatedFindings(t *gotest.T, pkg string) []string {
	ceiling := "GIT_CEILING_DIRECTORIES=" + filepath.Dir(filepath.Dir(pkg))
	run, err := testkit.RunCLIWithEnv(t.Context(), pkg, []string{ceiling}, "lint", "-json", ".")
	gotest.NoError(t, err)
	var byPkg map[string]map[string][]struct {
		Posn     string `json:"posn"`
		Message  string `json:"message"`
		Category string `json:"category"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &byPkg), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	var got []string
	for _, analyzers := range byPkg {
		for _, d := range analyzers["gotestlint"] {
			got = append(got, d.Category+" "+filepath.Base(d.Posn)+": "+d.Message)
		}
	}
	gotest.Contains(t, got, "stdlib-test plain_test.go:5:1: stdlib test TestPlain — consider using a gotest suite", "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	return got
}
