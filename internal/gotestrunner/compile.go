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
func compilePackage(ctx context.Context, pkgPath, overlayFlag string, buildFlags []string, binDir string) (CompileResult, error) {
	binaryName := sanitizePkgName(pkgPath) + ".test"
	binaryPath := filepath.Join(binDir, binaryName)

	args := []string{"test", "-c", overlayFlag, "-o", binaryPath}
	args = append(args, buildFlags...)
	args = append(args, pkgPath)

	cmd := exec.CommandContext(ctx, "go", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	defer logSlowBuild(os.Stderr, "test binary for "+pkgPath, slowBuildThreshold)()

	mp := NewManagedProcess(cmd, ProcessConfig{Grace: GraceKill})
	if err := mp.Start(); err != nil {
		return CompileResult{}, compileError(pkgPath, err, stderr.Bytes())
	}
	if err := mp.WaitWithGrace(ctx); err != nil {
		return CompileResult{}, compileError(pkgPath, err, stderr.Bytes())
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}

	return CompileResult{Package: pkgPath, BinaryPath: binaryPath}, nil
}

func compileError(pkgPath string, err error, diagnostics []byte) error {
	diagnostics = bytes.TrimRight(diagnostics, "\n")
	if len(diagnostics) == 0 {
		return fmt.Errorf("compile %s: %w", pkgPath, err)
	}
	return fmt.Errorf("compile %s: %w\n%s", pkgPath, err, diagnostics)
}

func CompilePackages(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string, compileParallel int) ([]CompileResult, []BuildFailure) {
	ch := CompilePackagesStream(ctx, packages, overlayFlag, buildFlags, outputDir, compileParallel)
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
	if compileParallel > 0 {
		return compileParallel
	}
	n := runtime.NumCPU()
	if SanitizerActive(buildFlags) {
		return max(1, n/2)
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

func CompilePackagesStream(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string, compileParallel int) <-chan CompileOutcome {
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

				cr, err := compilePackage(ctx, pkgPath, overlayFlag, buildFlags, binDir)
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

func sanitizePkgName(pkgPath string) string {
	h := sha256.Sum256([]byte(pkgPath))
	parts := strings.Split(pkgPath, "/")
	short := parts[len(parts)-1]
	return fmt.Sprintf("%s_%x", short, h[:4])
}

// slowBuildThreshold is when a single build is called out as unexpectedly
// long. Deliberately a log line, never a verdict: a first-ever -race build
// recompiles the stdlib's instrumented variant and legitimately runs for
// minutes — a hard cap here would be a number the user did not choose.
const slowBuildThreshold = 15 * time.Second

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
		if !timer.Stop() {
			// The notice is firing on the timer goroutine; wait it out so the
			// two writes never overlap on a non-concurrent-safe writer.
			<-fired
		}
		if d := time.Since(start); d >= threshold {
			fmt.Fprintf(w, "gotest: %s finished building after %s\n", what, d.Round(time.Second))
		}
	}
}
