package audit_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditMutateTestSuite: audit --mutate over testdata/audit/mutate, whose
// calc/calc.go is changed whole. Its verdicts are stated by hand: calc's
// own suite never gives Discount 100, so that boundary survives; only
// report's suite calls Grade; only the label binary a test builds calls
// Label; no test calls Unused; removing Sum's i-- loops forever.
type AuditMutateTestSuite struct {
	// base is the module's own record: the audits with --mutate change no
	// behavior, so no suite runs for a changed behavior.
	dir, module, diff, empty, base string
	// tree is the module's files before the audits.
	tree map[string]string
	// json, text and md audit with --mutate; spent with a budget spent at
	// once; failing with a suite that fails anyway; plain without --mutate;
	// unasked names survived without it; record records the module.
	json, text, md, spent, failing, plain, unasked, record testkit.Run
	after                                                  map[string]string
}

func (s *AuditMutateTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 180 * time.Second}
}

func (s *AuditMutateTestSuite) BeforeAll(t *gotest.T) {
	dir, err := os.MkdirTemp("", "gotest-audit-mutate-")
	gotest.NoError(t, err)
	s.dir = dir
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.module = filepath.Join(dir, "mutmod")
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "audit", "mutate"), s.module))
	// The diff adds calc.go whole.
	src, err := os.ReadFile(filepath.Join(s.module, "calc", "calc.go"))
	gotest.NoError(t, err)
	lines := strings.Split(strings.TrimSuffix(string(src), "\n"), "\n")
	diff := fmt.Sprintf("--- /dev/null\n+++ b/calc/calc.go\n@@ -0,0 +1,%d @@\n+%s\n", len(lines), strings.Join(lines, "\n+"))
	s.diff = filepath.Join(dir, "calc.diff")
	gotest.NoError(t, os.WriteFile(s.diff, []byte(diff), 0o600))
	s.empty, err = filepath.Abs(filepath.Join("..", "testdata", "audit", "empty-record.json"))
	gotest.NoError(t, err)
	s.tree = s.files(t)
	s.base = filepath.Join(dir, "base.json")
	gotest.Equal(t, 0, s.cli(t, nil, "audit", "--record", "--output="+s.base).Exit)

	s.json = s.audit(t, nil, "--mutate", "--format=json")
	s.text = s.audit(t, nil, "--mutate", "--fail-on=survived")
	s.md = s.audit(t, nil, "--mutate", "--format=md")
	s.spent = s.audit(t, nil, "--mutate", "--mutate-budget=1ns", "--format=json")
	s.failing = s.audit(t, []string{"MUTMOD_FAIL=1"}, "--mutate", "--format=json")
	s.plain = s.cli(t, nil, "audit", "--base-record="+s.empty, "--format=json")
	s.unasked = s.audit(t, nil, "--run", "--fail-on=survived")
	s.after = s.files(t)
	s.record = s.cli(t, nil, "audit", "--record")
}

func (s *AuditMutateTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *AuditMutateTestSuite) audit(t *gotest.T, env []string, args ...string) testkit.Run {
	return s.cli(t, env, append([]string{"audit", "--base-record=" + s.base, "--diff=" + s.diff}, args...)...)
}

// cli runs the CLI in the module with the suite's own gotest cache.
func (s *AuditMutateTestSuite) cli(t *gotest.T, env []string, args ...string) testkit.Run {
	env = append(append(outside(s.dir), "GOTEST_CACHE_DIR="+filepath.Join(s.dir, "cache"), "GOTMPDIR="+s.dir), env...)
	run, err := testkit.RunCLIWithEnv(t.Context(), s.module, env, args...)
	gotest.NoError(t, err)
	return run
}

// files maps each file of the module to its content.
func (s *AuditMutateTestSuite) files(t *gotest.T) map[string]string {
	out := map[string]string{}
	err := filepath.WalkDir(s.module, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G122: the suite's own staged module
		out[path] = string(data)
		return err
	})
	gotest.NoError(t, err)
	return out
}

