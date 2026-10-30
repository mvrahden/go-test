package lint_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterRulesTestSuite: a rule judges the code it is about, reports a misuse
// however it is reached, and its fix loses nothing the source said.
type LinterRulesTestSuite struct{}

func (s *LinterRulesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const rulesPkg = "../testdata/lintrules/"

func (s *LinterRulesTestSuite) TestTestifySuites(t *gotest.T) {
	t.When("a package holds a testify suite", func(w *gotest.T) {
		w.It("gotest's suite rules and generation-rejects leave a testify suite to the testify rule", func(it *gotest.T) {
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			dir := it.TempDir()
			gotest.NoError(it, testkit.StageModule(root, filepath.Join("..", "testdata", "lintruletestify"), dir))

			got := ruleFindings(it, dir, "./store/")
			gotest.Contains(it, got, "testify store_test.go:6:2: testify import github.com/stretchr/testify/suite — consider migrating to gotest")
			var others []string
			for _, f := range got {
				if !strings.HasPrefix(f, "testify ") {
					others = append(others, f)
				}
			}
			gotest.Empty(it, others)
		})
	})
}

func (s *LinterRulesTestSuite) TestOwnTMethod(t *gotest.T) {
	t.When("a type of its own has a T method", func(w *gotest.T) {
		w.It("lint leaves calls through that T method unreported", func(it *gotest.T) {
			var escapes []string
			for _, f := range ruleFindings(it, ".", rulesPkg) {
				if strings.HasPrefix(f, "t-escape ") {
					escapes = append(escapes, f)
				}
			}
			gotest.Empty(it, escapes)
		})
	})
}

func (s *LinterRulesTestSuite) TestShadowedGotest(t *gotest.T) {
	t.When("a local variable shadows the gotest import", func(w *gotest.T) {
		w.It("fail-guard leaves a guard in the local's scope unreported and reports one where gotest names the package", func(it *gotest.T) {
			var guards []string
			for _, f := range ruleFindings(it, ".", rulesPkg) {
				if strings.HasPrefix(f, "fail-guard ") {
					guards = append(guards, f)
				}
			}
			gotest.Equal(it, []string{"fail-guard rules_suite_test.go:59:2: use NoError instead of if+FailNow for error nil check"}, guards)
		})
	})
}

func (s *LinterRulesTestSuite) TestPollHelpers(t *gotest.T) {
	t.When("a polling callback hands the test's t to a helper", func(w *gotest.T) {
		w.It("lint reports the outer t passed to a helper inside a polling callback", func(it *gotest.T) {
			gotest.Contains(it, ruleFindings(it, ".", rulesPkg),
				"poll-scope rules_suite_test.go:28:9: t passed to ready in poll callback bypasses assertion recording — use poll")
		})
	})
}

func (s *LinterRulesTestSuite) TestLenMessage(t *gotest.T) {
	t.When("True asserts len(x) != 0", func(w *gotest.T) {
		w.It("lint names the len != 0 check in its message", func(it *gotest.T) {
			gotest.Contains(it, ruleFindings(it, ".", rulesPkg),
				"assertion-simplify rules_suite_test.go:34:2: use NotEmpty instead of True for len != 0 check")
		})
	})
}

func (s *LinterRulesTestSuite) TestFixesKeepText(t *gotest.T) {
	t.When("lint -fix rewrites assertions that carry messages and comments", func(w *gotest.T) {
		w.It("the stronger assertion takes a removed one's message and its comment stays", func(it *gotest.T) {
			gotest.Regexp(it, `\t// the load must fail\n\tgotest\.ErrorContains\(t, err, "boom", "loading config"\)\n`, fixedRules(it))
		})
		w.It("a rewritten assertion keeps the comment between its arguments", func(it *gotest.T) {
			gotest.Regexp(it, `\t/\* same seed \*/\n\tgotest\.Equal\(t, a, b, "seeded"\)\n`, fixedRules(it))
		})
	})
}

// fixedRules is the rules fixture's source after lint -fix on a staged copy.
func fixedRules(t *gotest.T) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "lintrules"), t.TempDir(), "lintrulesmod")
	gotest.NoError(t, err)
	fix, err := testkit.RunCLI(t.Context(), dir, "lint", "-fix", ".")
	gotest.NoError(t, err)
	gotest.NotEqual(t, 2, fix.Exit, "stderr: %s", fix.Stderr)
	src, err := os.ReadFile(filepath.Join(dir, "rules_suite_test.go"))
	gotest.NoError(t, err)
	return string(src)
}

