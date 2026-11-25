package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/mutate"
)

// Each mutant's run may take timeoutFactor times its suites' baseline
// time, plus twice the runner's own overhead and timeoutSlack.
const (
	timeoutFactor = 3
	timeoutSlack  = time.Second
	// stopGrace is how long a run stopped by its mutant's verdict has
	// before it is forced, and then killed.
	stopGrace = 5 * time.Second
)

// suiteRun is one suite of the coverage run and what it executed.
type suiteRun struct {
	pkg, name string
	executed  mutate.Executed
}

func (s *suiteRun) label() string { return s.pkg + " " + s.name }

// baseline is what the coverage run says: whether it passed and what
// each of its suites executed.
type baseline struct {
	passed bool
	failed []string
	suites []*suiteRun
}

// readBaseline reads the coverage run's verdicts and each suite's coverage,
// its children's folded in, and folds the children's into the run's
// profile too. It returns how many binaries wrote coverage data.
func (m *mutation) readBaseline(events []gotestspec.TestEvent, exit int, profile string) (*baseline, int, error) {
	b := &baseline{passed: exit == 0}
	for i := range events {
		ev := &events[i]
		if ev.Action != gotestspec.ActionFail || ev.Package == "" || strings.Contains(ev.Test, "/") {
			continue
		}
		b.failed = appendOnce(b.failed, strings.TrimSpace(ev.Package+" "+ev.Test))
	}
	b.failed = testsFirst(b.failed)
	b.passed = b.passed && len(b.failed) == 0
	list, err := gotestrunner.ReadSuiteCoverage(m.suiteDir())
	if err != nil {
		if b.passed {
			return nil, 0, fmt.Errorf("--mutate: the run kept no suite's coverage: %w", err)
		}
		return b, 0, nil
	}
	binaries := 0
	for _, sc := range list {
		s := &suiteRun{pkg: sc.Package, name: sc.Suite, executed: mutate.Executed{}}
		if err := addProfile(s.executed, sc.Profile); err != nil && !os.IsNotExist(err) {
			return nil, 0, err
		}
		text, n, err := childCoverage(sc.CoverDir, m.pkgs)
		if err != nil {
			return nil, 0, err
		}
		if n > 0 {
			binaries += n
			if err := appendFile(sc.Profile, text); err != nil {
				return nil, 0, err
			}
			if err := addProfile(s.executed, sc.Profile); err != nil {
				return nil, 0, err
			}
			if profile != "" {
				if err := appendFile(profile, text); err != nil {
					return nil, 0, err
				}
			}
		}
		b.suites = append(b.suites, s)
	}
	return b, binaries, nil
}

// testsFirst is the failed tests of a list of failures, the packages and
// runs alone when no test failed.
func testsFirst(failed []string) []string {
	tests := slices.DeleteFunc(slices.Clone(failed), func(f string) bool { return !strings.Contains(f, " Test") })
	if len(tests) > 0 {
		return tests
	}
	return failed
}

func appendOnce(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// addProfile marks in e the lines the profile's blocks that ran span.
func addProfile(e mutate.Executed, profile string) error {
	f, err := os.Open(profile) //nolint:gosec // G304: a profile the run wrote
	if err != nil {
		return err
	}
	defer f.Close()
	blocks, err := gotestspec.ReadCoverageBlocks(f)
	if err != nil {
		return err
	}
	out := make([]mutate.Block, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, mutate.Block{File: b.File, StartLine: b.StartLine, EndLine: b.EndLine, Ran: b.Count > 0})
	}
	e.Add(out)
	return nil
}

