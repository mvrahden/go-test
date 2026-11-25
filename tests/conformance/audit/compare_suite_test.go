package audit_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditCompareTestSuite: the audit of a head against a base lists what the
// suites declare now against before, and what got weaker.
type AuditCompareTestSuite struct {
	dir            string
	head           string
	baseFile       string
	headFile       string
	json, text, md testkit.Run
}

func (s *AuditCompareTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 60 * time.Second}
}

func (s *AuditCompareTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-compare-")
	gotest.NoError(t, err)
	s.dir = dir
	s.baseFile, s.headFile = filepath.Join(dir, "base.json"), filepath.Join(dir, "head.json")
	gotest.Equal(t, 0, record(t, stage(t, dir, "base"), s.baseFile).Exit)
	s.head = stage(t, dir, "head")
	gotest.Equal(t, 0, record(t, s.head, s.headFile).Exit)
	s.json = s.audit(t, "--format=json")
	s.text = s.audit(t)
	s.md = s.audit(t, "--format=md")
}

func (s *AuditCompareTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

// audit compares the two records with args.
func (s *AuditCompareTestSuite) audit(t *gotest.T, args ...string) testkit.Run {
	run, err := testkit.RunCLI(t.Context(), s.dir, append([]string{"audit", "--base-record=" + s.baseFile, "--head-record=" + s.headFile}, args...)...)
	gotest.NoError(t, err)
	return run
}

func (s *AuditCompareTestSuite) TestBehaviorsCompared(t *gotest.T) {
	t.When("audit compares a base record with a head record that adds, removes, rewords, moves and changes behaviors", func(w *gotest.T) {
		w.It("lists each as added, removed, reworded, moved or changed with its base and head file and line", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			type row struct{ Change, Base, Head string }
			at := func(p *place) string {
				if p == nil {
					return ""
				}
				return strings.Join(p.Path, " / ") + " @ " + p.File + ":" + strconv.Itoa(p.Line)
			}
			var got []row
			for _, c := range doc.Behaviors {
				got = append(got, row{c.Change, at(c.Base), at(c.Head)})
			}
			const cart, stock = "shop/cart_suite_test.go:", "shop/stock_suite_test.go:"
			gotest.Equal(it, []row{
				{"removed", "Cart / Copies / first copy @ " + cart + "50", ""},
				{"removed", "Cart / Copies / second copy @ " + cart + "53", ""},
				{"removed", "Cart / Remove @ " + cart + "32", ""},
				{"added", "", "Cart / Copies / the only copy @ " + cart + "49"},
				{"added", "", "Stock / Count / when the stock is empty @ " + stock + "30"},
				{"added", "", "Stock / Count / when the stock is refilled @ " + stock + "25"},
				{"reworded", "Cart / Add / when both numbers are positive / keeps the order irrelevant @ " + cart + "21", "Cart / Add / when both numbers are positive / ignores the order @ " + cart + "21"},
				{"moved", "Stock / Count / when the stock is full / counts every item @ " + stock + "15", "Stock / Count / when the stock is refilled / counts every item @ " + stock + "26"},
				{"changed", "Cart / Add / when one number is negative / returns the difference @ " + cart + "26", "Cart / Add / when one number is negative / returns the difference @ " + cart + "29"},
				{"changed", "Cart / Rounds @ " + cart + "44", "Cart / Rounds @ " + cart + "43"},
			}, got)
		})
	})
	t.When("two removed behaviors have the body of one added behavior", func(w *gotest.T) {
		w.It("lists both as removed and the new one as added", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			var copies []string
			for _, c := range doc.Behaviors {
				p := c.Head
				if p == nil {
					p = c.Base
				}
				if len(p.Path) > 1 && p.Path[1] == "Copies" {
					copies = append(copies, c.Change+" "+p.Path[len(p.Path)-1])
				}
			}
			gotest.Equal(it, []string{"removed first copy", "removed second copy", "added the only copy"}, copies)
		})
	})
}

