package mutate_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/mutate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlanTestSuite: which lines a comment keeps, the order a budget spends
// mutants in, the budget itself on a clock the suite drives, the second
// run of a mutant that timed out, and the mapping of a refused build back
// to its mutants.
type PlanTestSuite struct{}

func (s *PlanTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 10 * time.Second}
}

const ignored = `package p

// Heuristic picks.
//gotest:mutate-ignore any split is a valid diff
func Heuristic(a, b int) bool {
	return a < b
}

func Sum(n int) int {
	s := 0
	//gotest:mutate-ignore i == 0 adds nothing
	for i := n; i > 0; i-- {
		s += i
	}
	return s //gotest:mutate-ignore the zero sum is never read
}

func Bare() int {
	return 1 //gotest:mutate-ignore
}

//gotest:mutate-ignorex not the directive
var x = 1
`

func (s *PlanTestSuite) TestIgnoreComments(t *gotest.T) {
	igs := mutate.Ignores("p.go", []byte(ignored))
	t.When("the directive is read", func(w *gotest.T) {
		w.It("keeps the function it documents, the statement below it and the line it ends, each with its reason", func(it *gotest.T) {
			gotest.Equal(it, []mutate.Ignore{
				{Line: 4, Start: 5, End: 7, Scope: mutate.ScopeFunction, Func: "Heuristic", Reason: "any split is a valid diff"},
				{Line: 11, Start: 12, End: 14, Scope: mutate.ScopeStatement, Reason: "i == 0 adds nothing"},
				{Line: 15, Start: 15, End: 15, Scope: mutate.ScopeLine, Reason: "the zero sum is never read"},
				{Line: 19, Start: 19, End: 19, Scope: mutate.ScopeLine},
			}, igs)
		})
		w.It("keeps nothing for a comment without a reason", func(it *gotest.T) {
			gotest.True(it, igs[0].Keeps(6))
			gotest.True(it, igs[1].Keeps(13))
			gotest.False(it, igs[1].Keeps(15))
			gotest.False(it, igs[3].Keeps(19))
		})
	})
}

func muts(spec ...mutate.Mutant) []mutate.Mutant {
	for i := range spec {
		spec[i].ID = i + 1
	}
	return spec
}

func (s *PlanTestSuite) TestBudgetOrder(t *gotest.T) {
	all := muts(
		mutate.Mutant{Package: "m/a", File: "/m/a/a.go", Line: 3, Op: mutate.OpBoundary},
		mutate.Mutant{Package: "m/a", File: "/m/a/a.go", Line: 3, Op: mutate.OpNegate},
		mutate.Mutant{Package: "m/a", File: "/m/a/a.go", Line: 9, Op: mutate.OpDelete},
		mutate.Mutant{Package: "m/b", File: "/m/b/b.go", Line: 2, Op: mutate.OpLiteral},
		mutate.Mutant{Package: "m/b", File: "/m/b/b.go", Line: 2, Op: mutate.OpReturn},
	)
	t.When("mutants of several lines are ordered for a budget", func(w *gotest.T) {
		w.It("takes one of each line before a second of any, the first packages' lines first, each line's in operator order", func(it *gotest.T) {
			got := mutate.Order(all, func(pkg string) bool { return pkg == "m/b" })
			var ids []int
			for _, m := range got {
				ids = append(ids, m.ID)
			}
			// b:2 return, a:3 negate, a:9 delete; then b:2 literal, a:3 boundary.
			gotest.Equal(it, []int{5, 2, 3, 4, 1}, ids)
		})
	})
}

// clock is a clock the suite moves.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (s *PlanTestSuite) TestTheBudget(t *gotest.T) {
	list := muts(mutate.Mutant{Line: 1}, mutate.Mutant{Line: 2}, mutate.Mutant{Line: 3}, mutate.Mutant{Line: 4}, mutate.Mutant{Line: 5})
	t.When("the budget is spent while mutants are left", func(w *gotest.T) {
		w.It("starts none once spent and leaves the rest not judged", func(it *gotest.T) {
			c := &clock{now: time.Unix(0, 0)}
			sched := mutate.Scheduler{Jobs: 1, Budget: 25 * time.Second, Now: c.Now,
				AfterFunc: func(time.Duration, func()) func() bool { return func() bool { return true } }}
			out, spent := sched.Run(it.Context(), list, func(context.Context, *mutate.Mutant) mutate.Outcome {
				c.advance(10 * time.Second)
				return mutate.Outcome{Verdict: mutate.Survived}
			})
			var verdicts []string
			for _, o := range out {
				verdicts = append(verdicts, o.Verdict)
			}
			gotest.Equal(it, []string{mutate.Survived, mutate.Survived, mutate.Survived, mutate.NotJudged, mutate.NotJudged}, verdicts)
			gotest.Equal(it, 30*time.Second, spent)
		})
	})
	t.When("the budget ends while a mutant runs", func(w *gotest.T) {
		w.It("counts that mutant not judged unless it was killed", func(it *gotest.T) {
			var expire func()
			sched := mutate.Scheduler{Jobs: 1, Budget: time.Hour,
				AfterFunc: func(_ time.Duration, f func()) func() bool { expire = f; return func() bool { return true } }}
			out, _ := sched.Run(it.Context(), list[:3], func(ctx context.Context, m *mutate.Mutant) mutate.Outcome {
				switch m.Line {
				case 1:
					return mutate.Outcome{Verdict: mutate.Killed}
				case 2:
					expire()
					<-ctx.Done()
					return mutate.Outcome{Verdict: mutate.Timeout}
				}
				return mutate.Outcome{Verdict: mutate.Survived}
			})
			gotest.Equal(it, []mutate.Outcome{{Verdict: mutate.Killed}, {Verdict: mutate.NotJudged}, {Verdict: mutate.NotJudged}}, out)
		})
	})
}