// judge runs each mutant a suite of the coverage run executes against
// those suites, within the budget, after the suites pass on the mutants'
// build with none switched on, and writes the mutation section.
func (a *auditArgs) judge(doc *gotestaudit.Document, m *mutation, b *baseline) {
	jobs, per := a.mutationJobs()
	run := &gotestaudit.MutationRun{Budget: a.mutateBudget, Jobs: jobs, Operators: m.operators(a), Ignores: m.ignores,
		Baseline: gotestaudit.MutationBaseline{Passed: b != nil && b.passed}}
	if b != nil {
		run.Baseline.Failed = b.failed
	}
	results, todo, covering := m.classify(b, run.Baseline.Passed && !a.stopped)
	j := &mutantJudge{a: a, m: m, per: per, covering: covering}
	if len(todo) > 0 {
		ctx, stop := a.mutationContext()
		zero := j.zero(ctx, todo)
		run.Baseline.Suites, run.Baseline.Passed, run.Baseline.Failed = zero.suites, zero.passed, zero.failed
		if ctx.Err() == nil && zero.passed {
			first := map[string]bool{}
			for i := range doc.Behaviors {
				first[doc.Behaviors[i].Package] = true
			}
			todo = mutate.Order(todo, func(pkg string) bool { return first[pkg] })
			outcomes, spent := (&mutate.Scheduler{Jobs: jobs, Budget: a.mutateBudget}).Run(ctx, todo, j.run)
			run.Spent = spent
			for i, o := range outcomes {
				r := results[todo[i].ID]
				r.Verdict, r.By = o.Verdict, o.By
			}
		}
		if ctx.Err() != nil {
			a.stopped = true
			doc.Incomplete = append(doc.Incomplete, "--mutate: the mutants' runs were stopped; the mutants left read as not judged")
			doc.Summary.Incomplete = len(doc.Incomplete)
		}
		stop()
	}
	list := make([]gotestaudit.MutantResult, 0, len(m.all))
	for i := range m.all {
		list = append(list, *results[m.all[i].ID])
	}
	gotestaudit.AddMutation(doc, list, run)
	m.addNotes(doc, &run.Baseline)
}

// classify gives each mutant its result, judged or not: ignored, refused,
// not covered by a suite of b, or, when judge holds, one to run, with the
// suites that execute its line.
func (m *mutation) classify(b *baseline, judge bool) (map[int]*gotestaudit.MutantResult, []mutate.Mutant, map[int][]*suiteRun) {
	results := map[int]*gotestaudit.MutantResult{}
	var todo []mutate.Mutant
	covering := map[int][]*suiteRun{}
	for i := range m.all {
		mu := &m.all[i]
		r := m.result(mu)
		results[mu.ID] = r
		switch {
		case m.ignored[mu.ID] != "":
			r.Verdict, r.Reason = gotestaudit.MutantIgnored, m.ignored[mu.ID]
		case m.refused[mu.ID]:
			r.Verdict = gotestaudit.MutantUncompilable
		case !judge:
			r.Verdict = gotestaudit.MutantNotJudged
		default:
			for _, s := range b.suites {
				if s.executed.Ran(mu) {
					covering[mu.ID] = append(covering[mu.ID], s)
					r.Suites = append(r.Suites, s.label())
				}
			}
			if len(covering[mu.ID]) == 0 {
				r.Verdict = gotestaudit.MutantNotCovered
				continue
			}
			r.Verdict = gotestaudit.MutantNotJudged
			todo = append(todo, *mu)
		}
	}
	return results, todo, covering
}

// result is mu as the document names it.
func (m *mutation) result(mu *mutate.Mutant) *gotestaudit.MutantResult {
	f := m.files[mu.File]
	r := &gotestaudit.MutantResult{Package: mu.Package, Func: mu.Func, Line: mu.Line, Col: mu.Col, Operator: mu.Op, Original: mu.Original, Mutant: mu.Mutated}
	if f != nil {
		r.Module, r.File = f.Module, f.File
	}
	if lines := m.lines[mu.File]; mu.Line-1 < len(lines) {
		r.LineText = strings.TrimSpace(lines[mu.Line-1])
	}
	return r
}

// operators are the operators the run applied, in judging order.
func (m *mutation) operators(a *auditArgs) []string {
	return slices.DeleteFunc(slices.Clone(mutate.Operators), func(op string) bool { return op == mutate.OpString && !a.mutateStrings })
}

// addNotes says, once each, what the section cannot tell.
func (m *mutation) addNotes(doc *gotestaudit.Document, b *gotestaudit.MutationBaseline) {
	doc.Notes = append(doc.Notes, m.notes...)
	doc.Notes = append(doc.Notes, schemataNote)
	if doc.Mutation.Counts.Survived > 0 {
		doc.Notes = append(doc.Notes, "--mutate: "+gotestaudit.IgnoreNote)
	}
	if doc.Mutation.Counts.NotCovered > 0 {
		doc.Notes = append(doc.Notes, binaryNote)
	}
	if !b.Passed {
		doc.Incomplete = append(doc.Incomplete, "--mutate: no mutant was judged: "+strings.Join(b.Failed, ", ")+" failed before any mutant was switched on")
		doc.Summary.Incomplete = len(doc.Incomplete)
	}
}

