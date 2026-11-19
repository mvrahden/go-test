package gotestrunner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/protocol"
)

// CompileResult holds the result of compiling a single test package.
type CompileResult struct {
	Package    string // import path
	BinaryPath string // path to compiled test binary
}

type CompileOutcome struct {
	Package string
	Result  CompileResult
	Err     error
}

// BuildFailure is a per-package compile failure. A package that fails to
// compile is a failed package, not an aborted run: the caller books it as a
// verdict and keeps running the packages that did compile. An empty Package
// marks a failure of the compile stage itself rather than of one package.
type BuildFailure struct {
	Package string
	Err     error
}

// goTestC runs one go command for the compile stage; tests replace it.
var goTestC = func(ctx context.Context, args, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = stdout, stderr
	mp := NewManagedProcess(cmd, ProcessConfig{Grace: GraceKill})
	if err := mp.Start(); err != nil {
		return err
	}
	return mp.WaitWithGrace(ctx)
}

// goCacheDir is `go env GOCACHE`, asked once per process; "" when it
// cannot be read.
var goCacheDir = sync.OnceValue(func() string {
	out, err := exec.Command("go", "env", "GOCACHE").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
})

func compileError(pkgPath string, err error, diagnostics []byte) error {
	diagnostics = bytes.TrimRight(diagnostics, "\n")
	if len(diagnostics) == 0 {
		return fmt.Errorf("compile %s: %w", pkgPath, err)
	}
	if advice := vendorAdvice(diagnostics); advice != "" {
		return fmt.Errorf("compile %s: %w\n%s\n%s", pkgPath, err, diagnostics, advice)
	}
	return fmt.Errorf("compile %s: %w\n%s", pkgPath, err, diagnostics)
}

// vendorAdvice names the fix for a vendor directory that lacks the runtime:
// only the generated harness imports it, and go mod vendor copies it only
// for a module that declares the CLI as a tool.
func vendorAdvice(diagnostics []byte) string {
	pkg := about.Repo + "/pkg/gotestruntime"
	for line := range bytes.SplitSeq(diagnostics, []byte("\n")) {
		if bytes.Contains(line, []byte(pkg)) && bytes.Contains(line, []byte("-mod=vendor")) {
			return fmt.Sprintf("gotest: vendor/ lacks %[1]s, which the generated harness imports. Declare the CLI as a tool (go get -tool %[2]s/cmd/gotest@<the version go.mod requires>) and run go mod vendor, or run with -mod=mod.", pkg, about.Repo)
		}
	}
	return ""
}

func CompilePackages(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir, cacheDir string, compileParallel int) ([]CompileResult, []BuildFailure) {
	return collectOutcomes(CompilePackagesStream(ctx, packages, overlayFlag, buildFlags, outputDir, cacheDir, compileParallel))
}

// collectOutcomes reads ch to its close: the binaries that built, and the
// failures by package.
func collectOutcomes(ch <-chan CompileOutcome) ([]CompileResult, []BuildFailure) {
	var results []CompileResult
	var failures []BuildFailure
	for outcome := range ch {
		if outcome.Err != nil {
			failures = append(failures, BuildFailure{Package: outcome.Package, Err: outcome.Err})
			continue
		}
		results = append(results, outcome.Result)
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Package < failures[j].Package })
	return results, failures
}

func compileConcurrency(compileParallel int, buildFlags []string) int {
	n := compileParallel
	if n <= 0 {
		n = runtime.NumCPU()
		if SanitizerActive(buildFlags) {
			n = max(1, n/2)
		}
	}
	if cap := ExtractProcessCap(buildFlags); cap > 0 {
		n = min(n, cap)
	}
	return n
}

// SanitizerActive reports whether buildFlags enable an instrumentation mode
// (-race/-msan/-asan) that at least doubles the CPU cost per instruction
// stream. Compile and dispatch concurrency halve their defaults under it:
// the ordinary defaults are tuned for uninstrumented workloads, and keeping
// them would oversubscribe the machine — starving exactly the wall-clock
// budget verdicts that must stay trustworthy. An explicit --parallel or
// --compile-parallel always wins over this heuristic.
func SanitizerActive(buildFlags []string) bool {
	for _, f := range buildFlags {
		if f == "-race" || f == "-msan" || f == "-asan" {
			return true
		}
	}
	return false
}

func CompilePackagesStream(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir, cacheDir string, compileParallel int) <-chan CompileOutcome {
	ch := make(chan CompileOutcome)

	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		go func() {
			ch <- CompileOutcome{Err: fmt.Errorf("create bin dir: %w", err)}
			close(ch)
		}()
		return ch
	}

	emit := func(o CompileOutcome) {
		select {
		case ch <- o:
		case <-ctx.Done():
		}
	}
	goTmp := filepath.Join(outputDir, "gotmp")
	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		var alone atomic.Int64
		sem := make(chan struct{}, compileConcurrency(compileParallel, buildFlags))
		// build runs one go command for group in stageDir, then builds each
		// package it left without a verdict alone.
		var build func(group []string, stageDir string)
		build = func(group []string, stageDir string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			left := compileGroup(ctx, group, overlayFlag, buildFlags, stageDir, goTmp, cacheDir, emit)
			<-sem
			for _, pkg := range left {
				wg.Add(1)
				go build([]string{pkg}, filepath.Join(binDir, fmt.Sprintf("s%d", alone.Add(1))))
			}
		}
		for n, group := range compileGroups(packages, runtime.GOOS) {
			wg.Add(1)
			go build(group, filepath.Join(binDir, strconv.Itoa(n)))
		}
		wg.Wait()
	}()

	return ch
}

