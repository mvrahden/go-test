package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// runBench runs BenchmarkX wrapper functions for suites containing
// BenchmarkX methods, always dispatching them serially (never concurrently)
// so timing results stay meaningful. It mirrors runTest's compact flow
// (SplitArgs -> ClassifyGoTestArgs -> LoadPackages -> GenerateOverlay ->
// RunPipeline) with Bench: true.
func runBench(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	ownArgs, goTestArgs, err := SplitArgs(inv.DefaultArgs(), benchAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	specRequested := hasFlag(ownArgs, "--spec")
	noColor := hasFlag(ownArgs, "--no-color")

	// Unlike ordinary tests, benchmark results are the point of running
	// gotest bench at all: stdlib `go test -bench=.` always prints result
	// lines regardless of -v. Our batch collector, however, suppresses a
	// suite's stdout unless verbose or failed (mirroring go test's PASS
	// suppression for ordinary tests). Force verbose so ns/op lines always
	// show, matching go test's actual benchmark UX.
	if !gotestrunner.HasVerboseFlag(goTestArgs) {
		goTestArgs = append(goTestArgs, "-v")
	}

	cfg, err := parseExecFlags(ownArgs, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug, cfg.NoCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	if len(overlay.BenchesByPkg) == 0 {
		fmt.Println("no benchmarks found")
		return 0
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	mode := gotestrunner.RunBatchText
	if specRequested {
		mode = gotestrunner.RunCaptureJSON
	}

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		Streaming:       false,
		OutputMode:      mode,
		Bench:           true,
		BenchesByPkg:    overlay.BenchesByPkg,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	code := result.ExitCode
	if cfg.GlobalTimeout > 0 && ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "FAIL: global --timeout exceeded after %v\n", cfg.GlobalTimeout)
		if code == 0 {
			code = 1
		}
	}

	if specRequested {
		events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
			return 2
		}
		tree := gotestspec.BuildTree(events)

		var renderOpts []gotestspec.RenderOption
		if noColor {
			renderOpts = append(renderOpts, gotestspec.WithNoColor())
		}
		gotestspec.RenderTerminal(os.Stdout, tree, renderOpts...)
	}

	return code
}
