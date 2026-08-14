package gotestrunner

// Package note on fuzzing: this file deliberately does NOT reuse the
// compiled suite binaries produced by BuildSuiteTargets/CompilePackagesStream.
// Native Go fuzz instrumentation (the counters `go test -fuzz` relies on to
// drive the mutation engine) is woven in by cmd/go at `go test` invocation
// time, only when the `-fuzz` flag is present on that invocation. A binary
// produced by a plain `go test -c` (with no `-fuzz`) — which is exactly what
// every other gotest subcommand compiles once and reuses across suites —
// therefore lacks that instrumentation entirely: running it with `-test.fuzz`
// would be silently uninstrumented (coverage-guided mutation degrades to
// undirected random input, with no crash minimization). So RunFuzzTargets
// spawns one `go test -fuzz=...` process per fuzz target instead, letting
// cmd/go do its own build+instrument+run for each. This is slower than the
// binary-reuse path everywhere else in this package, but it is the only way
// to get real, coverage-guided fuzzing.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

// FuzzTarget identifies a single generated Fuzz<Suite>_<Method> wrapper
// function to drive with `go test -fuzz`.
type FuzzTarget struct {
	Package string // import path
	Dir     string // package source directory (working dir for the subprocess)
	Func    string // generated wrapper func name, e.g. FuzzParserTestSuite_FuzzParse
}

// FuzzRunConfig configures a RunFuzzTargets invocation.
type FuzzRunConfig struct {
	OverlayFlag string        // "-overlay=..." from GenerateOverlay's OverlayResult
	Total       time.Duration // --for budget, approximate wall-clock for the whole session; 0 => go's own default per target (no -fuzztime)
	Jobs        int           // concurrent targets; <=0 => defaultFuzzJobs()
	BuildFlags  []string      // additional go test build flags (-tags etc.)
}

// FuzzSchedule is the concrete plan a --for budget resolves to: how many
// targets run at a time, each target's -fuzztime share, and what that adds
// up to in wall-clock. One function owns this arithmetic so the up-front
// plan the CLI prints, the config validation, and the actual run can never
// disagree.
type FuzzSchedule struct {
	Targets   int
	Jobs      int           // resolved effective concurrency (min(jobs, targets))
	Waves     int           // ceil(targets / jobs)
	PerTarget time.Duration // -fuzztime per target; 0 = no -fuzztime (go's default)
	Floored   bool          // the 10s per-target floor raised PerTarget
	EstWall   time.Duration // Waves × PerTarget; 0 when PerTarget is 0
}

// PlanFuzzSchedule resolves total (the --for budget; <=0 = open-ended) into
// a schedule for n targets at the given concurrency (<=0 = default).
//
// The contract is wall-clock: a user writing --for=5m means "this command
// fuzzes for about five minutes", so each target's share is
// total × min(jobs, n) / n — waves of concurrent targets multiply back out
// to ≈total. (The previous jobs-blind total/n split made the session take
// total/jobs instead, quietly finishing a 10-minute budget in under two on
// a six-core default.) The 10s floor keeps a small budget over many targets
// from starving each one; when it engages, EstWall exceeds total and
// Floored says so, so the caller can surface the stretch instead of
// silently overrunning.
func PlanFuzzSchedule(total time.Duration, n, jobs int) FuzzSchedule {
	jobs = min(resolveFuzzJobs(jobs), n)
	s := FuzzSchedule{Targets: n, Jobs: jobs}
	if n <= 0 {
		return s
	}
	s.Waves = (n + jobs - 1) / jobs
	if total <= 0 {
		return s
	}
	per := total * time.Duration(jobs) / time.Duration(n)
	if per < 10*time.Second {
		per = 10 * time.Second
		s.Floored = true
	}
	s.PerTarget = per
	s.EstWall = time.Duration(s.Waves) * per
	return s
}

// fuzzGrace is the fixed grace period given to a `go test -fuzz` subprocess
// to shut down after context cancellation before it is force-killed. Fuzzing
// runs can be mid-mutation when interrupted, so this is generous compared to
// the ordinary suite grace budget.
const fuzzGrace = 30 * time.Second

