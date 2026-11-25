package gotestrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mvrahden/go-test/internal/protocol"
)

// Closing is what a run's closing line says.
type Closing struct {
	ExitCode int
	// Tests counts the test verdicts; nil for a run that does not see them,
	// as a text run does not see a test that passed.
	Tests *protocol.Tally
	// NotRun counts the declared units the census found without a verdict;
	// nil when it did not judge the run.
	NotRun   *int
	Packages protocol.Tally
	// Rerun is the command that reruns the run's failures; "" for none.
	Rerun string
}

// count is one number of a closing line and what it counts.
type count struct {
	n     int
	label string
}

// String is the closing line, without its newline:
//
//	gotest: FAIL exit 1 | tests: 2 passed, 2 failed, 0 skipped, 0 not run | packages: 1 passed, 1 failed, 0 skipped | rerun: gotest --failed
//
// PASS is exit 0 with something passed, EMPTY exit 0 with nothing passed,
// FAIL any other exit code. A segment's counts are a list, so one more can
// join it.
func (c Closing) String() string { //nolint:gocritic // hugeParam: once per run
	passed := c.Packages.Passed
	if c.Tests != nil {
		passed = c.Tests.Passed
	}
	verdict := "PASS"
	switch {
	case c.ExitCode != 0:
		verdict = "FAIL"
	case passed == 0:
		verdict = "EMPTY"
	}
	segments := []string{fmt.Sprintf("gotest: %s exit %d", verdict, c.ExitCode)}
	var tests []count
	if c.Tests != nil {
		tests = append(tests, count{c.Tests.Passed, "passed"}, count{c.Tests.Failed, "failed"}, count{c.Tests.Skipped, "skipped"})
	}
	if c.NotRun != nil && (c.Tests != nil || *c.NotRun > 0) {
		tests = append(tests, count{*c.NotRun, "not run"})
	}
	if len(tests) > 0 {
		segments = append(segments, "tests: "+joinCounts(tests))
	}
	segments = append(segments, "packages: "+joinCounts([]count{{c.Packages.Passed, "passed"}, {c.Packages.Failed, "failed"}, {c.Packages.Skipped, "skipped"}}))
	if c.Rerun != "" {
		segments = append(segments, "rerun: "+c.Rerun)
	}
	return strings.Join(segments, " | ")
}

func joinCounts(counts []count) string {
	parts := make([]string, len(counts))
	for i, c := range counts {
		parts[i] = fmt.Sprintf("%d %s", c.n, c.label)
	}
	return strings.Join(parts, ", ")
}

// ClosingOf is the closing line of a run that judged res and exited code:
// the census books each unit it finds without a verdict as a failed test,
// which the line counts as not run instead.
func ClosingOf(res *protocol.RunResult, code int, countTests bool) Closing {
	c := Closing{ExitCode: code}
	if res == nil {
		return c
	}
	c.NotRun, c.Packages = res.NotRun, res.Packages
	if countTests {
		tests := res.Tests
		if res.NotRun != nil {
			tests.Failed = max(0, tests.Failed-*res.NotRun)
		}
		c.Tests = &tests
	}
	return c
}

// AgentOutput renders a run's go test -json stream for a coding agent: what
// failed and nothing else, each failed test's output whole, then the
// closing line once the run's verdict arrives. A package's output waits for
// its verdict, and that of a package that passed is dropped.
type AgentOutput struct {
	mu      sync.Mutex
	w       io.Writer
	partial []byte
	pkgs    map[string]*agentPackage
	rerun   string
	closed  bool
}

// agentPackage is what a package's stream has held since its last verdict.
type agentPackage struct {
	lines  []agentLine
	failed map[string]bool
}

type agentLine struct{ test, text string }

// NewAgentOutput renders into w.
func NewAgentOutput(w io.Writer) *AgentOutput {
	return &AgentOutput{w: w, pkgs: map[string]*agentPackage{}}
}

// SetRerun names the command the closing line gives for rerunning the
// failures; "" for none.
func (a *AgentOutput) SetRerun(cmd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rerun = cmd
}

// Closed reports whether the closing line has been written.
func (a *AgentOutput) Closed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

func (a *AgentOutput) Write(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.partial = append(a.partial, p...)
	for {
		i := bytes.IndexByte(a.partial, '\n')
		if i < 0 {
			break
		}
		a.line(a.partial[:i])
		a.partial = a.partial[i+1:]
	}
	a.partial = append([]byte(nil), a.partial...)
	return len(p), nil
}

func (a *AgentOutput) line(line []byte) {
	var ev struct {
		Action     string
		Package    string
		Test       string
		Output     string
		ImportPath string
		Result     *protocol.RunResult
	}
	if json.Unmarshal(line, &ev) != nil {
		return
	}
	switch {
	case ev.Action == "build-output":
		_, _ = io.WriteString(a.w, ev.Output)
	case ev.Package == protocol.RunPackage:
		if ev.Result != nil {
			closing := ClosingOf(ev.Result, ev.Result.ExitCode, true)
			if ev.Result.ExitCode != 0 {
				closing.Rerun = a.rerun
			}
			fmt.Fprintln(a.w, closing)
			a.closed = true
		}
	case ev.Package == "":
	case ev.Action == "output":
		if quiet(ev.Test, ev.Output) {
			return
		}
		p := a.pkg(ev.Package)
		p.lines = append(p.lines, agentLine{ev.Test, ev.Output})
	case ev.Action == "fail" && ev.Test != "":
		a.pkg(ev.Package).failed[ev.Test] = true
	case ev.Test == "" && (ev.Action == "pass" || ev.Action == "skip" || ev.Action == "fail"):
		p := a.pkg(ev.Package)
		delete(a.pkgs, ev.Package)
		if ev.Action == "fail" {
			a.flush(ev.Package, p)
		}
	}
}

// flush writes what a failed package's tests that failed printed, in stream
// order. A test the run failed without a result line of its own, as the
// census books one, gets one after its output, and the package gets its
// FAIL line if it printed none.
func (a *AgentOutput) flush(pkg string, p *agentPackage) {
	last := map[string]int{}
	resulted := map[string]bool{}
	summarized := false
	for i, l := range p.lines {
		last[l.test] = i
		if l.test != "" && isResultLine(l.text, l.test) {
			resulted[l.test] = true
		}
		summarized = summarized || l.test == "" && strings.HasPrefix(l.text, "FAIL\t")
	}
	for i, l := range p.lines {
		if l.test != "" && !p.failed[l.test] {
			continue
		}
		_, _ = io.WriteString(a.w, l.text)
		if l.test != "" && !resulted[l.test] && last[l.test] == i {
			fmt.Fprintf(a.w, "--- FAIL: %s\n", l.test)
		}
	}
	if !summarized {
		fmt.Fprintf(a.w, "FAIL\t%s\n", pkg)
	}
}

func (a *AgentOutput) pkg(name string) *agentPackage {
	p := a.pkgs[name]
	if p == nil {
		p = &agentPackage{failed: map[string]bool{}}
		a.pkgs[name] = p
	}
	return p
}

// quiet reports whether an output line is progress rather than a failure's
// content: a test's framing lines, and a package's passing summary lines.
func quiet(test, output string) bool {
	trimmed := strings.TrimLeft(output, " ")
	if test != "" {
		return strings.HasPrefix(trimmed, "=== ")
	}
	line := strings.TrimRight(output, "\r\n")
	return line == "PASS" || line == "FAIL" || strings.HasPrefix(line, "ok  \t") || strings.HasPrefix(line, "?   \t")
}