func (s *PlanTestSuite) TestATimeoutRunTwice(t *gotest.T) {
	list := muts(mutate.Mutant{Line: 1}, mutate.Mutant{Line: 2}, mutate.Mutant{Line: 3})
	// judge answers line 1 a timeout once, line 2 always, line 3 survived,
	// logging each run and how many ran at once.
	judge := func(log *[]string, mu *sync.Mutex, busy *int) func(context.Context, *mutate.Mutant) mutate.Outcome {
		runs := map[int]int{}
		return func(_ context.Context, m *mutate.Mutant) mutate.Outcome {
			mu.Lock()
			*busy++
			runs[m.Line]++
			n := runs[m.Line]
			*log = append(*log, fmt.Sprintf("%d#%d with %d", m.Line, n, *busy))
			mu.Unlock()
			if n == 1 {
				time.Sleep(20 * time.Millisecond)
			}
			mu.Lock()
			*busy--
			mu.Unlock()
			switch {
			case m.Line == 1 && n == 1, m.Line == 2:
				return mutate.Outcome{Verdict: mutate.Timeout}
			case m.Line == 1:
				return mutate.Outcome{Verdict: mutate.Killed, By: "TestX"}
			}
			return mutate.Outcome{Verdict: mutate.Survived}
		}
	}
	t.When("a mutant's run times out", func(w *gotest.T) {
		w.It("runs it once more, alone after the others, and judges it by that run", func(it *gotest.T) {
			var log []string
			var mu sync.Mutex
			busy := 0
			sched := mutate.Scheduler{Jobs: 3, Budget: time.Hour}
			out, _ := sched.Run(it.Context(), list, judge(&log, &mu, &busy))
			gotest.Equal(it, []mutate.Outcome{{Verdict: mutate.Killed, By: "TestX"}, {Verdict: mutate.Timeout}, {Verdict: mutate.Survived}}, out)
			gotest.Len(it, log, 5)
			gotest.Equal(it, []string{"1#2 with 1", "2#2 with 1"}, log[3:])
		})
	})
	t.When("the budget is spent before the second run", func(w *gotest.T) {
		w.It("reads the mutant as not judged", func(it *gotest.T) {
			c := &clock{now: time.Unix(0, 0)}
			sched := mutate.Scheduler{Jobs: 1, Budget: 25 * time.Second, Now: c.Now,
				AfterFunc: func(time.Duration, func()) func() bool { return func() bool { return true } }}
			out, _ := sched.Run(it.Context(), list[1:2], func(context.Context, *mutate.Mutant) mutate.Outcome {
				c.advance(30 * time.Second)
				return mutate.Outcome{Verdict: mutate.Timeout}
			})
			gotest.Equal(it, []mutate.Outcome{{Verdict: mutate.NotJudged}}, out)
		})
	})
}

func (s *PlanTestSuite) TestRefusedBuild(t *gotest.T) {
	// The platform's own paths, as its compiler writes them: C:\m on Windows.
	root, err := filepath.Abs(string(filepath.Separator) + "m")
	gotest.NoError(t, err)
	a, b := filepath.Join(root, "a.go"), filepath.Join(root, "b.go")
	rel := filepath.FromSlash("./a.go")
	list := muts(
		mutate.Mutant{File: a, Line: 4},
		mutate.Mutant{File: a, Line: 4},
		mutate.Mutant{File: a, Line: 9},
		mutate.Mutant{File: b, Line: 7},
	)
	t.When("the compiler refuses lines of the schemata", func(w *gotest.T) {
		w.It("names every mutant of a refused line, and the nearest above a line without one", func(it *gotest.T) {
			out := []byte("# m\n" + rel + ":4:9: invalid operation\n" + b + ":12:2: undefined: x\n" + rel + ":2:1: no mutant above\n")
			gotest.Equal(it, []int{1, 2, 4}, mutate.Refused(out, root, list))
		})
	})
}

func (s *PlanTestSuite) TestExecutedLines(t *gotest.T) {
	t.When("blocks of a profile are read", func(w *gotest.T) {
		w.It("marks the lines of a block that ran, under the profile's name of the file", func(it *gotest.T) {
			e := mutate.Executed{}
			e.Add([]mutate.Block{{File: "m/a/a.go", StartLine: 3, EndLine: 5, Ran: true}, {File: "m/a/a.go", StartLine: 8, EndLine: 9}})
			gotest.True(it, e.Ran(&mutate.Mutant{Package: "m/a", File: "/src/m/a/a.go", Line: 4}))
			gotest.False(it, e.Ran(&mutate.Mutant{Package: "m/a", File: "/src/m/a/a.go", Line: 8}))
			gotest.False(it, e.Ran(&mutate.Mutant{Package: "m/b", File: "/src/m/b/a.go", Line: 4}))
		})
	})
}
