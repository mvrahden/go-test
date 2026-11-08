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

const escapesPkg = "../testdata/lintescapes/"

func (s *LinterRulesTestSuite) TestWrapperEscapes(t *gotest.T) {
	t.When("a test calls Name, Setenv and TempDir through t.T()", func(w *gotest.T) {
		w.It("lint reports each as a t-escape with a fix", func(it *gotest.T) {
			_, fixable := lintFindings(it, ".", escapesPkg)
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:67:6: Name is available on gotest.T — unnecessary T escape")
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:68:2: Setenv is available on gotest.T — unnecessary T escape")
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:69:6: TempDir is available on gotest.T — unnecessary T escape")
		})
	})
	t.When("a benchmark calls Loop, Helper, Cleanup, Setenv and TempDir through b.B() and reads b.B().N", func(w *gotest.T) {
		w.It("lint reports Loop as a t-escape with a fix, Helper as a t-escape without one and Cleanup as suite-lifecycle, and leaves N unreported", func(it *gotest.T) {
			got, fixable := lintFindings(it, ".", escapesPkg)
			loop := "t-escape escapes_suite_test.go:18:6: Loop is available on gotest.B — unnecessary B escape"
			helper := "t-escape escapes_suite_test.go:21:2: never call Helper — gotest resolves call sites automatically; Helper degrades failure locations"
			gotest.Contains(it, fixable, loop)
			gotest.Contains(it, got, helper)
			gotest.NotContains(it, fixable, helper)
			gotest.Contains(it, got, "suite-lifecycle escapes_suite_test.go:22:2: use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle")
			gotest.Empty(it, findingsAt(got, "escapes_suite_test.go:23:"))
		})
		w.It("lint reports Setenv and TempDir as t-escapes with a fix", func(it *gotest.T) {
			_, fixable := lintFindings(it, ".", escapesPkg)
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:24:2: Setenv is available on gotest.B — unnecessary B escape")
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:25:6: TempDir is available on gotest.B — unnecessary B escape")
		})
	})
	t.When("a fuzz target seeds through f.F().Add and calls TempDir through f.F()", func(w *gotest.T) {
		w.It("lint reports the seed as a t-escape with a fix and fuzz-seed stays silent", func(it *gotest.T) {
			got, fixable := lintFindings(it, ".", escapesPkg)
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:45:2: Add is available on gotest.F — f.F().Add skips the seed buffer: a struct seed panics inside testing, and the Add-after-Fuzz check is bypassed")
			gotest.Empty(it, findingsOf(got, "fuzz-seed "))
		})
		w.It("lint reports TempDir as a t-escape with a fix", func(it *gotest.T) {
			_, fixable := lintFindings(it, ".", escapesPkg)
			gotest.Contains(it, fixable, "t-escape escapes_suite_test.go:46:6: TempDir is available on gotest.F — unnecessary F escape")
		})
	})
	t.When("a benchmark loops with b.B().Loop() and sleeps inside it", func(w *gotest.T) {
		w.It("bench-loop stays silent and bench-wait reports the sleep", func(it *gotest.T) {
			got := ruleFindings(it, ".", escapesPkg)
			gotest.Empty(it, findingsOf(got, "bench-loop "))
			gotest.Contains(it, got, "bench-wait escapes_suite_test.go:30:3: benchmark EscapesTestSuite.BenchmarkSleepThroughEscape calls time.Sleep inside the measured loop — this times the wait, not the code; move it outside the loop")
		})
	})
}

func (s *LinterRulesTestSuite) TestOwnBMethod(t *gotest.T) {
	t.When("a type of its own has a B method", func(w *gotest.T) {
		w.It("lint leaves calls through that B method unreported", func(it *gotest.T) {
			got := ruleFindings(it, ".", escapesPkg)
			gotest.Empty(it, findingsAt(got, "escapes_suite_test.go:36:"))
			gotest.Empty(it, findingsAt(got, "escapes_suite_test.go:37:"))
			gotest.Empty(it, findingsAt(got, "escapes_suite_test.go:59:"))
		})
	})
}

// findingsOf is the findings of got under the rule prefix.
func findingsOf(got []string, prefix string) []string {
	var of []string
	for _, f := range got {
		if strings.HasPrefix(f, prefix) {
			of = append(of, f)
		}
	}
	return of
}

// findingsAt is the findings of got at a "<file>:<line>:" position.
func findingsAt(got []string, at string) []string {
	var of []string
	for _, f := range got {
		if _, rest, ok := strings.Cut(f, " "); ok && strings.HasPrefix(rest, at) {
			of = append(of, f)
		}
	}
	return of
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
	got, _ := lintFindings(t, dir, pkg)
	return got
}

// lintFindings is ruleFindings, and of those the ones that carry a fix.
func lintFindings(t *gotest.T, dir, pkg string) (got, fixable []string) {
	run, err := testkit.RunCLI(t.Context(), dir, "lint", "-json", pkg)
	gotest.NoError(t, err)
	var byPkg map[string]map[string][]struct {
		Posn     string            `json:"posn"`
		Message  string            `json:"message"`
		Category string            `json:"category"`
		Fixes    []json.RawMessage `json:"suggested_fixes"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &byPkg), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	for _, analyzers := range byPkg {
		for _, d := range analyzers["gotestlint"] {
			f := d.Category + " " + filepath.Base(d.Posn) + ": " + d.Message
			got = append(got, f)
			if len(d.Fixes) > 0 {
				fixable = append(fixable, f)
			}
		}
	}
	gotest.NotEmpty(t, got, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	return got, fixable
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
