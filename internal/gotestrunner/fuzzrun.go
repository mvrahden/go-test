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
	"regexp"
	"runtime"
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
	Total       time.Duration // --for budget, split evenly across all targets; 0 => go's own default per target (no -fuzztime)
	Jobs        int           // concurrent targets; <=0 => defaultFuzzJobs()
	BuildFlags  []string      // additional go test build flags (-tags etc.)
}

// fuzzGrace is the fixed grace period given to a `go test -fuzz` subprocess
// to shut down after context cancellation before it is force-killed. Fuzzing
// runs can be mid-mutation when interrupted, so this is generous compared to
// the ordinary suite grace budget.
const fuzzGrace = 30 * time.Second

// RunFuzzTargets runs each target as its own `go test -fuzz=...` subprocess,
// with bounded concurrency (cfg.Jobs, default max(1, GOMAXPROCS/2)). The
// --for budget (cfg.Total) is split evenly across all targets via
// splitBudget. Stdout and stderr of every subprocess are streamed live,
// line by line, each line prefixed with "[<Func>] ", so long fuzz runs show
// progress rather than going silent until they finish (unlike the buffered
// RunSingleSuite path used elsewhere). It returns the worst (highest) exit
// code across all targets; on a non-zero exit it also prints a hint to the
// target's crasher artifact directory.
func RunFuzzTargets(ctx context.Context, targets []FuzzTarget, cfg FuzzRunConfig) int {
	if len(targets) == 0 {
		return 0
	}

	jobs := resolveFuzzJobs(cfg.Jobs)
	budget := splitBudget(cfg.Total, len(targets))

	var wg sync.WaitGroup
	sem := make(chan struct{}, jobs)
	var out sync.Mutex // serializes writes to os.Stdout/os.Stderr across targets

	var mu sync.Mutex
	worst := 0

	for _, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		wg.Add(1)
		go func(t FuzzTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				out.Lock()
				fmt.Fprintf(os.Stderr, "[%s] skipped: global timeout reached before this target started\n", t.Func)
				out.Unlock()
				return
			}
			defer func() { <-sem }()

			code := runOneFuzzTarget(ctx, t, cfg, budget, &out)

			mu.Lock()
			if code > worst {
				worst = code
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()
	return worst
}

// splitBudget divides total evenly across n targets, flooring at 10s so no
// target gets a budget too short to be useful. A zero total stays zero,
// signaling "no -fuzztime flag; use go's own default" to buildFuzzArgs.
func splitBudget(total time.Duration, n int) time.Duration {
	if total <= 0 || n <= 0 {
		return 0
	}
	d := total / time.Duration(n)
	if d < 10*time.Second {
		return 10 * time.Second
	}
	return d
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
// live with a "[<Func>] " prefix, and returns its exit code.
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
func runOneFuzzTarget(ctx context.Context, t FuzzTarget, cfg FuzzRunConfig, budget time.Duration, out *sync.Mutex) int { //nolint:gocritic // hugeParam: stable API
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
		return 2
	}

	_ = mp.WaitWithGrace(ctx)
	// Flush whatever partial (unterminated) line remains once the process
	// has exited and exec.Cmd's internal copy has finished.
	_ = stdoutW.Close()
	_ = stderrW.Close()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if exitCode != 0 {
		out.Lock()
		fmt.Fprintf(os.Stderr, "[%s] crasher artifacts (if any): %s/testdata/fuzz/%s/\n", t.Func, t.Dir, t.Func)
		out.Unlock()
	}

	return exitCode
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
