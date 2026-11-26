package lint_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinterSuppressionsTestSuite: a nolint comment says why it is there, and
// lint lists every one for an auditor.
type LinterSuppressionsTestSuite struct{}

func (s *LinterSuppressionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

const reasonPkg = "../testdata/lintergreason/"

const noReason = "nolint comment gives no reason; write one after it, as //nolint:<rule> // <reason>"

func (s *LinterSuppressionsTestSuite) TestReasons(t *gotest.T) {
	t.When("a nolint comment naming a rule of lint's gives no reason", func(w *gotest.T) {
		w.It("lint reports the comment under nolint-reason at the comment and still honours its suppression", func(it *gotest.T) {
			gotest.Equal(it, []string{"nolint-reason nolint_test.go:5:36: " + noReason}, onLine(ruleFindings(it, ".", reasonPkg), "nolint_test.go:5:"))
		})
		w.It("the comment cannot suppress that finding, even naming nolint-reason", func(it *gotest.T) {
			gotest.Equal(it, []string{"nolint-reason nolint_test.go:7:34: " + noReason}, onLine(ruleFindings(it, ".", reasonPkg), "nolint_test.go:7:"))
		})
	})
	t.When("a nolint comment gives a reason after its rules", func(w *gotest.T) {
		w.It("lint leaves the comment unreported, bare or naming rules", func(it *gotest.T) {
			got := ruleFindings(it, ".", reasonPkg)
			gotest.Empty(it, onLine(got, "nolint_test.go:9:"))
			gotest.Empty(it, onLine(got, "nolint_test.go:11:"))
			gotest.Empty(it, onLine(got, "a_test.go:5:"))
		})
	})
	t.When("a nolint comment naming only another linter's rule gives no reason", func(w *gotest.T) {
		w.It("lint leaves the comment without a reason to that linter", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"stdlib-test nolint_test.go:13:1: stdlib test TestForeign — consider using a gotest suite",
			}, onLine(ruleFindings(it, ".", reasonPkg), "nolint_test.go:13:"))
		})
	})
}

// suppression is one entry of the lint -suppressions document.
type suppression struct {
	Module     string   `json:"module"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Col        int      `json:"col"`
	Rules      []string `json:"rules"`
	Reason     string   `json:"reason"`
	Suppressed bool     `json:"suppressed"`
}

func (s *LinterSuppressionsTestSuite) TestList(t *gotest.T) {
	const module, dir = "github.com/mvrahden/go-test", "tests/conformance/testdata/lintergreason/"
	list := func(t *gotest.T) (testkit.Run, []suppression) {
		run, err := testkit.RunCLI(t.Context(), ".", "lint", "-suppressions", reasonPkg)
		gotest.NoError(t, err)
		var doc struct {
			Suppressions []suppression `json:"suppressions"`
		}
		gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
		return run, doc.Suppressions
	}
	t.When("lint -suppressions runs over a package with nolint comments", func(w *gotest.T) {
		w.It("prints one document opening with version that lists each comment reaching a rule of lint's once, sorted by file and position, with its module, module-relative file, rules and reason, and exits 0", func(it *gotest.T) {
			run, got := list(it)
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `^\{"version":"[^"]+","suppressions":\[`, run.Stdout)
			type entry struct {
				Module, File string
				Line, Col    int
				Rules        []string
				Reason       string
			}
			var entries []entry
			for _, s := range got {
				entries = append(entries, entry{s.Module, s.File, s.Line, s.Col, s.Rules, s.Reason})
			}
			gotest.Equal(it, []entry{
				{module, dir + "a_test.go", 5, 33, []string{"stdlib-test"}, "sorts before nolint_test.go"},
				{module, dir + "nolint_test.go", 5, 36, []string{"stdlib-test"}, ""},
				{module, dir + "nolint_test.go", 7, 34, []string{"nolint-reason", "stdlib-test"}, ""},
				{module, dir + "nolint_test.go", 9, 36, []string{"stdlib-test"}, "kept as a stdlib test"},
				{module, dir + "nolint_test.go", 11, 32, []string{"all"}, "kept as a stdlib test"},
				{module, dir + "nolint_test.go", 15, 1, []string{"stdlib-test", "testify"}, "nothing below needs it"},
			}, entries)
		})
		w.It("marks as suppressed exactly the comments that suppressed a finding", func(it *gotest.T) {
			_, got := list(it)
			var suppressed []bool
			for _, s := range got {
				suppressed = append(suppressed, s.Suppressed)
			}
			gotest.Equal(it, []bool{true, true, true, true, true, false}, suppressed)
		})
		w.It("leaves out a comment naming only another linter's rule, and prints nothing on stderr", func(it *gotest.T) {
			run, got := list(it)
			for _, s := range got {
				gotest.NotEqual(it, 13, s.Line, "the gosec comment is listed: %+v", s)
			}
			gotest.Empty(it, run.Stderr)
		})
	})
	t.When("lint -suppressions is given with -json or -fix", func(w *gotest.T) {
		w.It("lint refuses the pair with exit 2, naming the other flag", func(it *gotest.T) {
			for _, other := range []string{"-json", "-fix"} {
				run, err := testkit.RunCLI(it.Context(), ".", "lint", "-suppressions", other, reasonPkg)
				gotest.NoError(it, err)
				gotest.Equal(it, 2, run.Exit, "stderr: %s", run.Stderr)
				gotest.Empty(it, run.Stdout)
				gotest.Equal(it, "FAIL: -suppressions prints its own document; it does not take "+other+"\n", run.Stderr)
			}
		})
	})
}
