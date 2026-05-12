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
	Package    string   // import path (for reporting)
	BinaryPath string   // path to compiled test binary
	SuiteName  string   // test function name, e.g., "TestFooTestSuite"
	RunFilter  string   // raw -test.run value (overrides SuiteName if set)
	RunFlags   []string // test binary flags (with -test. prefix)
}

// SuiteResult holds the output from running a single suite subprocess.
type SuiteResult struct {
	Target   SuiteTarget
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Duration time.Duration
}

// RunSuites executes each suite target in its own subprocess with bounded
// concurrency. Output is streamed per-package: when all suites for a package
// complete, their output is written to stdout followed by a package summary.
func RunSuites(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int) ([]SuiteResult, int) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	pkgSuiteCount := map[string]int{}
	pkgIndices := map[string][]int{}
	for i, t := range targets {
		pkgSuiteCount[t.Package]++
		pkgIndices[t.Package] = append(pkgIndices[t.Package], i)
	}

	results := make([]SuiteResult, len(targets))
	pkgCompleted := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
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
			r := RunSingleSuite(ctx, t, env)

			mu.Lock()
			results[idx] = r
			pkgCompleted[t.Package]++
			if pkgCompleted[t.Package] == pkgSuiteCount[t.Package] {
				pkgFailed := false
				var pkgDuration time.Duration
				for _, j := range pkgIndices[t.Package] {
					pr := results[j]
					os.Stdout.Write(StripTrailingStatus(pr.Stdout))
					if len(pr.Stderr) > 0 {
						os.Stderr.Write(pr.Stderr)
					}
					if pr.ExitCode != 0 {
						pkgFailed = true
					}
					pkgDuration += pr.Duration
				}
				WritePackageSummary(t.Package, pkgFailed, pkgDuration)
			}
			mu.Unlock()
		}(i, target)
	}
	wg.Wait()

	worstCode := 0
	for _, r := range results {
		if r.ExitCode > worstCode {
			worstCode = r.ExitCode
		}
	}

	return results, worstCode
}

// RunSuitesJSON is like RunSuites but captures output for programmatic
// consumption. Each suite's output is merged as it completes.
func RunSuitesJSON(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int) ([]byte, int) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	results := make([]SuiteResult, len(targets))
	var mu sync.Mutex
	var merged bytes.Buffer
	var wg sync.WaitGroup
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
			r := RunSingleSuite(ctx, t, env)

			mu.Lock()
			results[idx] = r
			merged.Write(r.Stdout)
			mu.Unlock()
		}(i, target)
	}
	wg.Wait()

	worstCode := 0
	for _, r := range results {
		if r.ExitCode > worstCode {
			worstCode = r.ExitCode
		}
	}
	return merged.Bytes(), worstCode
}

// RunSingleSuite executes a single suite subprocess.
// Exported for use by the streaming execution pipeline.
func RunSingleSuite(ctx context.Context, target SuiteTarget, env []string) SuiteResult {
	args := make([]string, 0, len(target.RunFlags)+1)
	if target.RunFilter != "" {
		args = append(args, "-test.run="+target.RunFilter)
	} else {
		args = append(args, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	args = append(args, target.RunFlags...)

	cmd := exec.CommandContext(ctx, target.BinaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = env

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

// RunSuitesTest2JSON executes each suite target wrapped with `go tool test2json`
// to produce proper JSON test events. Each suite's events are written to stdout
// as soon as the suite completes, enabling real-time progress in the test explorer.
func RunSuitesTest2JSON(ctx context.Context, targets []SuiteTarget, extraEnv map[string]string, maxParallel int) ([]SuiteResult, int) {
	if maxParallel <= 0 {
		maxParallel = 2 * runtime.GOMAXPROCS(0)
	}

	results := make([]SuiteResult, len(targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
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
			r := RunSingleSuiteTest2JSON(ctx, t, env)

			mu.Lock()
			results[idx] = r
			os.Stdout.Write(r.Stdout)
			if len(r.Stderr) > 0 {
				os.Stderr.Write(r.Stderr)
			}
			mu.Unlock()
		}(i, target)
	}
	wg.Wait()

	worstCode := 0
	for _, r := range results {
		if r.ExitCode > worstCode {
			worstCode = r.ExitCode
		}
	}

	return results, worstCode
}

// RunSingleSuiteTest2JSON executes a single suite via `go tool test2json`.
// Exported for use by the streaming execution pipeline.
func RunSingleSuiteTest2JSON(ctx context.Context, target SuiteTarget, env []string) SuiteResult {
	var testArgs []string
	if target.RunFilter != "" {
		testArgs = append(testArgs, "-test.run="+target.RunFilter)
	} else {
		testArgs = append(testArgs, fmt.Sprintf("-test.run=^%s$", regexp.QuoteMeta(target.SuiteName)))
	}
	testArgs = append(testArgs, "-test.v=test2json")
	for _, f := range target.RunFlags {
		if f == "-test.v" || strings.HasPrefix(f, "-test.v=") {
			continue
		}
		testArgs = append(testArgs, f)
	}

	args := []string{"tool", "test2json", "-p", target.Package, "-t", target.BinaryPath}
	args = append(args, testArgs...)

	cmd := exec.CommandContext(ctx, "go", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = env

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
func BuildSuiteTargets(compiled []CompileResult, suitesByPkg map[string][]string, runFlags []string, userRunFilter string) []SuiteTarget {
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
