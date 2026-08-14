package gotestrunner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// SuiteSpec identifies a test suite by its package and name. It carries only
// the immutable identity fields — the "what to run" — without execution details.
type SuiteSpec struct {
	Package   string // import path (for reporting)
	Dir       string // package source directory (working dir for the binary)
	SuiteName string // test function name, e.g., "TestFooTestSuite"
	RunFilter string // raw -test.run value (overrides SuiteName if set)
}

// SuiteTarget identifies a single test suite (or group of standalone tests)
// to run in its own subprocess.
type SuiteTarget struct {
	SuiteSpec
	BinaryPath   string   // path to compiled test binary
	RunFlags     []string // test binary flags (with -test. prefix)
	CoverProfile string   // per-suite cover profile path (empty if no -coverprofile)
	BudgetFile   string   // sidecar path for teardown budget (empty = use default)
	Bench        bool     // when true, run the Benchmark<SuiteName> wrapper instead of the suite's tests
	BenchFilter  string   // raw -test.bench value carrying the user's sub-benchmark segments (empty = the exact Benchmark<SuiteName> wrapper)
	Exclusive    bool     // SuiteConfig{Exclusive: true}: dispatched strictly alone, after every non-exclusive suite
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

// RunSuites executes each suite target in its own subprocess with bounded
// concurrency. Results are recorded via the collector, which handles
// mode-specific output formatting, JSON event filtering, and package ordering.
// barrier, when non-nil, runs at the bulk→tail boundary — after every
// parallel suite has drained, before the first exclusive suite dispatches —
// so the caller can re-window shared fixtures.
func RunSuites(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int, collector *OutputCollector, barrier func()) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	pkgCount := map[string]int{}
	var pkgOrder []string
	localIdx := make([]int, len(targets))
	for i := range targets {
		if _, seen := pkgCount[targets[i].Package]; !seen {
			pkgOrder = append(pkgOrder, targets[i].Package)
		}
		localIdx[i] = pkgCount[targets[i].Package]
		pkgCount[targets[i].Package]++
	}
	for _, pkg := range pkgOrder {
		collector.Register(pkg, pkgCount[pkg])
	}

	useTest2JSON := collector.UsesTest2JSON()

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallel)

	env := os.Environ()
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}

	var exclusiveIdx []int
	for i, target := range targets { //nolint:gocritic // rangeValCopy: intentional
		if target.Exclusive {
			exclusiveIdx = append(exclusiveIdx, i)
			continue
		}
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
			collector.RecordResult(t.Package, localIdx[idx], r)
		}(i, target)
	}
	wg.Wait()

	if barrier != nil {
		barrier()
	}

	// Exclusive suites own the machine: strictly after the parallel bulk,
	// one at a time, in deterministic order. Their verdicts measure
	// wall-clock behavior — timing budgets, contended resources — that
	// concurrently running suites would corrupt.
	sortTargetIndices(targets, exclusiveIdx)
	for _, i := range exclusiveIdx {
		if ctx.Err() != nil {
			return
		}
		r := RunSingleSuite(ctx, targets[i], env, useTest2JSON)
		collector.RecordResult(targets[i].Package, localIdx[i], r)
	}
}

func buildSuiteCmd(ctx context.Context, target SuiteTarget, env []string, test2json bool) *exec.Cmd { //nolint:gocritic // hugeParam: stable API
	var testArgs []string
	switch {
	case target.Bench:
		testArgs = append(testArgs,
			"-test.run=^$",
			fmt.Sprintf("-test.bench=^Benchmark%s$", regexp.QuoteMeta(target.SuiteName)))
		if !slices.Contains(target.RunFlags, "-test.benchmem") {
			testArgs = append(testArgs, "-test.benchmem")
		}
	case target.RunFilter != "":
		testArgs = append(testArgs, "-test.run="+target.RunFilter)
	default:
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}

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
			cmd.Env = append(cmd.Env, protocol.EnvTeardownBudgetFile+"="+target.BudgetFile)
		}
		cmd.Dir = target.Dir
		return cmd
	}

	cmd := exec.CommandContext(ctx, target.BinaryPath, testArgs...) //nolint:gosec // G204: binary built by this tool, not user-supplied
	cmd.Env = env
	if target.BudgetFile != "" {
		cmd.Env = append(cmd.Env, protocol.EnvTeardownBudgetFile+"="+target.BudgetFile)
	}
	cmd.Dir = target.Dir
	return cmd
}

