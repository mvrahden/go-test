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
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSummary(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	ownArgs, goTestArgs, err := SplitArgs(inv.DefaultArgs(), summaryAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	format := extractStringFlag(ownArgs, "--format", "terminal")
	output := extractStringFlag(ownArgs, "--output", "")
	input := extractStringFlag(ownArgs, "--input", "")
	coverageProfile := extractStringFlag(ownArgs, "--coverage", "")
	noColor := hasFlag(ownArgs, "--no-color")
	github := hasFlag(ownArgs, "--github") || os.Getenv("GITHUB_ACTIONS") == "true"

	if input != "" {
		return runSummaryFromInput(input, format, output, coverageProfile, noColor, github)
	}

	setupTimeout, err := parseSetupTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	globalTimeout, err := parseGlobalTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	globalTimeout = resolveGlobalTimeout(globalTimeout)
	minCoverage, err := parseMinFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if minCoverage == 0 {
		minCoverage = inv.Config.MinCoverage
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
		if coverProfile == "" {
			f, ferr := os.CreateTemp("", "gotest-cover-*.out")
			if ferr != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", ferr)
				return 2
			}
			coverProfile = f.Name()
			f.Close()
			defer os.Remove(coverProfile)
			goTestArgs = append(goTestArgs, "-coverprofile="+coverProfile)
		}
	}

	patterns := ExtractPackagePatterns(goTestArgs)

	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		GlobalTimeout:   globalTimeout,
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
		NoCache:         slices.Contains(ownArgs, "--no-cache"),
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

	pipelineStart := time.Now()
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

	if coverageProfile == "" {
		for _, arg := range goTestArgs {
			if v, ok := strings.CutPrefix(arg, "-coverprofile="); ok {
				coverageProfile = v
			}
		}
	}

	elapsed := time.Since(pipelineStart)
	writeSummaryOutput(tree, format, output, coverageProfile, noColor, github, elapsed)

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

func runSummaryFromInput(input, format, output, coverageProfile string, noColor, github bool) int {
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

	writeSummaryOutput(tree, format, output, coverageProfile, noColor, github, 0)

	stats := gotestspec.CollectStats(tree)
	if stats.Failed > 0 {
		return 1
	}
	return 0
}

func writeSummaryOutput(tree []*gotestspec.Package, format, output, coverageProfile string, noColor, github bool, elapsed time.Duration) {
	var w io.Writer = os.Stdout
	var closeFunc func()
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", err)
			return
		}
		closeFunc = func() { f.Close() }
		w = f
	}
	if closeFunc != nil {
		defer closeFunc()
	}

	var renderOpts []gotestspec.RenderOption
	if elapsed > 0 {
		renderOpts = append(renderOpts, gotestspec.WithElapsed(elapsed))
	}
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	if coverageProfile != "" {
		report, err := gotestspec.ParseCoverageProfile(coverageProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: reading coverage profile: %s\n", err)
		} else {
			renderOpts = append(renderOpts, gotestspec.WithCoverage(report))
		}
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdownSummary(w, tree, renderOpts...)
	case "json":
		gotestspec.RenderJSON(w, tree)
	default:
		gotestspec.RenderSummary(w, tree, renderOpts...)
	}

	if github {
		modulePath := gotestspec.ReadModulePath(".")
		annotations := gotestspec.CollectAnnotations(tree, modulePath)
		gotestspec.WriteGitHubAnnotations(os.Stdout, annotations)

		if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
			sf, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				gotestspec.RenderMarkdownSummary(sf, tree, renderOpts...)
				sf.Close()
			}
		}
	}
}
