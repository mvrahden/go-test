package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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
	renderOnly := hasFlag(ownArgs, "--render-only")

	if hasFlag(ownArgs, "--static") && input != "" {
		// One asks for the specification in the source, the other for the
		// specification in a recorded run. Silently honouring --input would
		// hand back a tree the caller did not ask for.
		fmt.Fprintf(os.Stderr, "FAIL: --static and --input are mutually exclusive\n")
		return 2
	}

	if renderOnly && input == "" {
		// Suppressing the verdict of a real run would turn a red pipeline
		// green. The flag only makes sense where the exit code describes a
		// rendering, not a test run.
		fmt.Fprintf(os.Stderr, "FAIL: --render-only requires --input\n")
		return 2
	}

	if input != "" {
		return runSpecFromInput(input, format, output, noColor, renderOnly)
	}

	if hasFlag(ownArgs, "--static") {
		// The specification is written in the source; reading it should not
		// require executing it.
		return runStaticSpec(ownArgs, goTestArgs, &inv.Config, format, output, noColor)
	}

	minCoverage, err := parseMinFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if minCoverage == 0 && inv.Config.MinCoverage > 0 {
		minCoverage = inv.Config.MinCoverage
	}

	goTestArgs, coverProfile, coverCleanup, err := ensureCoverProfile(goTestArgs, minCoverage > 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer coverCleanup()

	cfg, err := parseExecFlags(ownArgs, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	loaded, broken, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
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

	ctx, cancel := shutdownContext()
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
		OutputMode:      gotestrunner.RunCaptureJSON,
		FuzzFuncsByPkg:  overlay.FuzzFuncsByPkg,
		GlobalTimeout:   cfg.GlobalTimeout,
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
	gotestrunner.NoteBenchmarksNotRun(os.Stderr, goTestArgs, len(overlay.Declared.Benchmarks))

	tree := gotestspec.BuildTree(events, gotestspec.WithDeclarations(buildDeclarationIndex(loaded)))

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

func runSpecFromInput(input, format, output string, noColor, renderOnly bool) int {
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

	tree := gotestspec.BuildTree(events, gotestspec.WithDeclarations(declarationsForStream(events)))

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

	// The same exit rule as `summary --input`: a renderer fed a failing stream
	// must not exit 0 — in a pipe without pipefail this exit code is the only
	// verdict CI ever sees. A client that renders rather than gates opts out
	// with --render-only; every operational failure above still exits 2.
	return replayExitCode(events, tree, renderOnly)
}

// replayExitCode is the verdict of a replayed stream: 2 when it holds no
// verdict to report (see replayVerdictGap), 1 on failures, else 0.
// renderOnly drops the verdict entirely.
func replayExitCode(events []gotestspec.TestEvent, tree []*gotestspec.Package, renderOnly bool) int {
	if renderOnly {
		return 0
	}
	if gap := replayVerdictGap(events); gap != "" {
		fmt.Fprintf(os.Stderr, "FAIL: %s", gap)
		return 2
	}
	if gotestspec.HasFailures(tree) {
		return 1
	}
	return 0
}

// replayVerdictGap says why a replayed stream carries no verdict: it holds
// no test events at all (empty or not go test -json), or it ends before a
// package reported its own pass/fail/skip (cut short). "" means it is whole.
func replayVerdictGap(events []gotestspec.TestEvent) string {
	seen := false
	var order []string
	finished := map[string]bool{}
	for i := range events {
		ev := &events[i]
		if ev.Action == "" {
			continue
		}
		seen = true
		if ev.Package == "" { // build events name an ImportPath instead
			continue
		}
		if _, ok := finished[ev.Package]; !ok {
			order = append(order, ev.Package)
			finished[ev.Package] = false
		}
		if ev.Test == "" {
			switch ev.Action {
			case gotestspec.ActionPass, gotestspec.ActionFail, gotestspec.ActionSkip:
				finished[ev.Package] = true
			}
		}
	}
	if !seen {
		return "input holds no test events (expected a go test -json stream)\n"
	}
	var b strings.Builder
	for _, pkg := range order {
		if !finished[pkg] {
			fmt.Fprintf(&b, "  %s\n", pkg)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "input ends before these packages reported a verdict (stream cut short?):\n" + b.String()
}
