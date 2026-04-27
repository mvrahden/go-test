package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
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
	var allResults gotestgen.GenerateResults
	var allCollectorResults []gotestgen.CollectorResult
	for _, pattern := range cfg.PackagePatterns {
		results, collectorResults, err := gotestrunner.SuitesGenerateWithCollectorResults(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		allResults = append(allResults, results...)
		allCollectorResults = append(allCollectorResults, collectorResults...)
	}

	// 2. Write overlay
	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		defer os.RemoveAll(tmpDir)
	}

	goTestArgs := append([]string{"-overlay=" + filepath.Join(tmpDir, "overlay.json")}, cfg.GoTestArgs...)

	// 3. Discover shared fixtures from collector results
	sharedFixtures := gotestgen.DiscoverSharedFixtures(allCollectorResults)

	// 4. If any shared fixtures, start setup subprocess
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

	// 5. Run tests with env vars from shared fixtures
	extraEnv := make(map[string]string)
	if setupProc != nil {
		for k, v := range setupProc.Env() {
			extraEnv[k] = v
		}
	}
	if UPDATE_SNAPSHOTS {
		extraEnv["GOTEST_UPDATE_SNAPSHOTS"] = "1"
	}

	if SPEC {
		return runWithSpec(goTestArgs, extraEnv)
	}

	code, err := gotestrunner.StdlibRunTests(goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func runWithSpec(goTestArgs []string, extraEnv map[string]string) int {
	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	// Replay normal output
	for _, ev := range events {
		if ev.Output != "" {
			fmt.Print(ev.Output)
		}
	}

	// Render spec view
	fmt.Println()
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(os.Stdout, tree)

	return code
}
