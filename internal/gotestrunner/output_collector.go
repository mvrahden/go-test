package gotestrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// OutputCollector is the mode-aware output pipeline every suite result and
// every run-level failure flows through. It is safe for concurrent use from
// multiple goroutines.
type OutputCollector struct {
	mu       sync.Mutex
	mode     RunMode
	verbose  bool
	pkgs     map[string]*pkgState
	order    []string
	flushed  int
	worst    int
	captured bytes.Buffer
	// verdicts indexes the JSON stream for the census as it is written.
	verdicts verdictIndex
	// cutShort names the suites whose process the run's context ended.
	cutShort []CensusCase

	// StdlibTestsByPkg lets RecordNoSuite distinguish packages that truly
	// have no test files from packages whose tests gotest does not run
	// (stdlib tests).
	StdlibTestsByPkg map[string]int
	// bench marks a bench run, whose packages that run nothing pass as go
	// test -bench passes them: without a note.
	bench  bool
	stdout io.Writer
	stderr io.Writer
	// starts writes the JSON stream, opening each package as go test does.
	starts *startWriter
}

type pkgState struct {
	expected  int
	completed int
	results   []SuiteResult
	// failedBuild names the build whose failure failed this package; its
	// verdict carries the name the way go test's does.
	failedBuild string
	// fixed is the summary line of a package whose verdict needs no run:
	// one without suites, or none of whose suites run.
	fixed string
}

type OutputOption func(*OutputCollector)

func WithWriters(stdout, stderr io.Writer) OutputOption {
	return func(c *OutputCollector) {
		c.stdout = stdout
		c.stderr = stderr
	}
}

