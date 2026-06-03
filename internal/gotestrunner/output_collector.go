package gotestrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// OutputCollector is a unified, mode-aware output pipeline that replaces
// PackageBatcher and the scattered JSON / formatting helpers. It is safe
// for concurrent use from multiple goroutines.
type OutputCollector struct {
	mu       sync.Mutex
	mode     RunMode
	verbose  bool
	pkgs     map[string]*pkgState
	order    []string
	flushed  int
	worst    int
	captured bytes.Buffer
	stdout   io.Writer
	stderr   io.Writer
}

type pkgState struct {
	expected  int
	completed int
	results   []SuiteResult
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
func (c *OutputCollector) RecordResult(pkg string, idx int, r SuiteResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.ExitCode > c.worst {
		c.worst = r.ExitCode
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
		if len(r.Stderr) > 0 {
			c.stderr.Write(r.Stderr)
		}
		if s.completed == s.expected {
			c.emitJSONPackageSummary(w, pkg, s)
		}
	}
}

// Finalize emits trailing annotations after all suites have run.
// For RunBatchText: drains remaining completed packages, writes [no test files]
// annotations, and emits trailing FAIL. For JSON / captured modes: no-op.
func (c *OutputCollector) Finalize(noTestFilePkgs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != RunBatchText {
		return
	}
	for c.flushed < len(c.order) {
		pkg := c.order[c.flushed]
		s := c.pkgs[pkg]
		c.flushed++
		if s.expected > 0 && s.completed >= s.expected {
			c.flushTextPkg(s, pkg)
		}
	}
	for _, pkg := range noTestFilePkgs {
		fmt.Fprintf(c.stdout, "?   \t%s\t[no test files]\n", pkg)
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
	for pkg, names := range skippedByPkg {
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
		for _, r := range s.results {
			if r.ExitCode != 0 {
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
		if s.expected == 0 || s.completed < s.expected {
			break
		}
		c.flushTextPkg(s, pkg)
		c.flushed++
	}
}

func (c *OutputCollector) flushTextPkg(s *pkgState, pkg string) {
	failed := false
	var dur time.Duration
	for _, r := range s.results {
		if r.ExitCode != 0 {
			failed = true
		}
		dur += r.Duration
	}
	if c.verbose || failed {
		for _, r := range s.results {
			c.stdout.Write(StripTrailingStatus(r.Stdout))
			if len(r.Stderr) > 0 {
				c.stderr.Write(r.Stderr)
			}
		}
	}
	if failed {
		fmt.Fprintf(c.stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, dur.Seconds())
	} else if c.verbose {
		fmt.Fprintf(c.stdout, "PASS\nok  \t%s\t%.3fs\n", pkg, dur.Seconds())
	} else {
		fmt.Fprintf(c.stdout, "ok  \t%s\t%.3fs\n", pkg, dur.Seconds())
	}
}

func (c *OutputCollector) jsonWriter() io.Writer {
	if c.mode == RunCaptureJSON {
		return &c.captured
	}
	return c.stdout
}

func (c *OutputCollector) emitJSONPackageSummary(w io.Writer, pkg string, s *pkgState) {
	failed := false
	var dur time.Duration
	for _, r := range s.results {
		if r.ExitCode != 0 {
			failed = true
		}
		dur += r.Duration
	}
	now := time.Now()
	var summaryLine string
	if failed {
		summaryLine = fmt.Sprintf("FAIL\t%s\t%.3fs\n", pkg, dur.Seconds())
	} else {
		summaryLine = fmt.Sprintf("ok  \t%s\t%.3fs\n", pkg, dur.Seconds())
	}
	writeJSONLine(w, map[string]any{
		"Time": now, "Action": "output", "Package": pkg, "Output": summaryLine,
	})
	action := "pass"
	if failed {
		action = "fail"
	}
	writeJSONLine(w, map[string]any{
		"Time": now, "Action": action, "Package": pkg, "Elapsed": dur.Seconds(),
	})
}

// filterPackageLevelEvents writes only test-level JSON events (those with a
// non-empty Test field) to w, stripping package-level events that would be
// duplicated across per-suite test2json instances.
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
			Test string `json:"Test"`
		}
		if json.Unmarshal(line, &ev) != nil || ev.Test != "" {
			w.Write(line)
			w.Write([]byte{'\n'})
		}
	}
}

func writeJSONLine(w io.Writer, fields map[string]any) {
	data, _ := json.Marshal(fields)
	w.Write(data)
	w.Write([]byte{'\n'})
}