// mutationDoc is the part of the document the suite reads.
type mutationDoc struct {
	Summary struct {
		Classes  map[string]int `json:"classes"`
		Mutation *counts        `json:"mutation"`
	} `json:"summary"`
	Findings []struct {
		Class, Subject, Head string
		Weakens              bool
	} `json:"findings"`
	Mutation *struct {
		Budget, Spent string
		Baseline      struct {
			Passed bool
			Failed []string
		}
		Counts counts
		Lines  []mutatedLine
	} `json:"mutation"`
	Incomplete []string `json:"incomplete"`
	Notes      []string `json:"notes"`
}

type counts struct {
	Mutants, Killed, Survived, Timeout, NotJudged, NotCovered, Ignored, Uncompilable, Lines, SurvivedLines int
}

type mutatedLine struct {
	ID, Module, File, Package, Func, Group, Verdict, Reason string
	Line, Order                                             int
	Killed, Survived, Timeout, NotJudged, NotCovered        int
	Ignored                                                 int
	Suites                                                  []string
	Mutants                                                 []struct {
		ID, Operator, Original, Mutant, Verdict, By string
	}
}

func readMutation(t *gotest.T, run testkit.Run) *mutationDoc { //nolint:gocritic // hugeParam: a run as RunCLI returns it
	var doc mutationDoc
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "stderr: %s\nstdout: %s", run.Stderr, run.Stdout)
	return &doc
}

// line is the document's entry for a line of calc.go.
func (d *mutationDoc) line(t *gotest.T, n int) *mutatedLine {
	gotest.NotZero(t, d.Mutation)
	for i := range d.Mutation.Lines {
		if d.Mutation.Lines[i].Line == n {
			return &d.Mutation.Lines[i]
		}
	}
	gotest.Fail(t, fmt.Sprintf("no entry for line %d", n))
	return &mutatedLine{}
}

// verdicts lists a line's mutants as operator, mutated text, verdict and
// the test that killed it.
func verdicts(l *mutatedLine) []string {
	var out []string
	for _, m := range l.Mutants {
		out = append(out, strings.TrimSpace(m.Operator+" "+m.Original+" → "+m.Mutant+" "+m.Verdict+" "+m.By))
	}
	return out
}

func (s *AuditMutateTestSuite) TestMutantsOfTheChangedLines(t *gotest.T) {
	t.When("audit --mutate is given a diff that changes the lines of a package", func(w *gotest.T) {
		w.It("lists each changed line holding a mutant once, with each mutant's operator, original text and mutated text", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			var lines []int
			for i := range doc.Mutation.Lines {
				lines = append(lines, doc.Mutation.Lines[i].Line)
			}
			gotest.ElementsMatch(it, []int{6, 14, 17, 22, 23, 24, 25, 32, 37, 42}, lines)
			gotest.Equal(it, 21, doc.Mutation.Counts.Mutants)
			gotest.Equal(it, []string{
				"literal 0 → 1 killed mutmod/calc TestCalcTestSuite/TestDiscount",
			}, verdicts(doc.line(it, 17)))
			gotest.Equal(it, []string{
				"early-return s := 0 → return 0 killed mutmod/calc TestCalcTestSuite/TestSum",
				"literal 0 → 1 killed mutmod/calc TestCalcTestSuite/TestSum",
			}, verdicts(doc.line(it, 22)))
		})
		w.It("leaves the module as it was", func(it *gotest.T) {
			gotest.Equal(it, s.tree, s.after)
		})
	})
}

func (s *AuditMutateTestSuite) TestTheSuitesThatJudgeAMutant(t *gotest.T) {
	t.When("audit --mutate judges lines only another package's tests or a binary a test starts execute", func(w *gotest.T) {
		w.It("judges each line's mutants by the suites that executed it, that package's suite and the one starting the binary among them", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			grade, label := doc.line(it, 6), doc.line(it, 42)
			gotest.Equal(it, []string{"mutmod/report TestReportTestSuite"}, grade.Suites)
			gotest.Equal(it, []string{
				`early-return if score >= 50 { return "pass" } → return "" killed mutmod/report TestReportTestSuite/TestGrades`,
				"boundary score >= 50 → score > 50 killed mutmod/report TestReportTestSuite/TestGrades",
				"negate-compare score >= 50 → score < 50 killed mutmod/report TestReportTestSuite/TestGrades",
			}, verdicts(grade))
			gotest.Equal(it, []string{"mutmod/e2e TestLabelTestSuite"}, label.Suites)
			gotest.Equal(it, 3, label.Killed)
		})
		w.It("reads the mutants of a line no test executed as not covered", func(it *gotest.T) {
			unused := readMutation(it, s.json).line(it, 32)
			gotest.Equal(it, "not covered", unused.Verdict)
			gotest.Equal(it, 3, unused.NotCovered)
			gotest.Empty(it, unused.Suites)
		})
	})
}

