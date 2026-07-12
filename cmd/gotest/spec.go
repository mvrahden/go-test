package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	ownArgs, goTestArgs, err := SplitArgs(inv.DefaultArgs(), specAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	format := extractStringFlag(ownArgs, "--format", "terminal")
	output := extractStringFlag(ownArgs, "--output", "")
	input := extractStringFlag(ownArgs, "--input", "")
	noColor := hasFlag(ownArgs, "--no-color")

	if input != "" {
		return runSpecFromInput(input, format, output, noColor)
	}

	minCoverage, err := parseMinFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	goTestArgs, coverProfile, coverCleanup, err := ensureCoverProfile(goTestArgs, minCoverage)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer coverCleanup()

	cfg, err := parseExecFlags(ownArgs, goTestArgs, inv.Config)
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

	if cfg.CI {
		if code, err := enforceFocusGuard(loaded); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		} else if code != 0 {
			return code
		}
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug, cfg.NoCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer cancel()

	if cfg.GlobalTimeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer timeoutCancel()
	}

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		Streaming:       false,
		OutputMode:      gotestrunner.RunCaptureJSON,
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

	events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	var w io.Writer = os.Stdout
	if output != "" {
		f, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer f.Close()
		w = f
	}

	var renderOpts []gotestspec.RenderOption
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	case "json":
		gotestspec.RenderJSON(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree, renderOpts...)
	}

	return enforceCoverage(coverProfile, minCoverage, code)
}

func runSpecFromInput(input, format, output string, noColor bool) int {
	var r io.Reader
	if input == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: opening input file: %s\n", err)
			return 2
		}
		defer f.Close()
		r = f
	}

	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}

	tree := gotestspec.BuildTree(events)

	var w io.Writer = os.Stdout
	if output != "" {
		of, ferr := os.Create(output)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", ferr)
			return 2
		}
		defer of.Close()
		w = of
	}

	var renderOpts []gotestspec.RenderOption
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree)
	case "json":
		gotestspec.RenderJSON(w, tree)
	default:
		gotestspec.RenderTerminal(w, tree, renderOpts...)
	}

	return 0
}
