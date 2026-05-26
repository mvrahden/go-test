package gotestrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// SuiteTarget identifies a single test suite (or group of standalone tests)
// to run in its own subprocess.
type SuiteTarget struct {
	Package      string   // import path (for reporting)
	Dir          string   // package source directory (working dir for the binary)
	BinaryPath   string   // path to compiled test binary
	SuiteName    string   // test function name, e.g., "TestFooTestSuite"
	RunFilter    string   // raw -test.run value (overrides SuiteName if set)
	RunFlags     []string // test binary flags (with -test. prefix)
	CoverProfile string   // per-suite cover profile path (empty if no -coverprofile)
	BudgetFile   string   // sidecar path for teardown budget (empty = use default)
}

// SuiteResult holds the output from running a single suite subprocess.
type SuiteResult struct {
	Target   SuiteTarget
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Duration time.Duration
}

// RunMode controls how RunSuites executes and collects output.
type RunMode int

const (
	// RunBatchText runs suites directly (no test2json). Output is batched
	// per-package: when all suites for a package complete, their output is
	// written to stdout followed by a package summary line.
	RunBatchText RunMode = iota

	// RunStreamJSON wraps each suite with test2json and streams its JSON
	// events to stdout as soon as it completes.
	RunStreamJSON

	// RunCaptureJSON wraps each suite with test2json and captures all JSON
	// events into a buffer (nothing written to stdout).
	RunCaptureJSON
)

// PackageBatcher collects per-suite results and flushes output when all
// suites for a package have completed. Packages are flushed in registration
// order to match `go test` output ordering, regardless of completion order.
// Results within each package are stored at fixed indices to guarantee
// deterministic output order regardless of goroutine scheduling.
//
// All methods must be called under a single external mutex.
type PackageBatcher struct {
	pkgs    map[string]*pkgBatch
	order   []string
	flushed int
	verbose bool
}

type pkgBatch struct {
	expected  int
	completed int
	results   []SuiteResult
}

func NewPackageBatcher(verbose bool) *PackageBatcher {
	return &PackageBatcher{pkgs: map[string]*pkgBatch{}, verbose: verbose}
}

// Register prepares the batcher for a package with count suites.
// Packages are flushed in the order they are registered.
func (b *PackageBatcher) Register(pkg string, count int) {
	b.pkgs[pkg] = &pkgBatch{
		expected: count,
		results:  make([]SuiteResult, count),
	}
	b.order = append(b.order, pkg)
}

// Record stores a result at position idx within its package.
// Returns true when all suites for that package are now complete.
func (b *PackageBatcher) Record(pkg string, idx int, r SuiteResult) bool {
	s := b.pkgs[pkg]
	s.results[idx] = r
	s.completed++
	return s.completed == s.expected
}

// FlushReady writes output for all consecutive completed packages starting
// from the current head of the registration order. This ensures packages
// appear in registration order even when they complete out of order.
func (b *PackageBatcher) FlushReady() {
	for b.flushed < len(b.order) {
		pkg := b.order[b.flushed]
		s := b.pkgs[pkg]
		if s.completed < s.expected {
			break
		}
		b.flush(pkg)
		b.flushed++
	}
}

// Flush writes a single completed package's output unconditionally.
// Prefer FlushReady for ordered output.
func (b *PackageBatcher) Flush(pkg string) {
	b.flush(pkg)
}

func (b *PackageBatcher) flush(pkg string) {
	s := b.pkgs[pkg]
	pkgFailed := false
	var pkgDuration time.Duration
	for _, pr := range s.results {
		if pr.ExitCode != 0 {
			pkgFailed = true
		}
		pkgDuration += pr.Duration
	}
	if b.verbose || pkgFailed {
		for _, pr := range s.results {
			os.Stdout.Write(StripTrailingStatus(pr.Stdout))
			if len(pr.Stderr) > 0 {
				os.Stderr.Write(pr.Stderr)
			}
		}
	}
	WritePackageSummary(pkg, pkgFailed, pkgDuration, b.verbose)
}

// AnyFailed reports whether any recorded package had a non-zero exit code.
func (b *PackageBatcher) AnyFailed() bool {
	for _, s := range b.pkgs {
		for _, r := range s.results {
			if r.ExitCode != 0 {
				return true
			}
		}
	}
	return false
}