func (s *AuditCompareTestSuite) TestWhatGotWeaker(t *gotest.T) {
	const snap, gone = "shop/testdata/__snapshots__/TestCartTestSuite.snap", "shop/testdata/__snapshots__/TestStockTestSuite_ext.snap"
	t.When("the head removes a test, excludes one, focuses one, adds a guard, raises a budget, sets NoDeadline, adds a suppression, rewrites and deletes a snapshot, drops an assertion and adds AssertsBy", func(w *gotest.T) {
		w.It("lists a finding of each class with its subject and its base and head values", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			gotest.Equal(it, []finding{
				{"removed", "Cart / Copies / first copy", "it", ""},
				{"removed", "Cart / Copies / second copy", "it", ""},
				{"removed", "Cart / Remove", "test", ""},
				{"excluded", "Cart / Empty", "TestEmpty", "X_TestEmpty"},
				{"focus", "Cart / Total", "TestTotal", "F_TestTotal"},
				{"guard", "Stock", "no SuiteGuard", "SuiteGuard"},
				{"budget", "Cart", "Timeout 10s", "Timeout 1m0s"},
				{"budget", "Stock", "SetupTimeout 30s", "SetupTimeout none"},
				{"suppression", "shop/stock_suite_test.go", "", "//nolint:stdlib-test // a second stdlib test"},
				{"snapshot", snap, fixtureSum(it, "base", snap)[:12], fixtureSum(it, "head", snap)[:12]},
				{"snapshot", gone, fixtureSum(it, "base", gone)[:12], "deleted"},
				{"assertions", "Cart / Rounds", "2", "1"},
				{"asserts-by", "Cart / Rounds", "no AssertsBy", "AssertsBy"},
			}, findings(&doc, true))
		})
		w.It("lists each changed body as an informational finding, apart from the weakenings", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			var got []string
			for _, f := range findings(&doc, false) {
				if f.Class != "changed" {
					continue
				}
				gotest.Regexp(it, `^[0-9a-f]{16}$`, f.Head)
				gotest.NotEqual(it, f.Base, f.Head)
				got = append(got, f.Subject)
			}
			gotest.Equal(it, []string{"Cart / Add / when one number is negative / returns the difference", "Cart / Rounds"}, got)
		})
	})
	t.When("a suppression keeps its file and rules and gives a new reason", func(w *gotest.T) {
		w.It("lists it as an informational suppression finding naming both comments", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			var got []finding
			for _, f := range findings(&doc, false) {
				if f.Class == "suppression" {
					got = append(got, f)
				}
			}
			gotest.Equal(it, []finding{{"suppression", "shop/plain_test.go", "//nolint:stdlib-test", "//nolint:stdlib-test // a plain stdlib test"}}, got)
		})
	})
	t.When("a suppression moves to another line with its rules and reason", func(w *gotest.T) {
		w.It("lists no suppression finding for it", func(it *gotest.T) {
			doc := readDoc(it, s.json.Stdout)
			for _, f := range doc.Findings {
				gotest.NotContains(it, f.Head, "kept for go test", "%+v", f)
			}
		})
	})
}