// RunSingleSuite executes a single suite subprocess.
// When test2json is true, the binary is wrapped with `go tool test2json`.
//
// On context cancellation, cmd.Cancel sends SIGTERM to the process group.
// A per-process kill timer (from the teardown budget file) then governs
// when SIGKILL is sent, rather than Go's built-in WaitDelay.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) SuiteResult { //nolint:gocritic // hugeParam: stable API
	cmd := buildSuiteCmd(ctx, target, env, test2json)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()

	mp := NewManagedProcess(cmd, ProcessConfig{
		Grace:      GraceBudget,
		BudgetFile: target.BudgetFile,
	})
	if err := mp.Start(); err != nil {
		return SuiteResult{Target: target, ExitCode: 2, Duration: time.Since(start)}
	}

	_ = mp.WaitWithGrace(ctx)
	duration := time.Since(start)

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
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
	switch {
	case failed:
		fmt.Fprintf(os.Stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, d.Seconds())
	case verbose:
		fmt.Fprintf(os.Stdout, "PASS\nok  \t%s\t%.3fs\n", pkg, d.Seconds())
	default:
		fmt.Fprintf(os.Stdout, "ok  \t%s\t%.3fs\n", pkg, d.Seconds())
	}
}

// BuildSuiteTargets constructs SuiteTarget entries from compiled binaries
// and suite names. suitesByPkg maps import path to a list of suite struct
// names (e.g., "FooTestSuite"). The generated test function name is
// "Test" + suite struct name.
//
// If userRunFilter is non-empty, only suites whose test function name
// matches the filter regex are included.
//
// exclusiveByPkg (import path → suite struct name → true) marks suites with
// SuiteConfig{Exclusive: true}; their targets carry Exclusive and are
// dispatched strictly alone, after every non-exclusive suite (see RunSuites).
func BuildSuiteTargets(compiled []CompileResult, suitesByPkg map[string][]string, dirsByPkg map[string]string, exclusiveByPkg map[string]map[string]bool, runFlags []string, userRunFilter string) []SuiteTarget {
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
				SuiteSpec: SuiteSpec{
					Package:   pkg,
					Dir:       pkgDir,
					SuiteName: testFuncName,
				},
				BinaryPath: bin,
				RunFlags:   translatedFlags,
				Exclusive:  exclusiveByPkg[pkg][suiteName],
			}
			if rf := suiteRunFilter(userRunFilter, testFuncName); rf != "" {
				target.RunFilter = rf
			}
			targets = append(targets, target)
		}
	}
	return targets
}

// BuildBenchTargets constructs SuiteTarget entries for benchmark suites from
// compiled binaries and bench-eligible suite names. benchesByPkg maps import
// path to a list of suite struct names (e.g., "FooTestSuite") that have at
// least one effective benchmark. The generated benchmark wrapper function
// name is "Benchmark" + suite struct name.
//
// userRunFilter (from -run) and userBenchFilter (from -bench) are applied
// independently, each matched against the benchmark wrapper name exactly as
// matchesSuiteFunc does elsewhere. When both are non-empty, a suite must
// satisfy both (AND semantics) to be included; either one alone filters on
// its own, and when both are empty all suites are included.
func BuildBenchTargets(compiled []CompileResult, benchesByPkg map[string][]string, dirsByPkg map[string]string, runFlags []string, userRunFilter, userBenchFilter string) []SuiteTarget {
	binByPkg := make(map[string]string, len(compiled))
	for _, cr := range compiled {
		binByPkg[cr.Package] = cr.BinaryPath
	}

	translatedFlags := TranslateToTestBinaryFlags(runFlags)

	var targets []SuiteTarget
	for pkg, suites := range benchesByPkg {
		bin, ok := binByPkg[pkg]
		if !ok {
			continue
		}

		pkgDir := dirsByPkg[pkg]

		for _, suiteName := range suites {
			benchFuncName := "Benchmark" + suiteName
			if userRunFilter != "" && !matchesSuiteFunc(userRunFilter, benchFuncName) {
				continue
			}
			if userBenchFilter != "" && !matchesSuiteFunc(userBenchFilter, benchFuncName) {
				continue
			}
			target := SuiteTarget{
				SuiteSpec: SuiteSpec{
					Package:   pkg,
					Dir:       pkgDir,
					SuiteName: suiteName,
				},
				BinaryPath: bin,
				RunFlags:   translatedFlags,
				Bench:      true,
			}
			targets = append(targets, target)
		}
	}
	return targets
}

// sortTargetIndices orders target indices by (Package, SuiteName) so
// exclusive dispatch is deterministic run over run.
func sortTargetIndices(targets []SuiteTarget, idx []int) {
	sort.Slice(idx, func(a, b int) bool {
		ta, tb := targets[idx[a]], targets[idx[b]]
		if ta.Package != tb.Package {
			return ta.Package < tb.Package
		}
		return ta.SuiteName < tb.SuiteName
	})
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