// FuzzTargetOutcome records how one fuzz target's run ended. Interpretation
// is deliberately separated from raw observation: a `go test -fuzz`
// subprocess cut down by the session ending (deadline or interrupt) exits
// non-zero without having found anything, and one killed mid-crash can die
// before its FAIL output — so neither the exit code nor the output alone is
// trustworthy. The new-crasher-file scan is the finding signal that
// survives both.
type FuzzTargetOutcome struct {
	Func        string
	ExitCode    int      // raw subprocess exit code (0 when it never started)
	Canceled    bool     // the session context was done when the process exited
	Skipped     bool     // never started: the session ended before a job slot freed
	NewCrashers []string // corpus entry files that appeared under testdata/fuzz/<Func>/ during the run
}

// EffectiveExitCode maps the raw observation onto the session's exit
// contract: 0 = ran (or was stopped by the session ending) without a
// finding, 1 = a genuine finding — a failing target, or a new crasher file
// even when the shutdown killed the process mid-crash — and 2 = the
// subprocess could not run at all.
func (o FuzzTargetOutcome) EffectiveExitCode() int {
	if o.Skipped {
		return 0
	}
	code := o.ExitCode
	if o.Canceled && len(o.NewCrashers) == 0 {
		// The non-zero exit here is the shutdown this session caused, not a
		// verdict. A genuine failure racing the deadline is still caught:
		// a crash writes a corpus file (the scan sees it), and a failing
		// seed is deterministic (the next run reproduces it).
		code = 0
	}
	if code == 0 && len(o.NewCrashers) > 0 {
		code = 1
	}
	return code
}

// FuzzRunResult aggregates every target's outcome for one session.
type FuzzRunResult struct {
	Outcomes []FuzzTargetOutcome
}

// ExitCode returns the worst effective exit code across all targets.
func (r FuzzRunResult) ExitCode() int {
	worst := 0
	for _, o := range r.Outcomes {
		if c := o.EffectiveExitCode(); c > worst {
			worst = c
		}
	}
	return worst
}

// CutShort names the targets the session ending stopped or skipped without
// a finding — the ones whose time share was not honored.
func (r FuzzRunResult) CutShort() []string {
	var names []string
	for _, o := range r.Outcomes {
		if (o.Skipped || o.Canceled) && o.EffectiveExitCode() == 0 {
			names = append(names, o.Func)
		}
	}
	return names
}

// RunFuzzTargets runs each target as its own `go test -fuzz=...` subprocess,
// with bounded concurrency (cfg.Jobs, default max(1, GOMAXPROCS/2)). The
// --for budget (cfg.Total) is split evenly across all targets via
// splitBudget. Stdout and stderr of every subprocess are streamed live,
// line by line, each line prefixed with "[<Func>] ", so long fuzz runs show
// progress rather than going silent until they finish (unlike the buffered
// RunSingleSuite path used elsewhere). The caller derives the session exit
// code from the returned outcomes — see FuzzTargetOutcome.EffectiveExitCode
// for the contract.
func RunFuzzTargets(ctx context.Context, targets []FuzzTarget, cfg FuzzRunConfig) FuzzRunResult {
	if len(targets) == 0 {
		return FuzzRunResult{}
	}

	plan := PlanFuzzSchedule(cfg.Total, len(targets), cfg.Jobs)
	jobs := plan.Jobs
	budget := plan.PerTarget

	var wg sync.WaitGroup
	sem := make(chan struct{}, jobs)
	var out sync.Mutex // serializes writes to os.Stdout/os.Stderr across targets

	// Each goroutine writes only its own index; wg.Wait is the barrier.
	outcomes := make([]FuzzTargetOutcome, len(targets))

	for i, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		wg.Add(1)
		go func(i int, t FuzzTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				out.Lock()
				fmt.Fprintf(os.Stderr, "[%s] skipped: session ended before this target started\n", t.Func)
				out.Unlock()
				outcomes[i] = FuzzTargetOutcome{Func: t.Func, Skipped: true}
				return
			}
			defer func() { <-sem }()

			outcomes[i] = runOneFuzzTarget(ctx, t, cfg, budget, &out)
		}(i, target)
	}
	wg.Wait()
	return FuzzRunResult{Outcomes: outcomes}
}