func NewOutputCollector(mode RunMode, verbose bool, opts ...OutputOption) *OutputCollector {
	c := &OutputCollector{
		mode:    mode,
		verbose: verbose,
		pkgs:    map[string]*pkgState{},
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetFlushOrder pre-establishes the package ordering for head-of-line text
// flushing. Call this before Register when packages may be registered in a
// non-deterministic order (e.g. streaming compilation). Packages not in the
// list that are later Register-ed are appended to the tail.
func (c *OutputCollector) SetFlushOrder(pkgs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order = pkgs
	for _, pkg := range pkgs {
		if _, exists := c.pkgs[pkg]; !exists {
			c.pkgs[pkg] = &pkgState{}
		}
	}
}

// Register prepares the collector for a package with count suites.
// If SetFlushOrder was called, this activates a pre-ordered entry.
// Otherwise it appends the package to the flush order.
func (c *OutputCollector) Register(pkg string, count int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, exists := c.pkgs[pkg]; exists {
		s.expected = count
		s.results = make([]SuiteResult, count)
		return
	}
	c.pkgs[pkg] = &pkgState{
		expected: count,
		results:  make([]SuiteResult, count),
	}
	c.order = append(c.order, pkg)
}

// RecordResult stores a suite result and triggers output emission.
// For RunBatchText, output is deferred until all earlier packages complete
// (head-of-line ordering). For JSON modes, test-level events are emitted
// immediately; package-level synthetic events are emitted when all suites
// for a package complete.
func (c *OutputCollector) RecordResult(pkg string, idx int, r SuiteResult) { //nolint:gocritic // hugeParam: stable API
	c.mu.Lock()
	defer c.mu.Unlock()
	// A binary stopped from outside reports a status it never chose: -1 for a
	// signal on Unix, the raw termination status on Windows (0xC000013A for the
	// console interrupt that a --timeout sends). Neither is a verdict, and
	// neither survives the worst-code comparison below intact — -1 slips under
	// it and a Windows status towers over it, becoming the run's own exit code.
	// Normalize both to a failure at this choke point so every downstream
	// comparison agrees, and say what happened, because the binary's own output
	// ends mid-stream with no verdict of its own.
	if code, how := ExitStatusVerdict(r.ExitCode); how != "" {
		r.ExitCode = code
		note := fmt.Sprintf("gotest: suite binary terminated %s\n", how)
		r.Stderr = append(r.Stderr, note...)
		if c.mode != RunBatchText {
			// The stream is what -json consumers see; a note only stderr
			// carries would leave them a binary that stopped mid-stream
			// with no word why.
			_, _ = c.jsonTarget().Write(outputEvent(pkg, note))
		}
	}
	if r.ExitCode > c.worst {
		c.worst = r.ExitCode
	}
	if r.CutShort {
		name := r.Target.SuiteName
		if r.Target.Bench {
			name = protocol.PrefixBenchmark + name
		}
		c.cutShort = append(c.cutShort, CensusCase{Pkg: pkg, Path: name})
	}
	s := c.pkgs[pkg]
	s.results[idx] = r
	s.completed++

	switch c.mode {
	case RunBatchText:
		c.flushReadyText()
	case RunStreamJSON, RunCaptureJSON:
		w := c.jsonWriter()
		filterPackageLevelEvents(w, r.Stdout)
		c.verdicts.endSuite(r.CutShort)
		if len(r.Stderr) > 0 {
			_, _ = c.stderr.Write(r.Stderr)
		}
		if s.completed == s.expected {
			c.emitJSONPackageSummary(w, pkg, s)
		}
	}
}

// RecordNothingToRun books a package that built but none of whose suites
// run (-run deselected them, or it declares only benchmarks): it passes as
// go test passes a package that ran no tests. No binary ran, so no time
// passed; the line keeps go test's shape for every parser that reads it.
func (c *OutputCollector) RecordNothingToRun(pkg string) {
	line := fmt.Sprintf("ok  \t%s\t0.000s %s\n", pkg, noTestsToRun)
	if c.bench {
		line = fmt.Sprintf("ok  \t%s\t0.000s\n", pkg)
	}
	c.recordFixed(pkg, line, "pass")
}

// RecordNoSuite books a package without suites as go test labels it:
// "?   <pkg>  [no test files]", or "[no suites]" for one with stdlib tests.
// One whose suites are all excluded (X_) has test files and runs none, so it
// passes as RecordNothingToRun books it.
func (c *OutputCollector) RecordNoSuite(pkg string, excluded bool) {
	if excluded {
		c.RecordNothingToRun(pkg)
		return
	}
	note := "[no test files]"
	if c.StdlibTestsByPkg[pkg] > 0 {
		// The package has stdlib tests — gotest runs suites only, so
		// "[no test files]" would be a lie. Point at the truth instead.
		note = "[no suites]"
	}
	c.recordFixed(pkg, fmt.Sprintf("?   \t%s\t%s\n", pkg, note), "skip")
}

// recordFixed books a verdict that needs no run: text output prints it in
// package order, the JSON stream gets it at once.
func (c *OutputCollector) recordFixed(pkg, line, action string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.pkgs[pkg]
	if !ok {
		s = &pkgState{}
		c.pkgs[pkg] = s
		c.order = append(c.order, pkg)
	}
	s.fixed = line
	if c.mode == RunBatchText {
		c.flushReadyText()
		return
	}
	writePackageVerdict(c.jsonWriter(), pkg, line, action)
}

// noTestsToRun is go test's note for a package that ran no tests.
const noTestsToRun = "[no tests to run]"

// noTestsWarning is what a test binary prints on stderr when it ran no
// test; go test notes the package with noTestsToRun when it sees it.
var noTestsWarning = []byte("testing: warning: no tests to run")

// ranNoTests reports whether every suite binary of a package ran no test.
func ranNoTests(results []SuiteResult) bool {
	for i := range results {
		if !bytes.Contains(results[i].Stdout, noTestsWarning) && !bytes.Contains(results[i].Stderr, noTestsWarning) {
			return false
		}
	}
	return len(results) > 0
}

// writePackageVerdict writes the summary line and verdict go test gives a
// package that ran no test binary of ours; the stream opens the package.
func writePackageVerdict(w io.Writer, pkg, summary, action string) {
	now := time.Now()
	writeJSONLine(w, map[string]any{"Time": now, "Action": "output", "Package": pkg, "Output": summary})
	writeJSONLine(w, map[string]any{"Time": now, "Action": action, "Package": pkg, "Elapsed": 0})
}

// RecordBuildFailure books a package that failed to build as a failed package
// with exit code 2 — the code reserved for build errors, distinct from exit 1
// for tests that ran and failed. Build failures travel the same collector path
// as suite results so text output, the JSON stream, and the exit code cannot
// disagree about them. In JSON modes the stream takes go test's own shape:
// the diagnostics are build-output events keyed by the build, build-fail
// ends them, and the package's verdict names the build in FailedBuild.
func (c *OutputCollector) RecordBuildFailure(pkg, msg string) {
	c.Register(pkg, 1)
	build := buildName(pkg, msg)
	c.mu.Lock()
	c.pkgs[pkg].failedBuild = build
	if c.mode != RunBatchText {
		w := c.jsonWriter()
		for line := range strings.Lines(msg) {
			writeJSONLine(w, map[string]any{"ImportPath": build, "Action": "build-output", "Output": line})
		}
		writeJSONLine(w, map[string]any{"ImportPath": build, "Action": "build-fail"})
	}
	c.mu.Unlock()
	c.RecordResult(pkg, 0, SuiteResult{Stderr: []byte(msg), ExitCode: 2})
}

// buildName is the ImportPath go test gives the build that failed: the `#`
// header of the compiler's report — `pkg` or `pkg_test [pkg.test]` — else
// the package itself.
func buildName(pkg, msg string) string {
	for line := range strings.Lines(msg) {
		if h, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(h)
		}
	}
	return pkg
}

// RecordGivenUp books a suite that never ran as failed and says why: on
// stderr, and in JSON modes as a test that failed with the reason as its
// output, so every renderer shows the suite beside its cause.
func (c *OutputCollector) RecordGivenUp(pkg string, idx int, suite, reason string) {
	if c.mode != RunBatchText {
		c.mu.Lock()
		w := c.jsonWriter()
		now := time.Now()
		writeJSONLine(w, map[string]any{"Time": now, "Action": "run", "Package": pkg, "Test": suite})
		writeJSONLine(w, map[string]any{"Time": now, "Action": "output", "Package": pkg, "Test": suite, "Output": "    " + reason + "\n"})
		writeJSONLine(w, map[string]any{"Time": now, "Action": "fail", "Package": pkg, "Test": suite, "Elapsed": 0})
		c.mu.Unlock()
	}
	c.RecordResult(pkg, idx, SuiteResult{Stderr: []byte(reason + "\n"), ExitCode: 1})
}

// Finalize closes a text run: it drains the packages still waiting for an
// earlier one that never reported and emits a trailing FAIL. JSON modes have
// written every verdict as it came.
func (c *OutputCollector) Finalize() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != RunBatchText {
		return
	}
	for c.flushed < len(c.order) {
		pkg := c.order[c.flushed]
		s := c.pkgs[pkg]
		c.flushed++
		if s.fixed != "" || s.expected > 0 && s.completed >= s.expected {
			c.flushTextPkg(s, pkg)
		}
	}
	if c.anyFailedLocked() {
		fmt.Fprintln(c.stdout, "FAIL")
	}
}

