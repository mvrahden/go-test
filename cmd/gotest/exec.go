package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
)

func Run(cfg ExecConfig) int { //nolint:gocritic // hugeParam: stable API
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	loaded, broken, noTestFiles := pkgs.Loaded, pkgs.Broken, pkgs.NoTestFiles
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if cfg.CI {
		if code, err := enforceFocusGuard(loaded); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		} else if code != 0 {
			return code
		}
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, stop := shutdownContext()
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      modeFromJSON(cfg.JSON),
		Events:          cfg.Events,
		FuzzFuncsByPkg:  overlay.FuzzFuncsByPkg,
		GlobalTimeout:   cfg.GlobalTimeout,
		NoTestFiles:     noTestFiles,
		PackageOrder:    pkgs.Order,
		Groups:          cfg.Groups,
		SkipGroups:      cfg.SkipGroups,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// stderr is free in every output mode, so the notes about what did not
	// run reach the reader whether stdout carries text or JSON.
	noteStdlibTests(overlay.StdlibTestsByPkg)
	gotestrunner.NoteBenchmarksNotRun(os.Stderr, cfg.GoTestArgs, len(overlay.Declared.Benchmarks))
	return result.ExitCode
}

// noteStdlibTests says on stderr how many stdlib tests the run left to go test.
func noteStdlibTests(stdlibTestsByPkg map[string]int) {
	if total, names := sumStdlibTests(stdlibTestsByPkg); total > 0 {
		fmt.Fprintf(os.Stderr, "note: %d stdlib test(s) in %s not run — gotest runs suites; use 'go test' for stdlib tests\n", total, names)
	}
}

// loadRunPackages loads a run's packages and, as go test does, warns on
// stderr about each pattern that matched none while others did.
func loadRunPackages(patterns, loadFlags []string) (gotestgen.RunPackages, error) {
	pkgs, err := gotestgen.LoadRunPackages(patterns, loadFlags)
	for _, p := range pkgs.Unmatched {
		fmt.Fprintf(os.Stderr, "warning: %q matched no packages\n", p)
	}
	return pkgs, err
}

// warnMatchedNothing warns on stderr, as the go command does, about patterns
// that together matched no package.
func warnMatchedNothing(patterns []string) {
	for _, p := range patterns {
		fmt.Fprintf(os.Stderr, "warning: %q matched no packages\n", p)
	}
}

// sumStdlibTests totals the stdlib tests gotest reports but does not run, and
// names the affected packages (truncated) so the note is actionable.
func sumStdlibTests(byPkg map[string]int) (int, string) {
	total := 0
	pkgs := make([]string, 0, len(byPkg))
	for pkg, n := range byPkg {
		total += n
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	if len(pkgs) > 3 {
		pkgs = append(pkgs[:3], fmt.Sprintf("… %d more", len(byPkg)-3))
	}
	return total, strings.Join(pkgs, ", ")
}

func modeFromJSON(jsonMode bool) gotestrunner.RunMode {
	if jsonMode {
		return gotestrunner.RunStreamJSON
	}
	return gotestrunner.RunBatchText
}