// crasherDir is the corpus directory `go test -fuzz` writes new crashers to
// for target t.
func crasherDir(t FuzzTarget) string { //nolint:gocritic // hugeParam: stable API
	return filepath.Join(t.Dir, "testdata", "fuzz", t.Func)
}

// snapshotCrashers returns the corpus entry names currently on disk for t.
// A missing directory (no crasher ever recorded) is an empty snapshot.
func snapshotCrashers(t FuzzTarget) map[string]bool { //nolint:gocritic // hugeParam: stable API
	entries, err := os.ReadDir(crasherDir(t))
	if err != nil {
		return nil
	}
	set := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			set[e.Name()] = true
		}
	}
	return set
}

// newCrasherNames returns the names present in after but not before, sorted.
func newCrasherNames(before, after map[string]bool) []string {
	var fresh []string
	for name := range after {
		if !before[name] {
			fresh = append(fresh, name)
		}
	}
	sort.Strings(fresh)
	return fresh
}

// defaultFuzzJobs returns the default concurrent-target cap when Jobs is
// unset: max(1, GOMAXPROCS/2).
func defaultFuzzJobs() int {
	if j := runtime.GOMAXPROCS(0) / 2; j > 1 {
		return j
	}
	return 1
}

// resolveFuzzJobs returns jobs if positive, otherwise defaultFuzzJobs().
func resolveFuzzJobs(jobs int) int {
	if jobs > 0 {
		return jobs
	}
	return defaultFuzzJobs()
}

// buildFuzzArgs constructs the `go test` argument list for a single fuzz
// target: the overlay flag, a no-op -run (so no ordinary tests execute), an
// anchored+quoted -fuzz selecting exactly t.Func, an optional -fuzztime
// (omitted when d is zero), any extra build flags, and the target package
// last.
func buildFuzzArgs(t FuzzTarget, cfg FuzzRunConfig, d time.Duration) []string { //nolint:gocritic // hugeParam: stable API
	args := []string{"test"}
	if cfg.OverlayFlag != "" {
		args = append(args, cfg.OverlayFlag)
	}
	args = append(args, "-run=^$", "-fuzz=^"+regexp.QuoteMeta(t.Func)+"$")
	if d > 0 {
		args = append(args, "-fuzztime="+d.String())
	}
	args = append(args, cfg.BuildFlags...)
	args = append(args, t.Package)
	return args
}

func buildFuzzCmd(ctx context.Context, t FuzzTarget, cfg FuzzRunConfig, d time.Duration) *exec.Cmd { //nolint:gocritic // hugeParam: stable API
	cmd := exec.CommandContext(ctx, "go", buildFuzzArgs(t, cfg, d)...)
	cmd.Dir = t.Dir
	return cmd
}

