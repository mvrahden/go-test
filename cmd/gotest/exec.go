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
	suitesByPkg    map[string][]string // import path → suite struct names
}

func generateOverlay(patterns []string, debug bool) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateWithSharedFixtures(patterns, nil)
	if err != nil {
		return nil, nil, err
	}
	loaded, err := gotestgen.LoadPackages(patterns, nil)
	if err != nil {
		return nil, nil, err
	}
	suitesByPkg, err := collectSuiteNames(loaded)
	if err != nil {
		return nil, nil, err
	}
	return buildOverlay(allResults, allSharedFixtures, suitesByPkg, debug)
}

func generateOverlayFromLoaded(loaded []*gotestgen.LoadResult, debug bool) (*overlayResult, func(), error) {
	allResults, allSharedFixtures, err := gotestgen.GenerateFromLoaded(loaded)
	if err != nil {
		return nil, nil, err
	}
	suitesByPkg, err := collectSuiteNames(loaded)
	if err != nil {
		return nil, nil, err
	}
	return buildOverlay(allResults, allSharedFixtures, suitesByPkg, debug)
}

func collectSuiteNames(loaded []*gotestgen.LoadResult) (map[string][]string, error) {
	allSuites, err := gotestgen.CollectFromLoaded(loaded)
	if err != nil {
		return nil, err
	}
	seen := map[string]map[string]bool{}
	suitesByPkg := map[string][]string{}
	for _, s := range allSuites {
		pkgPath := s.Package().PkgPath
		pkgPath = strings.TrimSuffix(pkgPath, "_test")
		if seen[pkgPath] == nil {
			seen[pkgPath] = map[string]bool{}
		}
		id := s.Identifier()
		if seen[pkgPath][id] {
			continue
		}
		seen[pkgPath][id] = true
		suitesByPkg[pkgPath] = append(suitesByPkg[pkgPath], id)
	}
	return suitesByPkg, nil
}

func buildOverlay(allResults gotestgen.GenerateResults, allSharedFixtures []gotestgen.SharedFixtureInfo, suitesByPkg map[string][]string, debug bool) (*overlayResult, func(), error) {
	tmpDir, err := gotestrunner.WriteOverlay(allResults)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if debug {
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
		suitesByPkg:    suitesByPkg,
	}, cleanup, nil
}

func buildExtraEnv(cfg ExecConfig, proc *SharedFixtureProcess) map[string]string {
	env := make(map[string]string)
	if cfg.UpdateSnapshots {
		env["GOTEST_UPDATE_SNAPSHOTS"] = "1"
	}
	if proc != nil {
		env["GOTEST_SHARED_STATE_FILE"] = proc.StateFile()
	}
	return env
}

func executeTests(ctx context.Context, cfg ExecConfig, overlay *overlayResult) (int, error) {
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	userRunFilter := gotestrunner.ExtractRunFilter(classified.RunFlags)
	runFlags := gotestrunner.StripRunFilter(classified.RunFlags)

	compiled, err := gotestrunner.CompilePackages(ctx, overlay.suitePackages, overlay.overlayFlag, classified.BuildFlags, overlay.tmpDir)
	if err != nil {
		return 2, err
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
		if err != nil {
			return 2, fmt.Errorf("shared fixture setup: %w", err)
		}
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return 130, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, runFlags, userRunFilter)

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no test suites to run")
		return 0, nil
	}

	if cfg.Spec {
		return runWithSpec(ctx, targets, extraEnv), nil
	}

	_, code := gotestrunner.RunSuites(ctx, targets, extraEnv, 0)
	return code, nil
}

func executeTestsJSON(ctx context.Context, cfg ExecConfig, overlay *overlayResult) (int, error) {
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	userRunFilter := gotestrunner.ExtractRunFilter(classified.RunFlags)
	runFlags := gotestrunner.StripRunFilter(classified.RunFlags)

	compiled, err := gotestrunner.CompilePackages(ctx, overlay.suitePackages, overlay.overlayFlag, classified.BuildFlags, overlay.tmpDir)
	if err != nil {
		return 2, err
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
		if err != nil {
			return 2, fmt.Errorf("shared fixture setup: %w", err)
		}
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return 130, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, runFlags, userRunFilter)

	if len(targets) == 0 {
		return 0, nil
	}

	_, code := gotestrunner.RunSuitesTest2JSON(ctx, targets, extraEnv, 0)
	return code, nil
}

func executeTestsCaptured(ctx context.Context, cfg ExecConfig, overlay *overlayResult) ([]byte, int, error) {
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	userRunFilter := gotestrunner.ExtractRunFilter(classified.RunFlags)
	runFlags := gotestrunner.StripRunFilter(classified.RunFlags)

	compiled, err := gotestrunner.CompilePackages(ctx, overlay.suitePackages, overlay.overlayFlag, classified.BuildFlags, overlay.tmpDir)
	if err != nil {
		return nil, 2, err
	}

	var setupProc *SharedFixtureProcess
	if len(overlay.sharedFixtures) > 0 {
		setupProc, err = startSharedFixtures(ctx, overlay.tmpDir, overlay.sharedFixtures, cfg.SetupTimeout)
		if err != nil {
			return nil, 2, fmt.Errorf("shared fixture setup: %w", err)
		}
		defer setupProc.Teardown()
	}

	select {
	case <-ctx.Done():
		return nil, 130, nil
	default:
	}

	extraEnv := buildExtraEnv(cfg, setupProc)
	runFlags = append(runFlags, "-v")
	targets := gotestrunner.BuildSuiteTargets(compiled, overlay.suitesByPkg, runFlags, userRunFilter)

	if len(targets) == 0 {
		return nil, 0, nil
	}

	data, code := gotestrunner.RunSuitesJSON(ctx, targets, extraEnv, 0)
	return data, code, nil
}

func Run(cfg ExecConfig) int {
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if cfg.CI {
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

	overlay, cleanup, err := generateOverlayFromLoaded(loaded, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var code int
	var execErr error
	if cfg.JSON {
		code, execErr = executeTestsJSON(ctx, cfg, overlay)
	} else {
		code, execErr = executeTests(ctx, cfg, overlay)
	}
	if execErr != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", execErr)
		return 2
	}
	return code
}

func runWithSpec(ctx context.Context, targets []gotestrunner.SuiteTarget, extraEnv map[string]string) int {
	// Ensure -test.v for JSON parsing
	for i := range targets {
		hasV := false
		for _, f := range targets[i].RunFlags {
			if f == "-test.v" {
				hasV = true
				break
			}
		}
		if !hasV {
			targets[i].RunFlags = append(targets[i].RunFlags, "-test.v")
		}
	}

	jsonData, code := gotestrunner.RunSuitesJSON(ctx, targets, extraEnv, 0)

	events, err := gotestspec.ParseEvents(bytes.NewReader(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	for _, ev := range events {
		if ev.Output != "" {
			fmt.Print(ev.Output)
		}
	}

	fmt.Println()
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(os.Stdout, tree)

	return code
}
