package mutate_test

import (
	"context"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/mutate"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlanTestSuite: which lines a comment keeps, the order a budget spends
// mutants in, the budget itself on a clock the suite drives, and the
// mapping of a refused build back to its mutants.
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

func (s *PlanTestSuite) TestRefusedBuild(t *gotest.T) {
	list := muts(
		mutate.Mutant{File: "/m/a.go", Line: 4},
		mutate.Mutant{File: "/m/a.go", Line: 4},
		mutate.Mutant{File: "/m/a.go", Line: 9},
		mutate.Mutant{File: "/m/b.go", Line: 7},
	)
	t.When("the compiler refuses lines of the schemata", func(w *gotest.T) {
		w.It("names every mutant of a refused line, and the nearest above a line without one", func(it *gotest.T) {
			out := []byte("# m\n./a.go:4:9: invalid operation\n/m/b.go:12:2: undefined: x\n./a.go:2:1: no mutant above\n")
			gotest.Equal(it, []int{1, 2, 4}, mutate.Refused(out, "/m", list))
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