// mutationJobs is how many mutants run at once and each run's --parallel:
// half the processors, at most four, sharing --parallel when given.
func (a *auditArgs) mutationJobs() (jobs, per int) {
	jobs = min(4, max(1, runtime.GOMAXPROCS(0)/2))
	if a.parallel > 0 {
		jobs = min(jobs, a.parallel)
		per = max(1, a.parallel/jobs)
	}
	return jobs, per
}

// mutationContext ends on an interrupt and, under --control=stdin, on a
// stop line or the input's end, as a run's stop does.
func (a *auditArgs) mutationContext() (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	if !a.control {
		return ctx, stop
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		events := make(chan stopEvent, 1)
		go readControl(os.Stdin, events)
		select {
		case <-events:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, func() { cancel(); stop() }
}

// mutantJudge runs one mutant's suites through this CLI's runner.
type mutantJudge struct {
	a        *auditArgs
	m        *mutation
	per      int
	covering map[int][]*suiteRun
	mu       sync.Mutex
	// took is each suite's time on the mutants' build with none switched
	// on; overhead the least time a run took beyond its suites'.
	took     map[string]time.Duration
	overhead time.Duration
}

// zeroRun is the run of the mutants' suites with none switched on.
type zeroRun struct {
	passed bool
	suites int
	failed []string
}

// zero runs every suite a mutant is judged by on the mutants' build with
// none switched on: each must pass, or no mutant is judged. It times each
// suite for the mutants' limits.
func (j *mutantJudge) zero(ctx context.Context, todo []mutate.Mutant) zeroRun {
	var suites []*suiteRun
	for i := range todo {
		for _, s := range j.covering[todo[i].ID] {
			if !slices.Contains(suites, s) {
				suites = append(suites, s)
			}
		}
	}
	w := &streamWriter{collect: true, took: map[string]time.Duration{}}
	start := time.Now()
	code := j.runSuites(ctx, 0, suites, w, 0)
	j.took, j.overhead = w.took, time.Since(start)
	z := zeroRun{suites: len(suites), failed: testsFirst(w.failed), passed: code == 0 && len(w.failed) == 0 && ctx.Err() == nil}
	if !z.passed && len(z.failed) == 0 {
		z.failed = []string{fmt.Sprintf("the run (exit %d)", code)}
	}
	return z
}

// limit is how long a mutant's run may take before it counts as a timeout.
func (j *mutantJudge) limit(suites []*suiteRun) time.Duration {
	sum := j.sum(suites)
	j.mu.Lock()
	defer j.mu.Unlock()
	return timeoutFactor*sum + 2*j.overhead + timeoutSlack
}

func (j *mutantJudge) sum(suites []*suiteRun) time.Duration {
	var sum time.Duration
	for _, s := range suites {
		sum += j.took[s.label()]
	}
	return sum
}

// observe lowers the overhead estimate by a run that took took.
func (j *mutantJudge) observe(took time.Duration, suites []*suiteRun) {
	took -= j.sum(suites)
	j.mu.Lock()
	defer j.mu.Unlock()
	if took > 0 && took < j.overhead {
		j.overhead = took
	}
}

// run judges mu: killed at the first test that fails, survived when its
// suites pass, a timeout past its limit; not judged when ctx ends first.
func (j *mutantJudge) run(ctx context.Context, mu *mutate.Mutant) mutate.Outcome {
	suites := j.covering[mu.ID]
	w := &streamWriter{first: make(chan string, 1)}
	start := time.Now()
	code := j.runSuites(ctx, mu.ID, suites, w, j.limit(suites))
	o := mutate.Outcome{Elapsed: time.Since(start)}
	switch {
	case w.timedOut:
		o.Verdict = mutate.Timeout
	case w.killedBy != "":
		o.Verdict, o.By = mutate.Killed, w.killedBy
	case code == 130 || code < 0 || ctx.Err() != nil:
		o.Verdict = mutate.NotJudged
	case code != 0:
		o.Verdict, o.By = mutate.Killed, fmt.Sprintf("the run's exit %d", code)
	default:
		o.Verdict = mutate.Survived
		j.observe(o.Elapsed, suites)
	}
	return o
}

// runSuites runs suites through this CLI's runner with mutant id switched
// on, 0 for none, writing its -json stream to w: stopped at w's first
// failure unless w collects, at limit when given, or when ctx ends. It
// returns the run's exit code, -1 when it could not start.
func (j *mutantJudge) runSuites(ctx context.Context, id int, suites []*suiteRun, w *streamWriter, limit time.Duration) int {
	var names, pkgs []string
	for _, s := range suites {
		names = appendOnce(names, regexp.QuoteMeta(s.name))
		pkgs = appendOnce(pkgs, s.pkg)
	}
	exe, err := os.Executable()
	if err != nil {
		return -1
	}
	args := []string{"--live", "--control=stdin", "-json", "-run=^(" + strings.Join(names, "|") + ")$"}
	if j.per > 0 {
		args = append(args, fmt.Sprintf("--parallel=%d", j.per))
	}
	args = append(append(args, j.a.loadFlags...), pkgs...)
	cmd := exec.Command(exe, args...) //nolint:gosec // G204: this CLI, running the suites of one mutant
	cmd.Env = j.m.runEnv(os.Environ(), id)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return -1
	}
	cmd.Stdout, cmd.Stderr = w, io.Discard
	if err := cmd.Start(); err != nil {
		return -1
	}
	defer stdin.Close()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	var expired <-chan time.Time
	if limit > 0 {
		t := time.NewTimer(limit)
		defer t.Stop()
		expired = t.C
	}
	select {
	case <-w.first:
		return exitCodeOf(stopRun(stdin, waited, cmd))
	case err := <-waited:
		return exitCodeOf(err)
	case <-expired:
		w.timedOut = true
	case <-ctx.Done():
	}
	// A failure the stop itself causes kills nothing.
	w.halted.Store(true)
	return exitCodeOf(stopRun(stdin, waited, cmd))
}

// stopRun sends the run a stop line, then a force line, then kills it.
func stopRun(stdin io.Writer, waited <-chan error, cmd *exec.Cmd) error {
	for _, line := range []string{"stop\n", "force\n"} {
		_, _ = io.WriteString(stdin, line)
		select {
		case err := <-waited:
			return err
		case <-time.After(stopGrace):
		}
	}
	_ = cmd.Process.Kill()
	return <-waited
}

// streamWriter reads a -json stream as it is written: the first test or
// package it fails, sent on first, or, when it collects, every one, and
// each suite's time.
type streamWriter struct {
	first    chan string
	collect  bool
	killedBy string
	failed   []string
	took     map[string]time.Duration
	timedOut bool
	// halted stops the writer reading: the run was stopped for a timeout
	// or the budget's end.
	halted atomic.Bool
	part   []byte
}

func (w *streamWriter) Write(p []byte) (int, error) {
	if w.killedBy != "" || w.halted.Load() {
		return len(p), nil
	}
	w.part = append(w.part, p...)
	for {
		i := bytes.IndexByte(w.part, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := w.part[:i]
		w.part = w.part[i+1:]
		var ev struct {
			Action, Package, Test string
			Elapsed               float64
		}
		if json.Unmarshal(line, &ev) != nil {
			continue
		}
		top := ev.Test != "" && !strings.Contains(ev.Test, "/")
		if top && (ev.Action == "pass" || ev.Action == "fail") && w.took != nil {
			w.took[ev.Package+" "+ev.Test] = time.Duration(ev.Elapsed * float64(time.Second))
		}
		if ev.Action != "fail" {
			continue
		}
		by := strings.TrimSpace(ev.Package + " " + ev.Test)
		if w.collect {
			if ev.Test == "" || top {
				w.failed = appendOnce(w.failed, by)
			}
			continue
		}
		w.killedBy, w.part = by, nil
		w.first <- by
		return len(p), nil
	}
}

func exitCodeOf(err error) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		return -1
	}
}