func (s *AuditMutateTestSuite) TestAMutantsVerdict(t *gotest.T) {
	t.When("audit --mutate runs the mutants of a change one of whose lines no test checks at its boundary", func(w *gotest.T) {
		w.It("marks each mutant a test fails killed, naming that test", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			gotest.Equal(it, 0, s.json.Exit, "stderr: %s", s.json.Stderr)
			gotest.Equal(it, 13, doc.Mutation.Counts.Killed)
			gotest.Equal(it, []string{"delete-stmt s += i →  killed mutmod/calc TestCalcTestSuite/TestSum"}, verdicts(doc.line(it, 24)))
			gotest.Equal(it, []string{"early-return return n / 2 → return 0 killed mutmod/calc TestCalcTestSuite/TestHalf"}, verdicts(doc.line(it, 37)))
		})
		w.It("marks the boundary mutant survived, naming the suites that ran its line", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			discount := doc.line(it, 14)
			gotest.Equal(it, "survived", discount.Verdict)
			gotest.Equal(it, []string{
				"boundary total > 100 → total >= 100 survived",
				"early-return if total > 100 { return total / 10 } → return 0 killed mutmod/calc TestCalcTestSuite/TestDiscount",
				"negate-compare total > 100 → total <= 100 killed mutmod/calc TestCalcTestSuite/TestDiscount",
			}, verdicts(discount))
			gotest.Equal(it, []string{"mutmod/calc TestCalcTestSuite"}, discount.Suites)
			gotest.Equal(it, 1, doc.Mutation.Counts.Survived)
		})
		w.It("marks a mutant that keeps its suite from ending a timeout, apart from the killed ones", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			gotest.Equal(it, []string{"delete-stmt i-- →  timeout"}, verdicts(doc.line(it, 25)))
			gotest.Equal(it, 1, doc.Mutation.Counts.Timeout)
		})
	})
	t.When("a suite that judges a mutant fails before any mutant is switched on", func(w *gotest.T) {
		w.It("judges no mutant, names the failing test among what was not compared and exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, s.failing.Exit, "stderr: %s", s.failing.Stderr)
			doc := readMutation(it, s.failing)
			gotest.False(it, doc.Mutation.Baseline.Passed)
			gotest.Equal(it, []string{"mutmod/calc TestCalcTestSuite"}, doc.Mutation.Baseline.Failed)
			c := doc.Mutation.Counts
			gotest.Equal(it, []int{0, 0, 0, 18, 3}, []int{c.Killed, c.Survived, c.Timeout, c.NotJudged, c.Ignored})
			gotest.Contains(it, doc.Incomplete, "--mutate: no mutant was judged: mutmod/calc TestCalcTestSuite failed before any mutant was switched on")
		})
	})
}

func (s *AuditMutateTestSuite) TestTheMutationBudget(t *gotest.T) {
	t.When("audit --mutate is given a budget spent before its first mutant", func(w *gotest.T) {
		w.It("reads each mutant it would run as not judged and none as killed or survived", func(it *gotest.T) {
			doc := readMutation(it, s.spent)
			c := doc.Mutation.Counts
			gotest.Equal(it, []int{0, 0, 0, 15, 3, 3}, []int{c.Killed, c.Survived, c.Timeout, c.NotJudged, c.NotCovered, c.Ignored})
			gotest.Equal(it, "not judged", doc.line(it, 14).Verdict)
			gotest.Equal(it, []string{"mutmod/calc TestCalcTestSuite"}, doc.line(it, 14).Suites)
		})
		w.It("names the budget and the time spent", func(it *gotest.T) {
			doc := readMutation(it, s.spent)
			gotest.Equal(it, "1ns", doc.Mutation.Budget)
			gotest.Equal(it, "0s", doc.Mutation.Spent)
			gotest.Equal(it, "3m0s", readMutation(it, s.json).Mutation.Budget)
		})
	})
}

