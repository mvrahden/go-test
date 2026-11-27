package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditRecordTestSuite: a record says what a tree's suites declare, read
// from source, the same way every time.
type AuditRecordTestSuite struct {
	dir  string // the staged trees
	head string
	run  testkit.Run
	file string
}

func (s *AuditRecordTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *AuditRecordTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-record-")
	gotest.NoError(t, err)
	s.dir = dir
	s.head = stage(t, dir, "head")
	s.file = filepath.Join(dir, "head.json")
	s.run = record(t, s.head, s.file)
}

func (s *AuditRecordTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditRecordTestSuite) TestTheTreeRecord(t *gotest.T) {
	t.When("audit --record runs over a module", func(w *gotest.T) {
		w.It("prints one document opening with version and schema that lists each suite, test, When, It and row with its display path, module-relative file and lines and body hash, and exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.run.Exit, "stderr: %s", s.run.Stderr)
			data, err := os.ReadFile(s.file)
			gotest.NoError(it, err)
			gotest.Regexp(it, `^\{"version":"[^"]+","schema":1,`, string(data))
			rec := readRecord(it, s.file)
			gotest.Len(it, rec.Packages, 1)
			gotest.Equal(it, "shop", rec.Packages[0].Dir)
			got := flat(&rec)
			type where struct {
				Kind      string
				File      string
				Line, End int
			}
			for path, want := range map[string]where{
				"CartTestSuite / Add": {"test", "shop/cart_suite_test.go", 15, 33},
				"CartTestSuite / Add / when both numbers are positive":                     {"when", "shop/cart_suite_test.go", 16, 27},
				"CartTestSuite / Add / when both numbers are positive / returns their sum": {"it", "shop/cart_suite_test.go", 17, 20},
				"StockTestSuite / Count / when the stock is empty / reports zero":          {"it", "shop/stock_suite_test.go", 31, 33},
				"StockTestSuite / Count / large numbers":                                   {"row", "shop/stock_suite_test.go", 40, 40},
			} {
				b, ok := got[path]
				gotest.True(it, ok, "the record lacks %s", path)
				gotest.Equal(it, want, where{b.Kind, b.File, b.Line, b.EndLine}, path)
			}
			base := filepath.Join(s.dir, "base.json")
			gotest.Equal(it, 0, record(it, stage(it, s.dir, "base"), base).Exit)
			before := flatOf(it, base)
			const order, negative = "CartTestSuite / Add / when both numbers are positive / ", "CartTestSuite / Add / when one number is negative / returns the difference"
			gotest.Regexp(it, `^[0-9a-f]{16}$`, got[order+"ignores the order"].Hash)
			gotest.Equal(it, before[order+"keeps the order irrelevant"].Hash, got[order+"ignores the order"].Hash, "layout and comments changed the hash")
			gotest.NotEqual(it, before[negative].Hash, got[negative].Hash)
		})
		w.It("counts the gotest assertions written in each body, and marks AssertsBy, F_ and X_", func(it *gotest.T) {
			got := flatOf(it, s.file)
			sum := got["CartTestSuite / Add / when both numbers are positive / returns their sum"]
			gotest.Equal(it, 2, sum.Assertions)
			rounds := got["CartTestSuite / Rounds"]
			gotest.Equal(it, 1, rounds.Assertions)
			gotest.True(it, rounds.AssertsBy)
			gotest.False(it, sum.AssertsBy)
			gotest.True(it, got["CartTestSuite / Total"].Focused)
			gotest.True(it, got["CartTestSuite / Empty"].Excluded)
			gotest.False(it, got["CartTestSuite / Total"].Excluded)
			gotest.Equal(it, "F_TestTotal", got["CartTestSuite / Total"].Name)
		})
		w.It("names each suite's budgets and guard, each suppression with its rules and reason, and each snapshot file with its hash", func(it *gotest.T) {
			rec := readRecord(it, s.file)
			type suite struct {
				Name, Timeout, SetupTimeout string
				Guard                       bool
			}
			var suites []suite
			for _, st := range rec.Packages[0].Suites {
				suites = append(suites, suite{st.Name, st.Budget.Timeout, st.Budget.SetupTimeout, st.Guard})
			}
			gotest.Equal(it, []suite{
				{"CartTestSuite", "1m0s", "30s", false},
				{"StockTestSuite", "30s", "none", true},
			}, suites)
			type supp struct {
				File   string
				Line   int
				Rules  string
				Reason string
			}
			var supps []supp
			for _, sp := range rec.Suppressions {
				supps = append(supps, supp{sp.File, sp.Line, strings.Join(sp.Rules, ","), sp.Reason})
			}
			gotest.Equal(it, []supp{
				{"shop/plain_test.go", 5, "stdlib-test", "a plain stdlib test"},
				{"shop/stock_suite_test.go", 48, "stdlib-test", "kept for go test"},
				{"shop/stock_suite_test.go", 55, "stdlib-test", "a second stdlib test"},
			}, supps)
			const snap = "shop/testdata/__snapshots__/TestCartTestSuite.snap"
			gotest.Len(it, rec.Snapshots, 1)
			gotest.Equal(it, snap, rec.Snapshots[0].File)
			gotest.Equal(it, fixtureSum(it, "head", snap), rec.Snapshots[0].SHA256)
		})
	})
	t.When("the same tree is recorded twice", func(w *gotest.T) {
		w.It("the two records are byte for byte equal and hold no absolute path", func(it *gotest.T) {
			again := filepath.Join(s.dir, "again.json")
			gotest.Equal(it, 0, record(it, s.head, again).Exit)
			first, err := os.ReadFile(s.file)
			gotest.NoError(it, err)
			second, err := os.ReadFile(again)
			gotest.NoError(it, err)
			gotest.Equal(it, string(first), string(second))
			root, err := testkit.RepoRoot()
			gotest.NoError(it, err)
			gotest.NotContains(it, string(first), s.dir)
			gotest.NotContains(it, string(first), root)
			gotest.NotContains(it, string(first), filepath.ToSlash(os.TempDir()))
		})
	})
}

