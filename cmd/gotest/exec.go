package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
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

	// 1. Generate test code
	var allDirs []string
	var allCollectorResults []gotestgen.CollectorResult
	for _, pattern := range cfg.PackagePatterns {
		dirs, collectorResults, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allDirs = append(allDirs, dirs...)
		allCollectorResults = append(allCollectorResults, collectorResults...)
	}

	if !DEBUG {
		defer cleanupGeneratedFiles(allDirs)
	}

	// 2. Discover shared fixtures from collector results
	sharedFixtures := gotestgen.DiscoverSharedFixtures(allCollectorResults)

	// 3. If any shared fixtures, start setup subprocess
	var setupProc *SharedFixtureProcess
	if len(sharedFixtures) > 0 {
		var err error
		setupProc, err = startSharedFixtures(ctx, sharedFixtures)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: shared fixture setup: %s\n", err)
			return 2
		}
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return 130
	default:
	}

	// 4. Run tests with env vars from shared fixtures
	var extraEnv map[string]string
	if setupProc != nil {
		extraEnv = setupProc.Env()
	}
	code, err := gotestrunner.StdlibRunTests(cfg.GoTestArgs, extraEnv)
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
