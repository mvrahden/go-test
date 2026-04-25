package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func Run(cfg ExecConfig) int {
	if CI {
		violations, err := RunFocusGuard(cfg.PackagePatterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var allDirs []string
	for _, pattern := range cfg.PackagePatterns {
		dirs, err := gotestrunner.SuitesGenerate(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allDirs = append(allDirs, dirs...)
	}

	if !DEBUG {
		defer cleanupGeneratedFiles(allDirs)
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	code, err := gotestrunner.StdlibRunTests(cfg.GoTestArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func cleanupGeneratedFiles(dirs []string) {
	for _, dir := range dirs {
		os.Remove(filepath.Join(dir, about.PSuite))
		os.Remove(filepath.Join(dir, about.PXSuite))
	}
}
