package gotestrunner

import (
	"bytes"
	"context"
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
// suites for a package have completed. Results are stored at fixed indices
// to guarantee deterministic output order regardless of goroutine scheduling.
//
// All methods must be called under a single external mutex.
type PackageBatcher struct {
	pkgs map[string]*pkgBatch
}

type pkgBatch struct {
	expected  int
	completed int
	results   []SuiteResult
}

func NewPackageBatcher() *PackageBatcher {
	return &PackageBatcher{pkgs: map[string]*pkgBatch{}}
}

// Register prepares the batcher for a package with count suites.
func (b *PackageBatcher) Register(pkg string, count int) {
	b.pkgs[pkg] = &pkgBatch{
		expected: count,
		results:  make([]SuiteResult, count),
	}
}

// Record stores a result at position idx within its package.
// Returns true when all suites for that package are now complete.
func (b *PackageBatcher) Record(pkg string, idx int, r SuiteResult) bool {
	s := b.pkgs[pkg]
	s.results[idx] = r
	s.completed++
	return s.completed == s.expected
}

// Flush writes the completed package's output to stdout: each suite's output
// with trailing status stripped, followed by the package summary line.
func (b *PackageBatcher) Flush(pkg string) {
	s := b.pkgs[pkg]
	pkgFailed := false
	var pkgDuration time.Duration
	for _, pr := range s.results {
		os.Stdout.Write(StripTrailingStatus(pr.Stdout))
		if len(pr.Stderr) > 0 {
			os.Stderr.Write(pr.Stderr)
		}
		if pr.ExitCode != 0 {
			pkgFailed = true
		}
		pkgDuration += pr.Duration
	}
	WritePackageSummary(pkg, pkgFailed, pkgDuration)
}

// RunSuites executes each suite target in its own subprocess with bounded
// concurrency. The mode parameter controls output format and delivery.
// The returned []byte is non-nil only for RunCaptureJSON.
func RunSuites(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int, mode RunMode) ([]byte, int) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	useTest2JSON := mode != RunBatchText

	var batcher *PackageBatcher
	var localIdx []int
	if mode == RunBatchText {
		batcher = NewPackageBatcher()
		pkgCount := map[string]int{}
		localIdx = make([]int, len(targets))
		for i, t := range targets {
			localIdx[i] = pkgCount[t.Package]
			pkgCount[t.Package]++
		}
		for pkg, count := range pkgCount {
			batcher.Register(pkg, count)
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
			sem <- struct{}{}
			defer func() { <-sem }()
			r := RunSingleSuite(ctx, t, env, useTest2JSON)

			mu.Lock()
			if r.ExitCode > worstCode {
				worstCode = r.ExitCode
			}
			switch mode {
			case RunBatchText:
				if batcher.Record(t.Package, localIdx[idx], r) {
					batcher.Flush(t.Package)
				}
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
		cmd.Dir = target.Dir
		return cmd
	}

	cmd := exec.CommandContext(ctx, target.BinaryPath, testArgs...)
	cmd.Env = env
	cmd.Dir = target.Dir
	return cmd
}

// RunSingleSuite executes a single suite subprocess.
// When test2json is true, the binary is wrapped with `go tool test2json`.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string, test2json bool) SuiteResult {
	cmd := buildSuiteCmd(ctx, target, env, test2json)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
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

func WritePackageSummary(pkg string, failed bool, d time.Duration) {
	if failed {
		fmt.Fprintf(os.Stdout, "FAIL\nFAIL\t%s\t%.3fs\n", pkg, d.Seconds())
	} else {
		fmt.Fprintf(os.Stdout, "PASS\nok  \t%s\t%.3fs\n", pkg, d.Seconds())
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
// Non-suite test functions are discovered via -test.list and grouped into
// a single additional target per package so they are not silently skipped.
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

		suiteSet := make(map[string]bool, len(suites))
		for _, suiteName := range suites {
			suiteSet["Test"+suiteName] = true
		}

		allFuncs := listTestFuncs(bin)
		var standalone []string
		for _, fn := range allFuncs {
			if suiteSet[fn] {
				continue
			}
			if userRunFilter != "" && !matchesSuiteFunc(userRunFilter, fn) {
				continue
			}
			standalone = append(standalone, fn)
		}
		if len(standalone) > 0 {
			escaped := make([]string, len(standalone))
			for i, fn := range standalone {
				escaped[i] = regexp.QuoteMeta(fn)
			}
			pattern := "^(" + strings.Join(escaped, "|") + ")$"
			targets = append(targets, SuiteTarget{
				Package:    pkg,
				Dir:        pkgDir,
				BinaryPath: bin,
				SuiteName:  "(standalone)",
				RunFilter:  pattern,
				RunFlags:   translatedFlags,
			})
		}

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

func listTestFuncs(binaryPath string) []string {
	cmd := exec.Command(binaryPath, "-test.list", ".*")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var funcs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			funcs = append(funcs, line)
		}
	}
	return funcs
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
