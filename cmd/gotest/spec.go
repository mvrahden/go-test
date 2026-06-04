package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(inv Invocation) int {
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

	setupTimeout, err := parseSetupTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	minCoverage, err := parseMinFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	parallel, err := parseParallelFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if parallel == 0 {
		parallel = inv.Config.Parallel
	}

	var coverProfile string
	if minCoverage > 0 {
		for _, arg := range goTestArgs {
			if v, ok := strings.CutPrefix(arg, "-coverprofile="); ok {
				coverProfile = v
			}
		}
	}

	patterns := ExtractPackagePatterns(goTestArgs)

	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
		Parallel:        parallel,
	}

	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, err := gotestgen.LoadPackages(patterns, loadFlags)
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

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, cfg.Debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, cancel := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer cancel()

	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		Streaming:       false,
		OutputMode:      gotestrunner.RunCaptureJSON,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	code := result.ExitCode

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

	if code == 0 && minCoverage > 0 && coverProfile != "" {
		pct, err := readCoverageTotal(coverProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: reading coverage: %s\n", err)
			return 2
		}
		if pct < float64(minCoverage) {
			fmt.Fprintf(os.Stderr, "\nFAIL: %.1f%% coverage (minimum %d%%)\n", pct, minCoverage)
			return 1
		}
	}

	return code
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