// runOneFuzzTarget runs a single target to completion, streaming its output
// live with a "[<Func>] " prefix, and returns its outcome.
//
// Output is streamed by assigning a lineWriter directly to cmd.Stdout /
// cmd.Stderr (rather than reading from cmd.StdoutPipe()/StderrPipe() on a
// separate goroutine). exec.Cmd's own Wait() already copies from the
// process into any non-*os.File Writer and blocks until that copy finishes
// before returning — the same mechanism RunSingleSuite's bytes.Buffer relies
// on elsewhere in this package. Reading from StdoutPipe/StderrPipe instead
// would race ManagedProcess.Start's internal async cmd.Wait() call: the
// os/exec docs are explicit that Wait must not be called before all pipe
// reads complete, since Wait closes the pipes as soon as it sees the process
// exit, silently truncating whatever the reader hadn't drained yet (exactly
// where a FAIL summary or a crasher path would go missing).
func runOneFuzzTarget(ctx context.Context, t FuzzTarget, cfg FuzzRunConfig, budget time.Duration, out *sync.Mutex) FuzzTargetOutcome { //nolint:gocritic // hugeParam: stable API
	before := snapshotCrashers(t)
	cmd := buildFuzzCmd(ctx, t, cfg, budget)

	stdoutW := newLineWriter(os.Stdout, t.Func, out)
	stderrW := newLineWriter(os.Stderr, t.Func, out)
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	mp := NewManagedProcess(cmd, ProcessConfig{
		Grace:         GraceFixed,
		GraceDuration: fuzzGrace,
	})
	if err := mp.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] FAIL: %s\n", t.Func, err)
		return FuzzTargetOutcome{Func: t.Func, ExitCode: 2}
	}

	_ = mp.WaitWithGrace(ctx)
	// Sampled immediately after the process exits: a deadline firing in the
	// window between exit and this line misclassifies only a failing-seed
	// exit (crashes are covered by the file scan), and a failing seed is
	// deterministic on the next run. See EffectiveExitCode.
	canceled := ctx.Err() != nil
	// Flush whatever partial (unterminated) line remains once the process
	// has exited and exec.Cmd's internal copy has finished.
	_ = stdoutW.Close()
	_ = stderrW.Close()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	outcome := FuzzTargetOutcome{
		Func:        t.Func,
		ExitCode:    exitCode,
		Canceled:    canceled,
		NewCrashers: newCrasherNames(before, snapshotCrashers(t)),
	}

	out.Lock()
	switch {
	case len(outcome.NewCrashers) > 0:
		for _, name := range outcome.NewCrashers {
			fmt.Fprintf(os.Stderr, "[%s] new crasher: %s\n", t.Func, filepath.Join(crasherDir(t), name))
		}
		fmt.Fprintf(os.Stderr, "[%s] inspect it with `gotest fuzz triage`, then `gotest fuzz promote` to keep it as a typed seed\n", t.Func)
	case exitCode != 0 && canceled:
		fmt.Fprintf(os.Stderr, "[%s] stopped: session ended before this target finished (no failures found)\n", t.Func)
	case exitCode != 0:
		fmt.Fprintf(os.Stderr, "[%s] failing without a new crasher file — a seed or existing corpus entry fails; it reproduces on a regular `gotest` run\n", t.Func)
	}
	out.Unlock()

	return outcome
}

// lineWriterMaxBuf caps how much of an unterminated line lineWriter will
// buffer before force-flushing it in chunks. Fuzz crash dumps can contain a
// mutated []byte rendered as one very long line (e.g. via %q); without a
// cap, such a line would buffer forever, since nothing here ever reads it
// back out except on a '\n' or on Close.
const lineWriterMaxBuf = 1 << 20 // 1MB

// lineWriter is an io.WriteCloser that splits whatever is written to it on
// '\n', writing each complete line to dst prefixed with "[<label>] ". A
// trailing, not-yet-terminated line is held in an internal buffer until
// either the next '\n' arrives or Close flushes it. mu is shared across
// potentially many concurrently-active lineWriters (a target's stdout and
// stderr, and other targets running at the same time) so their lines are
// never interleaved mid-write.
type lineWriter struct {
	dst   io.Writer
	label string
	mu    *sync.Mutex
	buf   []byte
}

func newLineWriter(dst io.Writer, label string, mu *sync.Mutex) *lineWriter {
	return &lineWriter{dst: dst, label: label, mu: mu}
}

// Write implements io.Writer. It always consumes all of p and never
// returns an error: emitting to dst is best-effort console output, not
// something a failure here should propagate back into the child process's
// write path.
func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)

	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i], false)
		w.buf = w.buf[i+1:]
	}

	// No newline is in sight yet, but the buffered remainder has grown
	// past the cap: force it out in capped chunks rather than buffering
	// without bound. Each forced chunk is tagged so the split is visible
	// as a deliberate cap, not a corrupted line boundary.
	for len(w.buf) > lineWriterMaxBuf {
		w.emit(w.buf[:lineWriterMaxBuf], true)
		w.buf = w.buf[lineWriterMaxBuf:]
	}

	return len(p), nil
}

// Close flushes any remaining partial (unterminated) line. It is safe to
// call once the underlying command has finished writing.
func (w *lineWriter) Close() error {
	if len(w.buf) > 0 {
		w.emit(w.buf, false)
		w.buf = nil
	}
	return nil
}

func (w *lineWriter) emit(line []byte, continued bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if continued {
		fmt.Fprintf(w.dst, "[%s] (line continued) %s\n", w.label, line)
		return
	}
	fmt.Fprintf(w.dst, "[%s] %s\n", w.label, line)
}