func (s *AuditRecordTestSuite) TestATreeThatDoesNotCompile(t *gotest.T) {
	dir := t.TempDir()
	head := stage(t, dir, "head")
	gotest.NoError(t, os.WriteFile(filepath.Join(head, "shop", "broken_test.go"), []byte("package shop\n\nvar _ int = \"not a number\"\n"), 0o600))
	file := filepath.Join(dir, "head.json")
	run := record(t, head, file)
	base := filepath.Join(dir, "base.json")
	gotest.Equal(t, 0, record(t, stage(t, dir, "base"), base).Exit)
	compared, err := testkit.RunCLI(t.Context(), head, "audit", "--base-record="+base, "--format=json")
	gotest.NoError(t, err)

	t.When("the head tree holds a package that does not compile", func(w *gotest.T) {
		w.It("audit --record lists the package with its errors and no suites and exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stderr: %s", run.Stderr)
			rec := readRecord(it, file)
			gotest.Len(it, rec.Packages, 1)
			gotest.Equal(it, "auditmod/shop", rec.Packages[0].Path)
			gotest.Empty(it, rec.Packages[0].Suites)
			gotest.NotEmpty(it, rec.Packages[0].Unreadable)
			gotest.Contains(it, strings.Join(rec.Packages[0].Unreadable, "\n"), "shop/broken_test.go:3")
			gotest.NotContains(it, strings.Join(rec.Packages[0].Unreadable, "\n"), dir)
		})
		w.It("audit compares none of its behaviors, names it as not compared and exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, compared.Exit, "stderr: %s", compared.Stderr)
			doc := readDoc(it, compared.Stdout)
			gotest.Empty(it, doc.Behaviors)
			for _, f := range doc.Findings {
				gotest.NotEqual(it, "removed", f.Class, "%+v", f)
			}
			gotest.Len(it, doc.Incomplete, 1)
			gotest.Regexp(it, `^auditmod/shop: the head cannot be read: `, doc.Incomplete[0])
		})
	})
}