func (s *AuditMutateTestSuite) TestMutantsInTheDocument(t *gotest.T) {
	t.When("a tool reads the document of audit --mutate as JSON", func(w *gotest.T) {
		w.It("finds a mutation section whose lines carry an id, module, file, line, verdict, order, counts, the suites that ran them and their mutants", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			ids := map[string]bool{}
			for i := range doc.Mutation.Lines {
				l := &doc.Mutation.Lines[i]
				gotest.Equal(it, i, l.Order)
				gotest.Len(it, l.ID, 16)
				ids[l.ID] = true
				gotest.Equal(it, "mutmod", l.Module)
				gotest.Equal(it, "calc/calc.go", l.File)
				gotest.Equal(it, "mutmod/calc", l.Package)
				gotest.Equal(it, "calc/calc.go "+l.Func, l.Group)
				gotest.NotEmpty(it, l.Mutants)
			}
			gotest.Len(it, ids, 10)
			// Survived, timeout, not covered, ignored, then the killed lines.
			gotest.Equal(it, []int{14, 25, 32, 23}, []int{doc.Mutation.Lines[0].Line, doc.Mutation.Lines[1].Line, doc.Mutation.Lines[2].Line, doc.Mutation.Lines[3].Line})
		})
		w.It("finds the mutants counted by verdict in the summary", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			gotest.Equal(it, &counts{Mutants: 21, Killed: 13, Survived: 1, Timeout: 1, NotCovered: 3, Ignored: 3, Lines: 10, SurvivedLines: 1}, doc.Summary.Mutation)
			gotest.Equal(it, 1, doc.Summary.Classes["survived"])
		})
		w.It("finds neither in a document made without --mutate", func(it *gotest.T) {
			gotest.Equal(it, 0, s.plain.Exit, "stderr: %s", s.plain.Stderr)
			gotest.NotContains(it, s.plain.Stdout, `"mutation"`)
		})
	})
	t.When("audit --mutate renders text", func(w *gotest.T) {
		w.It("writes per file the survived and timed-out lines first, each with its mutants, function and suites, and counts the other lines", func(it *gotest.T) {
			_, section, found := strings.Cut(s.text.Stdout, "\nmutation: ")
			gotest.True(it, found, s.text.Stdout)
			section, _, _ = strings.Cut(section, "\n\n")
			gotest.Equal(it, strings.Join([]string{
				"1 survived on 1 line, 13 killed, 1 timeout, 0 not judged, 3 not covered, 3 ignored",
				"  calc/calc.go · ran by ./calc TestCalcTestSuite",
				"    14    survived   total > 100 → total >= 100 (boundary); 2 killed · Discount",
				"    25    timeout    i-- removed (delete-stmt) · Sum",
				"    8 more lines: 6 killed, 1 not covered, 1 ignored",
			}, "\n"), dropRunLine(section))
		})
		w.It("the closing line counts the mutants survived, killed, timed out and not judged", func(it *gotest.T) {
			gotest.Contains(it, lastLine(s.text.Stdout), " | mutation: 1 survived on 1 line, 13 killed, 1 timeout, 0 not judged")
		})
	})
	t.When("audit --mutate renders --format=md", func(w *gotest.T) {
		w.It("writes the survived and timed-out lines as a table", func(it *gotest.T) {
			gotest.Equal(it, 0, s.md.Exit, "stderr: %s", s.md.Stderr)
			gotest.Contains(it, s.md.Stdout, "#### Mutation\n")
			gotest.Contains(it, s.md.Stdout, "| File | Line | Verdict | Mutants | Function | Ran by | Killed |\n|---|---|---|---|---|---|---|\n"+
				"| calc/calc.go | 14 | survived | total &gt; 100 → total &gt;= 100 (boundary) | Discount | ./calc TestCalcTestSuite | 2 |\n"+
				"| calc/calc.go | 25 | timeout | i-- removed (delete-stmt) | Sum | ./calc TestCalcTestSuite | 0 |\n")
		})
	})
}

