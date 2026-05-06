package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

type overlayResult struct {
	tmpDir         string
	overlayFlag    string
	sharedFixtures []gotestgen.SharedFixtureInfo
	suitePackages  []string
}

func generateOverlay(patterns []string) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateWithSharedFixtures(patterns, nil)
	if err != nil {
		return nil, nil, err
	}
	return buildOverlay(allResults, allSharedFixtures)
}

func generateOverlayFromLoaded(loaded []*gotestgen.LoadResult) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}
	return buildOverlay(allResults, allSharedFixtures)
}

func buildOverlay(allResults gotestgen.GenerateResults, allSharedFixtures []gotestgen.SharedFixtureInfo) (*overlayResult, func(), error) {
	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if DEBUG {
		fmt.Fprintf(os.Stderr, "DEBUG: overlay dir: %s\n", tmpDir)
	} else {
		cleanup = func() { os.RemoveAll(tmpDir) }
	}

	var suitePkgs []string
	for _, r := range allResults {
		if len(r.PTest) > 0 || len(r.PXTest) > 0 {
			suitePkgs = append(suitePkgs, r.Package)
		}
	}

	return &overlayResult{
		tmpDir:         tmpDir,
		overlayFlag:    "-overlay=" + filepath.Join(tmpDir, "overlay.json"),
		sharedFixtures: allSharedFixtures,
		suitePackages:  suitePkgs,
	}, cleanup, nil
}

func buildExtraEnv() map[string]string {
	env := make(map[string]string)
	if UPDATE_SNAPSHOTS {
		env["GOTEST_UPDATE_SNAPSHOTS"] = "1"
	}
	return env
}

func Run(cfg ExecConfig) int {
	var overlay *overlayResult
	var cleanup func()

	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if CI {
		suites, err := gotestgen.CollectFromLoaded(loaded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		violations := CheckFocusViolations(suites)
		if len(violations) > 0 {
			fmt.Fprintln(os.Stderr, "FAIL: focus prefix detected — remove F_ before merging:")
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v.String())
			}
			return 1
		}
	}

	{
		var err error
		overlay, cleanup, err = generateOverlayFromLoaded(loaded)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	cfg.GoTestArgs = resolveWildcardArgs(cfg.GoTestArgs, cfg.PackagePatterns, loaded, overlay)

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer cleanup()

	goTestArgs := append([]string{overlay.overlayFlag}, cfg.GoTestArgs...)

	// If any shared fixtures, start setup subprocess
	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		var err error
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
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

	extraEnv := buildExtraEnv()
	if setupProc != nil {
		extraEnv["GOTEST_SHARED_STATE_FILE"] = setupProc.StateFile()
	}

	if SPEC {
		return runWithSpec(ctx, goTestArgs, extraEnv)
	}

	code, err := gotestrunner.StdlibRunTests(ctx, goTestArgs, extraEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return code
}

func resolveWildcardArgs(goTestArgs []string, patterns []string, loaded []*gotestgen.LoadResult, overlay *overlayResult) []string {
	hasWildcard := false
	for _, p := range patterns {
		if strings.HasSuffix(p, "/...") || p == "./..." || p == "..." {
			hasWildcard = true
			break
		}
	}
	if !hasWildcard {
		return goTestArgs
	}

	suitePkgs := make(map[string]bool, len(overlay.suitePackages))
	for _, pkg := range overlay.suitePackages {
		suitePkgs[pkg] = true
	}

	allHaveSuites := len(loaded) > 0
	for _, lr := range loaded {
		if !suitePkgs[lr.PkgPath] {
			allHaveSuites = false
			break
		}
	}
	if allHaveSuites {
		return goTestArgs
	}

	var result []string
	seenArgs := false
	for _, arg := range goTestArgs {
		if arg == "-args" {
			seenArgs = true
		}
		if !seenArgs && looksLikePackagePattern(arg) && isWildcard(arg) {
			continue
		}
		result = append(result, arg)
	}
	for _, pkg := range overlay.suitePackages {
		result = append(result, pkg)
	}
	return result
}

func isWildcard(s string) bool {
	return strings.HasSuffix(s, "/...") || s == "./..." || s == "..."
}

func runWithSpec(ctx context.Context, goTestArgs []string, extraEnv map[string]string) int {
	jsonData, code, err := gotestrunner.StdlibRunTestsJSON(ctx, goTestArgs, extraEnv)
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
