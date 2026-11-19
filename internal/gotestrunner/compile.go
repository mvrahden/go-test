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
	"sort"
	"strings"
	"sync"
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

// compilePackage captures the compiler's stderr into the returned error
// instead of streaming it: the diagnostics belong to the failing package's
// verdict, and only the collector can place them there. On success any
// captured stderr (toolchain notices, module downloads) is forwarded.
//
// With a cache dir the link targets the cached path, where go skips it when
// the binary is up to date, and the result is copied to binDir for this run.
// A cache that cannot be locked is treated as absent: it is an optimization.
func compilePackage(ctx context.Context, pkgPath, overlayFlag string, buildFlags []string, binDir, cacheDir string) (CompileResult, error) {
	binaryName := sanitizePkgName(pkgPath) + ".test"
	binaryPath := filepath.Join(binDir, binaryName)
	outputPath := binaryPath
	if cacheDir != "" {
		cached := filepath.Join(cacheDir, binaryName)
		if unlock, err := lockFile(cached + ".lock"); err == nil {
			defer unlock()
			outputPath = cached
		}
	}

	args := []string{"test", "-c", overlayFlag, "-o", outputPath}
	args = append(args, buildFlags...)
	args = append(args, pkgPath)

	defer logSlowBuild(os.Stderr, "test binary for "+pkgPath, slowBuildThreshold())()

	link := func() error {
		cmd := exec.CommandContext(ctx, "go", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		mp := NewManagedProcess(cmd, ProcessConfig{Grace: GraceKill})
		if err := mp.Start(); err != nil {
			return compileError(pkgPath, err, stderr.Bytes())
		}
		if err := mp.WaitWithGrace(ctx); err != nil {
			return compileError(pkgPath, err, stderr.Bytes())
		}
		if stderr.Len() > 0 {
			_, _ = os.Stderr.Write(stderr.Bytes())
		}
		return nil
	}
	if outputPath == binaryPath {
		if err := link(); err != nil {
			return CompileResult{}, err
		}
		return CompileResult{Package: pkgPath, BinaryPath: binaryPath}, nil
	}

	// The go command touches an up-to-date target and keeps its build id
	// through truncation, so the bytes tell a skipped link from a new one:
	// unchanged, they must match the record of the last link.
	before, beforeErr := sumFile(outputPath)
	if err := link(); err != nil {
		return CompileResult{}, err
	}
	after, err := copyFileSum(outputPath, binaryPath)
	if err != nil {
		return CompileResult{}, compileError(pkgPath, err, nil)
	}
	record, recorded := readSum(outputPath)
	if beforeErr == nil && before == after && (!recorded || record != after) {
		_ = os.Remove(outputPath)
		if err := link(); err != nil {
			return CompileResult{}, err
		}
		if after, err = copyFileSum(outputPath, binaryPath); err != nil {
			return CompileResult{}, compileError(pkgPath, err, nil)
		}
		if recorded {
			fmt.Fprintf(os.Stderr, "gotest: the cached test binary of %s did not match its record and was linked again\n", pkgPath)
		}
	}
	if !recorded || record != after {
		_ = writeSum(outputPath, after) // the cache is an optimization
	}

	return CompileResult{Package: pkgPath, BinaryPath: binaryPath}, nil
}

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
// only the generated harness imports it, so go mod vendor never copied it.
func vendorAdvice(diagnostics []byte) string {
	pkg := about.Repo + "/pkg/gotestruntime"
	for line := range bytes.SplitSeq(diagnostics, []byte("\n")) {
		if bytes.Contains(line, []byte(pkg)) && bytes.Contains(line, []byte("-mod=vendor")) {
			return fmt.Sprintf("gotest: vendor/ lacks %[1]s, which the generated harness imports and no file of this module does. Add a blank import of it to a test file (import _ %[1]q) and run go mod vendor, or run with -mod=mod.", pkg)
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

	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		sem := make(chan struct{}, compileConcurrency(compileParallel, buildFlags))

		for _, pkg := range packages {
			wg.Add(1)
			go func(pkgPath string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}

				cr, err := compilePackage(ctx, pkgPath, overlayFlag, buildFlags, binDir, cacheDir)
				outcome := CompileOutcome{Package: pkgPath, Result: cr, Err: err}
				select {
				case ch <- outcome:
				case <-ctx.Done():
				}
			}(pkg)
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