// dropRunLine leaves out the section's second line, which holds the time
// spent.
func dropRunLine(section string) string {
	lines := strings.Split(section, "\n")
	if len(lines) < 2 {
		return section
	}
	return strings.Join(append(lines[:1:1], lines[2:]...), "\n")
}

func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	return lines[len(lines)-1]
}

func (s *AuditMutateTestSuite) TestSurvivorsInThePolicy(t *gotest.T) {
	t.When("--fail-on names survived and a mutant survives", func(w *gotest.T) {
		w.It("exits 1 and the closing line names the class", func(it *gotest.T) {
			gotest.Equal(it, 1, s.text.Exit, "stderr: %s", s.text.Stderr)
			gotest.Regexp(it, `^gotest audit: FAIL exit 1 \|.* \| fail-on: survived$`, lastLine(s.text.Stdout))
		})
	})
	t.When("--fail-on names survived without --mutate", func(w *gotest.T) {
		w.It("audit refuses it with exit 2, naming --mutate", func(it *gotest.T) {
			gotest.Equal(it, 2, s.unasked.Exit)
			gotest.Equal(it, "FAIL: --fail-on=survived judges mutants: give --mutate, with --base or --diff\n", s.unasked.Stderr)
		})
	})
}

func (s *AuditMutateTestSuite) TestKeepingALineFromMutation(t *gotest.T) {
	t.When("audit --mutate meets a comment with a reason at the end of a changed line, and one without a reason above a function", func(w *gotest.T) {
		w.It("reads the mutants of the first line as ignored, giving the reason", func(it *gotest.T) {
			loop := readMutation(it, s.json).line(it, 23)
			gotest.Equal(it, "ignored", loop.Verdict)
			gotest.Equal(it, 3, loop.Ignored)
			gotest.Equal(it, "at i == 0 the loop adds nothing", loop.Reason)
		})
		w.It("judges the function's mutants and notes that the comment without a reason keeps no line", func(it *gotest.T) {
			doc := readMutation(it, s.json)
			gotest.Equal(it, "killed", doc.line(it, 37).Verdict)
			gotest.Contains(it, doc.Notes, "--mutate: calc/calc.go:35: //gotest:mutate-ignore without a reason keeps no line")
		})
	})
	t.When("the head adds mutate-ignore comments", func(w *gotest.T) {
		w.It("audit --record lists each with its file, line, scope and reason", func(it *gotest.T) {
			gotest.Equal(it, 0, s.record.Exit, "stderr: %s", s.record.Stderr)
			var rec struct {
				MutateIgnores []struct {
					File, Scope, Func, Reason string
					Line, Start, End          int
				} `json:"mutateIgnores"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(s.record.Stdout), &rec))
			gotest.Len(it, rec.MutateIgnores, 2)
			first, second := rec.MutateIgnores[0], rec.MutateIgnores[1]
			gotest.Equal(it, []any{"calc/calc.go", 23, "line", 23, 23, "at i == 0 the loop adds nothing"}, []any{first.File, first.Line, first.Scope, first.Start, first.End, first.Reason})
			gotest.Equal(it, []any{"calc/calc.go", 35, "function", 36, 38, "Half", ""}, []any{second.File, second.Line, second.Scope, second.Start, second.End, second.Func, second.Reason})
		})
		w.It("audit lists each as a suppression finding naming the comment", func(it *gotest.T) {
			var heads []string
			for _, f := range readMutation(it, s.plain).Findings {
				if f.Class == "suppression" {
					gotest.True(it, f.Weakens)
					gotest.Equal(it, "calc/calc.go", f.Subject)
					heads = append(heads, f.Head)
				}
			}
			gotest.ElementsMatch(it, []string{"//gotest:mutate-ignore at i == 0 the loop adds nothing", "//gotest:mutate-ignore"}, heads)
		})
	})
}