func (c *OutputCollector) CapturedJSON() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.captured.Bytes()
}

func (c *OutputCollector) AnyFailed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.anyFailedLocked()
}

func (c *OutputCollector) WorstExitCode() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.worst
}

func (c *OutputCollector) UsesTest2JSON() bool {
	return c.mode != RunBatchText
}

func (c *OutputCollector) EmitSkippedSuites(skippedByPkg map[string][]string) {
	if c.mode == RunBatchText {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.jsonWriter()
	now := time.Now()
	for _, pkg := range slices.Sorted(maps.Keys(skippedByPkg)) {
		names := skippedByPkg[pkg]
		for _, name := range names {
			testFunc := "Test" + name
			writeJSONLine(w, map[string]any{
				"Time": now, "Action": "run", "Package": pkg, "Test": testFunc,
			})
			writeJSONLine(w, map[string]any{
				"Time": now, "Action": "output", "Package": pkg, "Test": testFunc,
				"Output": fmt.Sprintf("--- SKIP: %s (0.00s)\n", testFunc),
			})
			writeJSONLine(w, map[string]any{
				"Time": now, "Action": "output", "Package": pkg, "Test": testFunc,
				"Output": "    test suite was excluded by user\n",
			})
			writeJSONLine(w, map[string]any{
				"Time": now, "Action": "skip", "Package": pkg, "Test": testFunc, "Elapsed": 0,
			})
		}
	}
}

func (c *OutputCollector) anyFailedLocked() bool {
	for _, s := range c.pkgs {
		for i := range s.results {
			if s.results[i].ExitCode != 0 {
				return true
			}
		}
	}
	return false
}

func (c *OutputCollector) flushReadyText() {
	for c.flushed < len(c.order) {
		pkg := c.order[c.flushed]
		s := c.pkgs[pkg]
		if s.fixed == "" && (s.expected == 0 || s.completed < s.expected) {
			break
		}
		c.flushTextPkg(s, pkg)
		c.flushed++
	}
}

func (c *OutputCollector) flushTextPkg(s *pkgState, pkg string) {
	if s.fixed != "" {
		fmt.Fprint(c.stdout, s.fixed)
		return
	}
	failed := false
	dur := packageSpan(s.results)
	for i := range s.results {
		if s.results[i].ExitCode != 0 {
			failed = true
		}
	}
	if c.verbose || failed {
		for i := range s.results {
			r := &s.results[i]
			_, _ = c.stdout.Write(StripTrailingStatus(r.Stdout))
			if len(r.Stderr) > 0 {
				_, _ = c.stderr.Write(r.Stderr)
			}
		}
	}
	switch {
	case s.failedBuild != "":
		fmt.Fprintf(c.stdout, "FAIL\t%s [build failed]\n", pkg)
	case failed:
		fmt.Fprintf(c.stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, dur.Seconds())
	case c.verbose:
		fmt.Fprintf(c.stdout, "PASS\nok  \t%s\t%.3fs%s\n", pkg, dur.Seconds(), ranNothingNote(s))
	default:
		fmt.Fprintf(c.stdout, "ok  \t%s\t%.3fs%s\n", pkg, dur.Seconds(), ranNothingNote(s))
	}
}

// ranNothingNote is go test's note on a passing package whose binaries ran
// no test, "" otherwise.
func ranNothingNote(s *pkgState) string {
	if ranNoTests(s.results) {
		return " " + noTestsToRun
	}
	return ""
}

// jsonWriter is where JSON events are written; the census indexes them on
// the way.
func (c *OutputCollector) jsonWriter() io.Writer {
	return io.MultiWriter(c.jsonTarget(), &c.verdicts)
}

// bookRunFailure books a failure that happened outside any test binary as a
// failed synthetic package, on whichever writer this mode's stream goes to.
// Text mode has no stream; the caller's stderr line is all it gets.
func (c *OutputCollector) bookRunFailure(pkg, msg string, details ...string) {
	if c.mode == RunBatchText {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, _ = c.jsonTarget().Write(runFailureEvents(pkg, msg, details...))
}

func (c *OutputCollector) jsonTarget() io.Writer {
	if c.starts == nil {
		w := c.stdout
		if c.mode == RunCaptureJSON {
			w = &c.captured
		}
		c.starts = &startWriter{w: w, started: map[string]bool{}}
	}
	return c.starts
}

// startWriter writes a JSON event stream, opening each package with go
// test's start event before any other event of it, exactly once. Writes may
// split a line; each is passed on whole.
type startWriter struct {
	w       io.Writer
	buf     []byte
	started map[string]bool
}

func (s *startWriter) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			break
		}
		s.writeLine(s.buf[:i+1])
		s.buf = s.buf[i+1:]
	}
	s.buf = append([]byte(nil), s.buf...)
	return len(p), nil
}