// RunSuites executes each suite target in its own subprocess with bounded
// concurrency. The mode parameter controls output format and delivery.
// The returned []byte is non-nil only for RunCaptureJSON.
func RunSuites(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int, mode RunMode, verbose bool) ([]byte, int) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	useTest2JSON := mode != RunBatchText

	var batcher *PackageBatcher
	var localIdx []int
	if mode == RunBatchText {
		batcher = NewPackageBatcher(verbose)
		pkgCount := map[string]int{}
		var pkgOrder []string
		localIdx = make([]int, len(targets))
		for i, t := range targets {
			if _, seen := pkgCount[t.Package]; !seen {
				pkgOrder = append(pkgOrder, t.Package)
			}
			localIdx[i] = pkgCount[t.Package]
			pkgCount[t.Package]++
		}
		for _, pkg := range pkgOrder {
			batcher.Register(pkg, pkgCount[pkg])
		}
	}

	var merged bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup
	var worstCode int
	sem := make(chan struct{}, maxParallel)

	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}

	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t SuiteTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			r := RunSingleSuite(ctx, t, env, useTest2JSON)

			mu.Lock()
			if r.ExitCode > worstCode {
				worstCode = r.ExitCode
			}
			switch mode {
			case RunBatchText:
				batcher.Record(t.Package, localIdx[idx], r)
				batcher.FlushReady()
			case RunStreamJSON:
				os.Stdout.Write(r.Stdout)
				if len(r.Stderr) > 0 {
					os.Stderr.Write(r.Stderr)
				}
			case RunCaptureJSON:
				merged.Write(r.Stdout)
			}
			mu.Unlock()
		}(i, target)
	}
	wg.Wait()

	return merged.Bytes(), worstCode
}

func buildSuiteCmd(ctx context.Context, target SuiteTarget, env []string, test2json bool) *exec.Cmd {
	var runArg string
	if target.RunFilter != "" {
		runArg = "-test.run=" + target.RunFilter
	} else {
		runArg = fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName))
	}

	var testArgs []string
	testArgs = append(testArgs, runArg)

	if test2json {
		testArgs = append(testArgs, "-test.v=test2json")
		for _, f := range target.RunFlags {
			if f == "-test.v" || strings.HasPrefix(f, "-test.v=") {
				continue
			}
			testArgs = append(testArgs, f)
		}
	} else {
		testArgs = append(testArgs, target.RunFlags...)
	}

	if target.CoverProfile != "" {
		testArgs = append(testArgs, "-test.coverprofile="+target.CoverProfile)
	}

	if test2json {
		args := []string{"tool", "test2json", "-p", target.Package, "-t", target.BinaryPath}
		args = append(args, testArgs...)
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Env = env
		if target.BudgetFile != "" {
			cmd.Env = append(cmd.Env, "GOTEST_TEARDOWN_BUDGET_FILE="+target.BudgetFile)
		}
		cmd.Dir = target.Dir
		SetProcessGroup(cmd)
		cmd.WaitDelay = 0
		return cmd
	}

	cmd := exec.CommandContext(ctx, target.BinaryPath, testArgs...)
	cmd.Env = env
	if target.BudgetFile != "" {
		cmd.Env = append(cmd.Env, "GOTEST_TEARDOWN_BUDGET_FILE="+target.BudgetFile)
	}
	cmd.Dir = target.Dir
	SetProcessGroup(cmd)
	cmd.WaitDelay = 0
	return cmd
}

// RunSingleSuite executes a single suite subprocess.
// When test2json is true, the binary is wrapped with `go tool test2json`.
//
// On context cancellation, cmd.Cancel sends SIGTERM to the process group.
// A per-process kill timer (from the teardown budget file) then governs
// when SIGKILL is sent, rather than Go's built-in WaitDelay.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) SuiteResult {
	cmd := buildSuiteCmd(ctx, target, env, test2json)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	if err := cmd.Start(); err != nil {
		return SuiteResult{Target: target, ExitCode: 2, Duration: time.Since(start)}
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		budget := readTeardownBudget(target.BudgetFile)
		select {
		case err = <-done:
		case <-time.After(budget):
			ForceKillProcessGroup(cmd.Process.Pid)
			err = <-done
		}
	}

	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else {
			exitCode = 2
		}
	}

	return SuiteResult{
		Target:   target,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
		Duration: duration,
	}
}

func readTeardownBudget(path string) time.Duration {
	if path == "" {
		return GracefulShutdownDelay
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return GracefulShutdownDelay
	}
	d, err := time.ParseDuration(strings.TrimSpace(string(data)))
	if err != nil || d <= 0 {
		return GracefulShutdownDelay
	}
	return d
}

func StripTrailingStatus(data []byte) []byte {
	s := bytes.TrimRight(data, "\n")
	idx := bytes.LastIndex(s, []byte("\n"))
	if idx < 0 {
		return nil
	}
	lastLine := string(s[idx+1:])
	if lastLine == "PASS" || lastLine == "FAIL" {
		return s[:idx+1]
	}
	return data
}

func WritePackageSummary(pkg string, failed bool, d time.Duration, verbose bool) {
	if failed {
		fmt.Fprintf(os.Stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, d.Seconds())
	} else if verbose {
		fmt.Fprintf(os.Stdout, "PASS\nok  \t%s\t%.3fs\n", pkg, d.Seconds())
	} else {
		fmt.Fprintf(os.Stdout, "ok  \t%s\t%.3fs\n", pkg, d.Seconds())
	}
}