func (s *AuditCompareTestSuite) TestTheAuditDocument(t *gotest.T) {
	t.When("audit compares two records with --format=json", func(w *gotest.T) {
		w.It("prints one document opening with version and schema that holds base, head, summary, findings, behaviors and empty evidence and unproven lists", func(it *gotest.T) {
			gotest.Equal(it, 0, s.json.Exit, "stderr: %s", s.json.Stderr)
			gotest.Regexp(it, `^\{"version":"[^"]+","schema":1,"base":\{`, s.json.Stdout)
			for _, key := range []string{`"head":{`, `"summary":{"weakenings":13,"informational":3,`, `"findings":[{`, `"behaviors":[{`, `"evidence":[]`, `"unproven":[]`} {
				gotest.Contains(it, s.json.Stdout, key)
			}
			gotest.Equal(it, 1, strings.Count(s.json.Stdout, "\n"))
		})
	})
	t.When("audit compares two records in the text format", func(w *gotest.T) {
		w.It("prints the weakenings before the behaviors and ends stdout with a closing line naming the verdict, the exit code and the counts, with nothing on stderr", func(it *gotest.T) {
			gotest.Equal(it, 0, s.text.Exit)
			gotest.Empty(it, s.text.Stderr)
			lines := strings.Split(strings.TrimRight(s.text.Stdout, "\n"), "\n")
			weak, behaviors := indexOf(lines, "weakenings: 13"), indexOf(lines, "behaviors: 3 added, 3 removed, 1 reworded, 1 moved, 2 changed")
			gotest.Greater(it, weak, 0, "stdout: %s", s.text.Stdout)
			gotest.Greater(it, behaviors, weak)
			gotest.Equal(it, "gotest audit: PASS exit 0 | weakenings: 13 (removed 3, excluded 1, focus 1, guard 1, budget 2, suppression 1, snapshot 2, assertions 1, asserts-by 1) | behaviors: 3 added, 3 removed, 1 reworded, 1 moved, 2 changed", lines[len(lines)-1])
		})
	})
	t.When("audit compares two records with --format=md", func(w *gotest.T) {
		w.It("writes a heading, a table of the weakenings and a table of the behaviors", func(it *gotest.T) {
			gotest.Equal(it, 0, s.md.Exit, "stderr: %s", s.md.Stderr)
			gotest.Regexp(it, `^### Audit: 13 weakenings\n`, s.md.Stdout)
			gotest.Contains(it, s.md.Stdout, "\n#### Weakenings\n\n| Class | Package | Subject | Base | Head | Where |\n|---|---|---|---|---|---|\n| removed | ./shop | Cart / Copies / first copy | it | — | shop/cart\\_suite\\_test.go:50 |\n")
			gotest.Contains(it, s.md.Stdout, "\n#### Behaviors\n\n| Change | Package | Behavior | Where |\n|---|---|---|---|\n| removed | ./shop | Cart / Copies / first copy | shop/cart\\_suite\\_test.go:50 |\n")
		})
	})
}

func (s *AuditCompareTestSuite) TestThePolicyExit(t *gotest.T) {
	t.When("audit finds weakenings and is given no --fail-on", func(w *gotest.T) {
		w.It("exits 0", func(it *gotest.T) {
			gotest.Equal(it, 0, s.text.Exit)
			gotest.Contains(it, s.text.Stdout, "weakenings: 13")
		})
	})
	t.When("--fail-on names a class the document lists", func(w *gotest.T) {
		w.It("exits 1 and the closing line names FAIL and the classes", func(it *gotest.T) {
			run := s.audit(it, "--fail-on=focus,guard")
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			gotest.Regexp(it, `\ngotest audit: FAIL exit 1 \| weakenings: 13 \(.*\) \| behaviors: .* \| fail-on: focus,guard\n$`, run.Stdout)
			gotest.Equal(it, 0, s.audit(it, "--fail-on=changed").Exit-1, "changed is listed too")
		})
	})
	t.When("--fail-on names an unknown class", func(w *gotest.T) {
		w.It("audit refuses it with exit 2, naming the classes", func(it *gotest.T) {
			run := s.audit(it, "--fail-on=focus,flaky")
			gotest.Equal(it, 2, run.Exit)
			gotest.Empty(it, run.Stdout)
			gotest.Equal(it, "FAIL: --fail-on: unknown finding class \"flaky\"; the classes are removed, excluded, focus, guard, budget, suppression, snapshot, assertions, asserts-by, changed, failing, not-run, vacuous, stale, unproven, survived, all and proof\n", run.Stderr)
		})
	})
}

func indexOf(lines []string, line string) int {
	for i, l := range lines {
		if l == line {
			return i
		}
	}
	return -1
}