func (s *startWriter) writeLine(line []byte) {
	var ev struct {
		Time    json.RawMessage
		Action  string
		Package string
	}
	if json.Unmarshal(line, &ev) == nil && ev.Package != "" {
		switch {
		case !s.started[ev.Package] && ev.Action == "start":
			s.started[ev.Package] = true
		case !s.started[ev.Package]:
			s.started[ev.Package] = true
			start := map[string]any{"Time": ev.Time, "Action": "start", "Package": ev.Package}
			if len(ev.Time) == 0 {
				start["Time"] = time.Now()
			}
			writeJSONLine(s.w, start)
		case ev.Action == "start":
			return
		}
	}
	_, _ = s.w.Write(line)
}

func (c *OutputCollector) emitJSONPackageSummary(w io.Writer, pkg string, s *pkgState) {
	failed := false
	dur := packageSpan(s.results)
	for i := range s.results {
		if s.results[i].ExitCode != 0 {
			failed = true
		}
	}
	now := time.Now()
	var summaryLine string
	switch {
	case s.failedBuild != "":
		summaryLine = fmt.Sprintf("FAIL\t%s [build failed]\n", pkg)
	case failed:
		summaryLine = fmt.Sprintf("FAIL\t%s\t%.3fs\n", pkg, dur.Seconds())
	default:
		summaryLine = fmt.Sprintf("ok  \t%s\t%.3fs%s\n", pkg, dur.Seconds(), ranNothingNote(s))
	}
	writeJSONLine(w, map[string]any{
		"Time": now, "Action": "output", "Package": pkg, "Output": summaryLine,
	})
	action := "pass"
	if failed {
		action = "fail"
	}
	verdict := map[string]any{"Time": now, "Action": action, "Package": pkg, "Elapsed": dur.Seconds()}
	if s.failedBuild != "" {
		verdict["FailedBuild"] = s.failedBuild
	}
	writeJSONLine(w, verdict)
}