// WriteTrailingFail prints a bare FAIL line to stdout, matching what
// `go test` emits after all package results when any package fails.
func WriteTrailingFail() {
	fmt.Fprintln(os.Stdout, "FAIL")
}

// WriteNoTestFiles prints the `go test` annotation for packages with
// no test files: `?   \tpkg\t[no test files]`.
func WriteNoTestFiles(pkg string) {
	fmt.Fprintf(os.Stdout, "?   \t%s\t[no test files]\n", pkg)
}

// WriteJSONPackageSummary emits the JSON output event for the package
// summary line that `go test -json` includes. It writes a single
// Output action containing the `ok` or `FAIL` summary text.
func WriteJSONPackageSummary(pkg string, failed bool, d time.Duration) {
	var line string
	if failed {
		line = fmt.Sprintf("FAIL\t%s\t%.3fs\n", pkg, d.Seconds())
	} else {
		line = fmt.Sprintf("ok  \t%s\t%.3fs\n", pkg, d.Seconds())
	}
	evt := jsonOutputEvent{
		Time:    time.Now(),
		Action:  "output",
		Package: pkg,
		Output:  line,
	}
	data, _ := json.Marshal(evt)
	data = append(data, '\n')
	os.Stdout.Write(data)
}

type jsonOutputEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Output  string    `json:"Output,omitempty"`
}

// BuildSuiteTargets constructs SuiteTarget entries from compiled binaries
// and suite names. suitesByPkg maps import path to a list of suite struct
// names (e.g., "FooTestSuite"). The generated test function name is
// "Test" + suite struct name.
//
// If userRunFilter is non-empty, only suites whose test function name
// matches the filter regex are included.
func BuildSuiteTargets(compiled []CompileResult, suitesByPkg map[string][]string, dirsByPkg map[string]string, runFlags []string, userRunFilter string) []SuiteTarget {
	binByPkg := make(map[string]string, len(compiled))
	for _, cr := range compiled {
		binByPkg[cr.Package] = cr.BinaryPath
	}

	translatedFlags := TranslateToTestBinaryFlags(runFlags)

	var targets []SuiteTarget
	for pkg, suites := range suitesByPkg {
		bin, ok := binByPkg[pkg]
		if !ok {
			continue
		}

		pkgDir := dirsByPkg[pkg]

		for _, suiteName := range suites {
			testFuncName := "Test" + suiteName
			if userRunFilter != "" && !matchesSuiteFunc(userRunFilter, testFuncName) {
				continue
			}
			target := SuiteTarget{
				Package:    pkg,
				Dir:        pkgDir,
				BinaryPath: bin,
				SuiteName:  testFuncName,
				RunFlags:   translatedFlags,
			}
			if rf := suiteRunFilter(userRunFilter, testFuncName); rf != "" {
				target.RunFilter = rf
			}
			targets = append(targets, target)
		}
	}
	return targets
}

// matchesSuiteFunc checks if the user's -run regex could match a given
// test function name. The first segment (before /) of the regex is tested
// against the function name.
func matchesSuiteFunc(runRegex string, funcName string) bool {
	parts := strings.SplitN(runRegex, "/", 2)
	topLevel := parts[0]
	re, err := regexp.Compile(topLevel)
	if err != nil {
		return true // let the test binary report the invalid regex
	}
	return re.MatchString(funcName)
}

// suiteRunFilter extracts the portion of userRunFilter that applies to
// testFuncName and includes a subtest component. Returns empty string when
// the filter has no subtest part (suite-name-only matching suffices).
func suiteRunFilter(userRunFilter, testFuncName string) string {
	if userRunFilter == "" || !strings.Contains(userRunFilter, "/") {
		return ""
	}

	alts := splitTopLevelOr(userRunFilter)
	if len(alts) == 1 {
		return userRunFilter
	}

	var matching []string
	for _, alt := range alts {
		if matchesSuiteFunc(alt, testFuncName) {
			matching = append(matching, alt)
		}
	}
	if len(matching) == 0 {
		return ""
	}
	return strings.Join(matching, "|")
}

// splitTopLevelOr splits a regex pattern on | that is not inside parentheses
// or character classes.
func splitTopLevelOr(pattern string) []string {
	var result []string
	depth := 0
	start := 0
	escaped := false
	inBracket := false

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '[' && !inBracket {
			inBracket = true
			continue
		}
		if c == ']' && inBracket {
			inBracket = false
			continue
		}
		if inBracket {
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				result = append(result, pattern[start:i])
				start = i + 1
			}
		}
	}
	return append(result, pattern[start:])
}