// buildSet is the members of suitePackages, in order, that dispatched holds:
// a package none of whose suites runs is not built.
func buildSet(suitePackages []string, dispatched map[string]bool) []string {
	var out []string
	for _, pkg := range suitePackages {
		if dispatched[pkg] {
			out = append(out, pkg)
		}
	}
	return out
}

// dispatchedPackages is the set of packages the given suites belong to.
func dispatchedPackages(suites []*PlannedSuite) map[string]bool {
	out := make(map[string]bool, len(suites))
	for _, s := range suites {
		out[s.Package] = true
	}
	return out
}

// testBinaryName is the file go test -c -o <dir>/ writes for importPath: its
// last element, or the one before a major-version element such as v2, then
// .test and the platform's executable suffix.
func testBinaryName(importPath, goos string) string {
	name := importPath
	if i := strings.LastIndex(importPath, "/"); i >= 0 {
		name = importPath[i+1:]
		if isVersionElement(name) {
			rest := importPath[:i]
			name = rest[strings.LastIndex(rest, "/")+1:]
		}
	}
	name += ".test"
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// isVersionElement reports whether elem is v followed by a number of at
// least 2 without a leading zero.
func isVersionElement(elem string) bool {
	if len(elem) < 2 || elem[0] != 'v' || elem[1] == '0' || elem == "v1" {
		return false
	}
	for _, c := range elem[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// compileGroups deals pkgs, in order, into the first group that does not
// hold their binary's name yet: go test -c refuses two packages with one
// name in one call. Names are compared case-folded, for case-insensitive
// filesystems.
func compileGroups(pkgs []string, goos string) [][]string {
	var groups [][]string
	var names []map[string]bool
	for _, pkg := range pkgs {
		name := strings.ToLower(testBinaryName(pkg, goos))
		i := slices.IndexFunc(names, func(held map[string]bool) bool { return !held[name] })
		if i < 0 {
			i = len(groups)
			groups = append(groups, nil)
			names = append(names, map[string]bool{})
		}
		groups[i] = append(groups[i], pkg)
		names[i][name] = true
	}
	return groups
}

// spanOf passes ch's outcomes on and measures the compile stage: span is
// the time from this call to ch's close, zero until then.
func spanOf(ch <-chan CompileOutcome, now func() time.Time) (<-chan CompileOutcome, func() time.Duration) {
	out := make(chan CompileOutcome)
	var mu sync.Mutex
	var d time.Duration
	start := now()
	go func() {
		defer close(out)
		for o := range ch {
			out <- o
		}
		end := now()
		mu.Lock()
		d = end.Sub(start)
		mu.Unlock()
	}()
	return out, func() time.Duration {
		mu.Lock()
		defer mu.Unlock()
		return d
	}
}

func sanitizePkgName(pkgPath string) string {
	h := sha256.Sum256([]byte(pkgPath))
	parts := strings.Split(pkgPath, "/")
	short := parts[len(parts)-1]
	return fmt.Sprintf("%s_%x", short, h[:4])
}

// defaultSlowBuildThreshold is when a single build is called out as
// unexpectedly long. Deliberately a log line, never a verdict: a first-ever
// -race build recompiles the stdlib's instrumented variant and legitimately
// runs for minutes — a hard cap here would be a number the user did not choose.
const defaultSlowBuildThreshold = 15 * time.Second

// slowBuildThreshold is GOTEST_SLOW_BUILD_THRESHOLD as a Go duration; unset,
// unparsable or not positive, it is the default.
func slowBuildThreshold() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(protocol.EnvSlowBuildThreshold)); err == nil && d > 0 {
		return d
	}
	return defaultSlowBuildThreshold
}

// logSlowBuild arms a one-shot notice for a long-running build and returns a
// done func: past threshold it logs that the build is still running, and on
// completion past threshold it logs the effective duration — so slowness is
// visible without ever being failed.
func logSlowBuild(w io.Writer, what string, threshold time.Duration) (done func()) {
	start := time.Now()
	fired := make(chan struct{})
	timer := time.AfterFunc(threshold, func() {
		defer close(fired)
		fmt.Fprintf(w, "gotest: %s has been building for %s and is still running\n", what, threshold)
	})
	return func() {
		notified := !timer.Stop()
		if notified {
			// The notice is firing on the timer goroutine; wait it out so the
			// two writes never overlap on a non-concurrent-safe writer.
			<-fired
		}
		// A notice is always closed: Windows' clock moves in ticks, so
		// time.Since can read below a threshold the timer has passed.
		if d := time.Since(start); notified || d >= threshold {
			fmt.Fprintf(w, "gotest: %s finished building after %s\n", what, d.Round(time.Second))
		}
	}
}
