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

type CompileOutcome struct {
	Result CompileResult
	Err    error
}

func compilePackage(ctx context.Context, pkgPath, overlayFlag string, buildFlags []string, binDir string) (CompileResult, error) {
	binaryName := sanitizePkgName(pkgPath) + ".test"
	binaryPath := filepath.Join(binDir, binaryName)

	args := []string{"test", "-c", overlayFlag, "-o", binaryPath}
	args = append(args, buildFlags...)
	args = append(args, pkgPath)

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stderr = os.Stderr

	mp := NewManagedProcess(cmd, ProcessConfig{Grace: GraceKill})
	if err := mp.Start(); err != nil {
		return CompileResult{}, fmt.Errorf("compile %s: %w", pkgPath, err)
	}
	if err := mp.WaitWithGrace(ctx); err != nil {
		return CompileResult{}, fmt.Errorf("compile %s: %w", pkgPath, err)
	}

	return CompileResult{Package: pkgPath, BinaryPath: binaryPath}, nil
}

func CompilePackages(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string) ([]CompileResult, error) {
	ch := CompilePackagesStream(ctx, packages, overlayFlag, buildFlags, outputDir)
	var results []CompileResult
	var errs []error
	for outcome := range ch {
		if outcome.Err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", outcome.Err)
			errs = append(errs, outcome.Err)
			continue
		}
		results = append(results, outcome.Result)
	}
	return results, errors.Join(errs...)
}

func CompilePackagesStream(ctx context.Context, packages []string, overlayFlag string, buildFlags []string, outputDir string) <-chan CompileOutcome {
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
		sem := make(chan struct{}, runtime.NumCPU())

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
				outcome := CompileOutcome{Result: cr, Err: err}
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
