package spec_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecAssertionsTestSuite: the spec says which leaves passed without an
// assertion, and which declared how they assert instead.
type SpecAssertionsTestSuite struct{}

func (s *SpecAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second, Parallel: true}
}

const (
	countPkg = "../testdata/assertcount/"
	countRed = "../testdata/assertcountred/"
)

func (s *SpecAssertionsTestSuite) TestLeavesThatPassWithAndWithoutAssertions(t *gotest.T) {
	doc := specJSON(t, 1, countPkg, countRed)
	green := packageOf(t, doc, "assertcount")
	red := packageOf(t, doc, "assertcountred")
	t.When("a run holds leaves that pass with assertions, pass with none and fail with none", func(w *gotest.T) {
		w.It("the JSON spec gives each leaf its count and marks only the leaves that passed with none vacuous", func(it *gotest.T) {
			two := node(it, green.Nodes, "TestCountTestSuite", "TestTwo")
			gotest.Equal(it, 2, *two.Assertions)
			gotest.False(it, two.Vacuous)
			nothing := node(it, green.Nodes, "TestCountTestSuite", "TestNothing")
			gotest.Equal(it, 0, *nothing.Assertions)
			gotest.True(it, nothing.Vacuous)
			fails := node(it, red.Nodes, "TestFailTestSuite", "TestFails")
			gotest.Equal(it, 0, *fails.Assertions)
			gotest.False(it, fails.Vacuous)
			panics := node(it, red.Nodes, "TestPanicTestSuite", "TestPanics")
			gotest.False(it, panics.Vacuous)
			skipped := node(it, green.Nodes, "TestCountTestSuite", "TestSkipped")
			gotest.Zero(it, skipped.Assertions)
			gotest.False(it, skipped.Vacuous)
			gotest.False(it, node(it, green.Nodes, "TestCountTestSuite", "TestBehaviors").Vacuous)
		})
		w.It("stats count the vacuous leaves", func(it *gotest.T) {
			gotest.Equal(it, 5, doc.Stats.Vacuous)
		})
	})
}

func (s *SpecAssertionsTestSuite) TestAVacuousLeafInTheReports(t *gotest.T) {
	t.When("a run holds a vacuous leaf", func(w *gotest.T) {
		w.It("the terminal spec marks it VACUOUS on its row and under Needs attention", func(it *gotest.T) {
			run := spec(it, 0, countPkg)
			gotest.Regexp(it, `(?m)^  ✓ Nothing — VACUOUS: passed without an assertion`, run.Stdout)
			gotest.Regexp(it, `(?m)^Needs attention\n(.*\n)*✓ .*Count › Nothing — VACUOUS: passed without an assertion`, run.Stdout)
		})
		w.It("the Markdown spec marks it VACUOUS on its row and under Needs attention", func(it *gotest.T) {
			md := spec(it, 0, "--format=md", countPkg)
			gotest.Contains(it, md.Stdout, "- PASS **Nothing** — VACUOUS: passed without an assertion")
			gotest.Regexp(it, `- \*\*VACUOUS\*\* .*Count › Nothing\n`, md.Stdout)
		})
		w.It("the summary names it", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "summary", "--no-color", countPkg)
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `VACUOUS  \S+assertcount Count / Nothing — passed without an assertion`, run.Stdout)
			gotest.Contains(it, run.Stdout, "5 vacuous")
		})
	})
}

func (s *SpecAssertionsTestSuite) TestAStreamThatHoldsNoCount(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), countPkg)
	gotest.NoError(t, err)
	var kept []string
	for line := range strings.Lines(run.Stdout) {
		if !strings.Contains(line, `"Action":"attr"`) {
			kept = append(kept, line)
		}
	}
	saved := filepath.Join(t.TempDir(), "older.json")
	gotest.NoError(t, os.WriteFile(saved, []byte(strings.Join(kept, "")), 0o600))
	t.When("a saved stream's leaves carry no count, as a runtime's that reports none", func(w *gotest.T) {
		w.It("the JSON spec gives no leaf a count and marks none vacuous", func(it *gotest.T) {
			doc := specJSON(it, 0, "--input", saved)
			nothing := node(it, packageOf(it, doc, "assertcount").Nodes, "TestCountTestSuite", "TestNothing")
			gotest.Zero(it, nothing.Assertions)
			gotest.False(it, nothing.Vacuous)
			gotest.Zero(it, doc.Stats.Vacuous)
		})
	})
}

func (s *SpecAssertionsTestSuite) TestALeafThatDeclaredHowItAsserts(t *gotest.T) {
	t.When("a run holds leaves that declared how they assert", func(w *gotest.T) {
		w.It("the JSON spec carries each reason and stats count them apart from the vacuous ones", func(it *gotest.T) {
			doc := specJSON(it, 0, countPkg)
			declared := node(it, packageOf(it, doc, "assertcount").Nodes, "TestCountTestSuite", "TestDeclared")
			gotest.Equal(it, "the call must not panic", declared.AssertsBy)
			gotest.False(it, declared.Vacuous)
			gotest.Equal(it, 2, doc.Stats.AssertsBy)
			gotest.Equal(it, 5, doc.Stats.Vacuous)
		})
		w.It("the terminal spec shows the reason on the row", func(it *gotest.T) {
			run := spec(it, 0, countPkg)
			gotest.Regexp(it, `(?m)^  ✓ Declared — ASSERTS BY: the call must not panic`, run.Stdout)
		})
	})
}

func (s *SpecAssertionsTestSuite) TestAGuardedAndAnExcludedSuite(t *gotest.T) {
	t.When("a run holds a suite its guard kept from running and an excluded suite", func(w *gotest.T) {
		doc := specJSON(w, 0, "../testdata/declaredguard/", "../testdata/e2eexcluded/")
		w.It("the JSON spec gives each its skip reason", func(it *gotest.T) {
			guarded := node(it, packageOf(it, doc, "declaredguard").Nodes, "TestGuardedTestSuite")
			gotest.Equal(it, "suite guard: the guard keeps this suite from running", guarded.SkipReason)
			excluded := node(it, packageOf(it, doc, "e2eexcluded").Nodes, "TestX_RetiredTestSuite")
			gotest.Equal(it, "test suite was excluded by user", excluded.SkipReason)
		})
		w.It("stats count the excluded suite as excluded", func(it *gotest.T) {
			gotest.Equal(it, 1, doc.Stats.Excluded)
		})
	})
}

// packageOf is the document's package whose path ends in name.
func packageOf(t *gotest.T, doc specDoc, name string) specPackage { //nolint:gocritic // hugeParam: a test helper
	for _, p := range doc.Packages {
		if strings.HasSuffix(p.Path, "/"+name) {
			return p
		}
	}
	gotest.Fail(t, "no package "+name)
	return specPackage{}
}
