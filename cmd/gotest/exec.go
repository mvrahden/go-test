package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

func Run(cfg ExecConfig) int { //nolint:gocritic // hugeParam: stable API
	code, _, _, phases := execRun(cfg)
	if phases != "" {
		fmt.Fprintln(os.Stderr, phases)
	}
	return code
}

// execRun is Run with what the pipeline judged, nil when it did not run, and
// the --debug line of where its time went, "" without --debug, for the
// caller to print last.
func execRun(cfg ExecConfig) (exit int, judged *gotestrunner.PipelineResult, plan *runPlan, phases string) { //nolint:gocritic // hugeParam: stable API
	clock := phaseClock{start: time.Now()}
	if cfg.Debug {
		// Deferred first, so it is taken last: after the cleanup and the stop.
		defer func() { phases = fmt.Sprint(clock.phases(time.Now())) }()
	}
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	clock.discovered = time.Now()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, nil, nil, ""
	}

	if refuseUnfailed(cfg.Only, pkgs.Order) {
		return 2, nil, nil, ""
	}

	// The focus guard sees every package given, those --changed leaves out
	// among them.
	if code, err := focusGate(pkgs.Loaded, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, nil, nil, ""
	} else if code != 0 {
		return code, nil, nil, ""
	}

	mode, manifest := modeFromJSON(cfg.JSON), cfg.Manifest
	if manifest == nil {
		manifest = manifestIf(cfg.JSON)
		cfg.Manifest = manifest
	}
	plan, ok := planRun(&cfg, &pkgs)
	if !ok {
		return 2, nil, nil, ""
	}
	loaded, broken, noTestFiles := pkgs.Loaded, pkgs.Broken, pkgs.NoTestFiles

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	clock.generated = time.Now()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, nil, nil, ""
	}
	defer cleanup()

	ctx, stop := shutdownContext(cfg.Control)
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	var stdout io.Writer
	if cfg.Agent != nil {
		// An agent's text is rendered from the stream, so the census and
		// the counts are the -json run's.
		mode, stdout = gotestrunner.RunStreamJSON, cfg.Agent
	}

	clock.piped = time.Now()
	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      mode,
		Events:          cfg.Events,
		GlobalTimeout:   cfg.GlobalTimeout,
		NoTestFiles:     noTestFiles,
		PackageOrder:    pkgs.Order,
		Groups:          cfg.Groups,
		SkipGroups:      cfg.SkipGroups,
		Manifest:        manifest,
		Only:            cfg.Only,
		Stdout:          stdout,
		Inputs:          plan.inputs.stamp(),
		Live:            cfg.Live,
	}, overlay)
	clock.ran, clock.compile = time.Now(), result.CompileSpan
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, nil, nil, ""
	}
	// stderr is free in every output mode, so the notes about what did not
	// run reach the reader whether stdout carries text or JSON.
	noteStdlibTests(overlay.Plan.StdlibTests())
	gotestrunner.NoteBenchmarksNotRun(os.Stderr, cfg.GoTestArgs, len(overlay.Declared.Benchmarks))
	return result.ExitCode, &result, plan, ""
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

// manifestIf is the run's manifest when its stream is asked for, else nil.
func manifestIf(stream bool) *protocol.Manifest {
	if !stream {
		return nil
	}
	return runManifest()
}

func modeFromJSON(jsonMode bool) gotestrunner.RunMode {
	if jsonMode {
		return gotestrunner.RunStreamJSON
	}
	return gotestrunner.RunBatchText
}