// filterPackageLevelEvents writes test-level JSON events (those with a
// non-empty Test field) to w unchanged, strips package-level structural
// events (start, pass, fail, etc.) and synthesized summary lines that would
// be duplicated across per-suite test2json instances, but keeps
// package-level "output" events carrying diagnostic text (e.g. race
// detector reports, panic stack traces) that would otherwise be silently
// dropped.
func filterPackageLevelEvents(w io.Writer, data []byte) {
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		var line []byte
		if idx >= 0 {
			line = data[:idx]
			data = data[idx+1:]
		} else {
			line = data
			data = nil
		}
		if len(line) == 0 {
			continue
		}
		var ev struct {
			Test   string `json:"Test"`
			Action string `json:"Action"`
			Output string `json:"Output"`
		}
		if json.Unmarshal(line, &ev) != nil || ev.Test != "" {
			_, _ = w.Write(line)
			_, _ = w.Write([]byte{'\n'})
			continue
		}
		if ev.Action == "output" && !protocol.IsPackageSummaryLine(ev.Output) {
			_, _ = w.Write(line)
			_, _ = w.Write([]byte{'\n'})
		}
	}
}

func writeJSONLine(w io.Writer, fields map[string]any) {
	data, _ := json.Marshal(fields)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte{'\n'})
}

// outputEvent is one go test -json output line for pkg.
func outputEvent(pkg, text string) []byte {
	line, _ := json.Marshal(map[string]string{"Action": "output", "Package": pkg, "Output": text})
	return append(line, '\n')
}

// packageSpan is the clock a package's suites occupied: from the first start
// to the last end. Results without a start time, as tests and replays build
// them, fall back to the sum of their durations.
func packageSpan(results []SuiteResult) time.Duration {
	var first, last time.Time
	var sum time.Duration
	timed := true
	for i := range results {
		r := &results[i]
		sum += r.Duration
		if r.Started.IsZero() {
			timed = false
			continue
		}
		end := r.Started.Add(r.Duration)
		if first.IsZero() || r.Started.Before(first) {
			first = r.Started
		}
		if end.After(last) {
			last = end
		}
	}
	if !timed || first.IsZero() {
		return sum
	}
	return last.Sub(first)
}
