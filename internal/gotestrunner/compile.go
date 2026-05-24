package gotestrunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// CompileResult holds the result of compiling a single test package.
type CompileResult struct {
	Package    string // import path
	BinaryPath string // path to compiled test binary
}

// CompilePackages compiles test binaries for the given packages using
// `go test -c`. Compilation runs concurrently with a limit of maxProcs.
// The overlay flag (e.g., "-overlay=/tmp/gotest/overlay.json") is passed
// to include generated test code.
func CompilePackages(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string) ([]CompileResult, error) {
	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("create bin dir: %w", err)
	}

	type result struct {
		cr  CompileResult
		err error
	}

	results := make([]result, len(packages))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())

	for i, pkg := range packages {
		wg.Add(1)
		go func(idx int, pkgPath string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			binaryName := sanitizePkgName(pkgPath) + ".test"
			binaryPath := filepath.Join(binDir, binaryName)

			args := []string{"test", "-c", overlayFlag, "-o", binaryPath}
			args = append(args, buildFlags...)
			args = append(args, pkgPath)

			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Stderr = os.Stderr
			SetProcessGroup(cmd)
			cmd.WaitDelay = BuildShutdownDelay

			if err := cmd.Run(); err != nil {
				results[idx] = result{err: fmt.Errorf("compile %s: %w", pkgPath, err)}
				return
			}

			results[idx] = result{cr: CompileResult{Package: pkgPath, BinaryPath: binaryPath}}
		}(i, pkg)
	}
	wg.Wait()

	var compiled []CompileResult
	var errs []error
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", r.err)
			errs = append(errs, r.err)
			continue
		}
		compiled = append(compiled, r.cr)
	}
	return compiled, errors.Join(errs...)
}

// CompilePackagesStream is like CompilePackages but sends results to a
// channel as each package finishes compiling, enabling execution to overlap
// with compilation. The channel is closed when all packages are done.
func CompilePackagesStream(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string) <-chan CompileResult {
	ch := make(chan CompileResult)

	binDir := filepath.Join(outputDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "create bin dir: %s\n", err)
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		sem := make(chan struct{}, runtime.NumCPU())

		for _, pkg := range packages {
			wg.Add(1)
			go func(pkgPath string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()

				binaryName := sanitizePkgName(pkgPath) + ".test"
				binaryPath := filepath.Join(binDir, binaryName)

				args := []string{"test", "-c", overlayFlag, "-o", binaryPath}
				args = append(args, buildFlags...)
				args = append(args, pkgPath)

				cmd := exec.CommandContext(ctx, "go", args...)
				cmd.Stderr = os.Stderr
				SetProcessGroup(cmd)
				cmd.WaitDelay = BuildShutdownDelay

				if err := cmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "compile %s: %s\n", pkgPath, err)
					return
				}

				select {
				case ch <- CompileResult{Package: pkgPath, BinaryPath: binaryPath}:
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
