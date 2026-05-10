package gotestrunner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	sem := make(chan struct{}, 4)

	for i, pkg := range packages {
		wg.Add(1)
		go func(idx int, pkgPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			binaryName := sanitizePkgName(pkgPath) + ".test"
			binaryPath := filepath.Join(binDir, binaryName)

			args := []string{"test", "-c", overlayFlag, "-o", binaryPath}
			args = append(args, buildFlags...)
			args = append(args, pkgPath)

			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				results[idx] = result{err: fmt.Errorf("compile %s: %w", pkgPath, err)}
				return
			}

			results[idx] = result{cr: CompileResult{Package: pkgPath, BinaryPath: binaryPath}}
		}(i, pkg)
	}
	wg.Wait()

	var compiled []CompileResult
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", r.err)
			continue
		}
		compiled = append(compiled, r.cr)
	}
	return compiled, nil
}

func sanitizePkgName(pkgPath string) string {
	h := sha256.Sum256([]byte(pkgPath))
	parts := strings.Split(pkgPath, "/")
	short := parts[len(parts)-1]
	return fmt.Sprintf("%s_%x", short, h[:4])
}