// ruleFindings lints pkg from dir with -json and returns each gotest finding
// as "<rule> <file>:<line>:<col>: <message>".
func ruleFindings(t *gotest.T, dir, pkg string) []string {
	run, err := testkit.RunCLI(t.Context(), dir, "lint", "-json", pkg)
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
	gotest.NotEmpty(t, got, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	return got
}

// LintRulesTestSuite: lint names its rules without linting anything, as the
// help lists them and as a document a program reads.
type LintRulesTestSuite struct{}

func (s *LintRulesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

// helpRules is the rule list of gotest help lint, and the rules its -skip-<rule>
// flag names as skippable.
func helpRules(t *gotest.T) (table string, skippable []string) {
	run, err := testkit.RunCLI(t.Context(), ".", "help", "lint")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	_, rest, ok := strings.Cut(run.Stdout, "Rules:\n")
	gotest.True(t, ok, "help lint has no Rules: section:\n%s", run.Stdout)
	table, _, ok = strings.Cut(rest, "\n\n")
	gotest.True(t, ok)
	list := regexp.MustCompile(`(?s)-skip-<rule>.*?\((.*?)\)`).FindStringSubmatch(run.Stdout)
	gotest.Len(t, list, 2, "help lint names no skippable rules")
	for _, id := range strings.Split(list[1], ",") {
		skippable = append(skippable, strings.TrimSpace(id))
	}
	return table + "\n", skippable
}

func (s *LintRulesTestSuite) TestTheRules(t *gotest.T) {
	t.When("the rules are asked for with lint -rules", func(w *gotest.T) {
		w.It("prints the rule list gotest help lint prints and exits 0, with no package to lint", func(it *gotest.T) {
			table, _ := helpRules(it)
			// A directory outside any module: there is nothing to lint.
			run, err := testkit.RunCLI(it.Context(), it.TempDir(), "lint", "-rules")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Equal(it, table, run.Stdout)
		})
		w.It("with -json prints one document opening with version that lists every rule once with its tier, scope, skippable, fixable and description", func(it *gotest.T) {
			table, skippable := helpRules(it)
			run, err := testkit.RunCLI(it.Context(), it.TempDir(), "lint", "-rules", "-json")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^\{"version":"[^"]+","rules":\[`, run.Stdout)
			var doc struct {
				Rules []struct {
					ID          string `json:"id"`
					Tier        string `json:"tier"`
					Scope       string `json:"scope"`
					Skippable   *bool  `json:"skippable"`
					Fixable     *bool  `json:"fixable"`
					Description string `json:"description"`
				} `json:"rules"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc))
			var ids, skippableIDs []string
			helpText := strings.Join(strings.Fields(table), " ")
			for _, r := range doc.Rules {
				ids = append(ids, r.ID)
				gotest.Contains(it, []string{"integrity", "expressiveness", "migration"}, r.Tier, "rule %s", r.ID)
				gotest.NotEmpty(it, r.Scope, "rule %s", r.ID)
				gotest.NotZero(it, r.Skippable, "rule %s has no skippable", r.ID)
				gotest.NotZero(it, r.Fixable, "rule %s has no fixable", r.ID)
				gotest.Equal(it, r.Tier != "integrity", *r.Skippable, "rule %s: only integrity rules are not skippable", r.ID)
				if *r.Skippable {
					skippableIDs = append(skippableIDs, r.ID)
				}
				gotest.NotContains(it, r.Description, "\n")
				gotest.Contains(it, helpText, r.ID+" "+r.Description, "the help does not describe %s so", r.ID)
			}
			var listed []string
			for _, line := range strings.Split(strings.TrimSuffix(table, "\n"), "\n") {
				if !strings.HasPrefix(line, "    ") {
					listed = append(listed, strings.Fields(line)[0])
				}
			}
			gotest.Equal(it, listed, ids)
			unique := slices.Clone(ids)
			slices.Sort(unique)
			gotest.Len(it, slices.Compact(unique), len(ids), "a rule is listed twice")
			slices.Sort(skippable)
			slices.Sort(skippableIDs)
			gotest.Equal(it, skippable, skippableIDs)
		})
	})
}
