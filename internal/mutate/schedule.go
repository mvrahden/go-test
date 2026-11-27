package mutate

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Block is one block of a coverage profile: its file as the profile names
// it (import path and base name), its lines and whether it ran.
type Block struct {
	File               string
	StartLine, EndLine int
	Ran                bool
}

// Executed is, per profile file, the lines a block that ran spans.
type Executed map[string]map[int]bool

// Add marks the lines of the blocks that ran.
func (e Executed) Add(blocks []Block) {
	for _, b := range blocks {
		if !b.Ran {
			continue
		}
		lines := e[b.File]
		if lines == nil {
			lines = map[int]bool{}
			e[b.File] = lines
		}
		for l := b.StartLine; l <= b.EndLine; l++ {
			lines[l] = true
		}
	}
}

// Ran reports whether the line of m was executed.
func (e Executed) Ran(m *Mutant) bool { return e[ProfileFile(m)][m.Line] }

// ProfileFile is the name a coverage profile gives m's file.
func ProfileFile(m *Mutant) string { return m.Package + "/" + filepath.Base(m.File) }

// Order sorts mutants for a budget: the first mutant of every line before
// the second of any, a line of a first package before the others, then by
// file, line and operator.
func Order(muts []Mutant, first func(pkg string) bool) []Mutant {
	out := slices.Clone(muts)
	type key struct {
		file string
		line int
	}
	opRank := func(op string) int { return slices.Index(Operators, op) }
	// A line's mutants in operator order give each its round.
	byLine := map[key][]int{}
	for i := range out {
		k := key{out[i].File, out[i].Line}
		byLine[k] = append(byLine[k], i)
	}
	round := make([]int, len(out))
	for _, idx := range byLine {
		slices.SortFunc(idx, func(a, b int) int {
			return cmp.Or(cmp.Compare(opRank(out[a].Op), opRank(out[b].Op)), cmp.Compare(out[a].ID, out[b].ID))
		})
		for r, i := range idx {
			round[i] = r
		}
	}
	rank := func(pkg string) int {
		if first != nil && first(pkg) {
			return 0
		}
		return 1
	}
	order := make([]int, len(out))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ma, mb := &out[a], &out[b]
		return cmp.Or(cmp.Compare(round[a], round[b]), cmp.Compare(rank(ma.Package), rank(mb.Package)),
			cmp.Compare(ma.File, mb.File), cmp.Compare(ma.Line, mb.Line),
			cmp.Compare(opRank(ma.Op), opRank(mb.Op)), cmp.Compare(ma.ID, mb.ID))
	})
	sorted := make([]Mutant, len(out))
	for i, j := range order {
		sorted[i] = out[j]
	}
	return sorted
}

// Verdicts of a judged mutant and of one left.
const (
	Killed    = "killed"
	Survived  = "survived"
	Timeout   = "timeout"
	NotJudged = "not judged"
)

// Outcome is what the run of one mutant found.
type Outcome struct {
	Verdict string
	// By names the test that failed first, for a killed mutant.
	By      string
	Elapsed time.Duration
}

// Scheduler runs mutants within a budget, Jobs at a time. Now and AfterFunc
// stand in for the clock, so a test can drive the budget.
type Scheduler struct {
	Jobs      int
	Budget    time.Duration
	Now       func() time.Time
	AfterFunc func(time.Duration, func()) (stop func() bool)
}

// Run judges muts in order with run, starting none once the budget is
// spent or ctx is done: those, and any run the budget's end cut short, are
// not judged. It returns each mutant's outcome by index and how long the
// mutants took.
func (s *Scheduler) Run(ctx context.Context, muts []Mutant, run func(ctx context.Context, m *Mutant) Outcome) ([]Outcome, time.Duration) {
	out := make([]Outcome, len(muts))
	for i := range out {
		out[i].Verdict = NotJudged
	}
	now, after := s.Now, s.AfterFunc
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	start := now()
	budget, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := after(s.Budget, cancel)
	defer stop()
	next := 0
	var mu sync.Mutex
	take := func() int {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(muts) || budget.Err() != nil || now().Sub(start) >= s.Budget {
			return -1
		}
		next++
		return next - 1
	}
	var wg sync.WaitGroup
	for range max(1, s.Jobs) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := take(); i >= 0; i = take() {
				o := run(budget, &muts[i])
				if budget.Err() != nil && o.Verdict != Killed && o.Verdict != Survived {
					o.Verdict = NotJudged
				}
				out[i] = o
			}
		}()
	}
	wg.Wait()
	return out, now().Sub(start)
}
