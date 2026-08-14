package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"

	"github.com/mvrahden/go-test/internal/gotestbench"
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
	jsonRequested := hasFlag(ownArgs, "--json")

	// -v is forced further down for benchmark result visibility; capture
	// whether the caller actually asked for it first, since that's also our
	// signal for showing every delta row (vs. only significant ones) below.
	verboseRequested := gotestrunner.HasVerboseFlag(goTestArgs)

	saveTarget := extractStringFlag(ownArgs, "--save", "")
	if saveTarget == "" && hasFlag(ownArgs, "--save") {
		// `--save=` with no value asks for the configured baseline path —
		// the same fallback --against already has. Config resolution stays
		// in the CLI so tooling never parses .gotest.yml itself.
		saveTarget = inv.Config.Bench.Baseline
		if saveTarget == "" {
			fmt.Fprintln(os.Stderr, "FAIL: --save needs a path (or bench.baseline in .gotest.yml)")
			return 2
		}
	}

	againstPath := extractStringFlag(ownArgs, "--against", "")
	if againstPath == "" {
		againstPath = inv.Config.Bench.Baseline
	}

	gateGiven := hasFlag(ownArgs, "--gate")
	gatePct := inv.Config.Bench.Gate
	if gateGiven {
		raw := extractStringFlag(ownArgs, "--gate", "")
		v, perr := strconv.ParseFloat(raw, 64)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: invalid --gate value %q: %s\n", raw, perr)
			return 2
		}
		gatePct = v
	}
	gateActive := gateGiven || inv.Config.Bench.Gate > 0
	if gateActive && againstPath == "" {
		fmt.Fprintln(os.Stderr, "FAIL: --gate requires --against (or bench.baseline in .gotest.yml)")
		return 2
	}

	// --json needs the harvested results even without --save/--against, so it
	// rides the same capture path.
	benchAnalysisRequested := saveTarget != "" || againstPath != "" || jsonRequested

	// Unlike ordinary tests, benchmark results are the point of running
	// gotest bench at all: stdlib `go test -bench=.` always prints result
	// lines regardless of -v. Our batch collector, however, suppresses a
	// suite's stdout unless verbose or failed (mirroring go test's PASS
	// suppression for ordinary tests). Force verbose so ns/op lines always
	// show, matching go test's actual benchmark UX.
	if !verboseRequested {
		goTestArgs = append(goTestArgs, "-v")
	}

	cfg, err := parseExecFlags(ownArgs, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, broken, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// Timing numbers from a partially built tree are meaningless: fail fast
	// like generate/prepare rather than book-and-continue like run.
	if reportBrokenPackages(broken) {
		return 2
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	if len(overlay.BenchesByPkg) == 0 {
		if jsonRequested {
			// Machine consumers get a valid empty document, not prose.
			return emitBenchReport(gotestbench.FromPackages(nil), nil, nil)
		}
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
	if specRequested || benchAnalysisRequested {
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

	var tree []*gotestspec.Package
	if mode == gotestrunner.RunCaptureJSON {
		events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
			return 2
		}
		tree = gotestspec.BuildTree(events)
	}

	var newBaseline gotestbench.Baseline
	if benchAnalysisRequested {
		newBaseline = gotestbench.FromPackages(tree)
	}

	if saveTarget != "" {
		if err := gotestbench.Save(saveTarget, newBaseline); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	var deltas []gotestbench.Delta
	var specDeltas []gotestspec.BenchDelta

	if againstPath != "" {
		oldBaseline, err := gotestbench.Load(againstPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}

		deltas = gotestbench.Compare(oldBaseline, newBaseline)
		specDeltas = toSpecDeltas(filterDeltas(deltas, verboseRequested))
	}

	// Render the spec view exactly once. --spec/--save always want the full
	// tree; --against alone still needs it too, otherwise a bare
	// `gotest bench --against=X` shows nothing but a delta table (or, if no
	// delta is significant and -v wasn't passed, nothing at all) with zero
	// evidence any benchmark actually ran. The delta table (if a comparison
	// ran) renders as part of this same call, via WithBenchDeltas, rather
	// than a second stacked summary trailer. Under --json, stdout belongs to
	// the report document alone: the human rendering is suppressed entirely
	// (--spec included).
	if !jsonRequested && (specRequested || saveTarget != "" || againstPath != "") {
		var renderOpts []gotestspec.RenderOption
		if noColor {
			renderOpts = append(renderOpts, gotestspec.WithNoColor())
		}
		if againstPath != "" {
			renderOpts = append(renderOpts, gotestspec.WithBenchDeltas(specDeltas))
		}
		gotestspec.RenderTerminal(os.Stdout, tree, renderOpts...)
	}

	var gateVerdict *gotestbench.Gate
	if againstPath != "" && gateActive {
		verdict := gotestbench.GateVerdict(deltas, gatePct)
		gateVerdict = &verdict
		if verdict.Breached {
			fmt.Fprintf(os.Stderr, "bench gate: %s +%.1f%% exceeds %g%% gate\n", verdict.WorstKey, verdict.WorstPct, gatePct)
			if code == 0 {
				code = 1
			}
		}
	}

	if jsonRequested {
		if jsonCode := emitBenchReport(newBaseline, deltasForReport(deltas, againstPath), gateVerdict); jsonCode != 0 {
			return jsonCode
		}
	}

	// Mirror gotest summary --github's own $GITHUB_STEP_SUMMARY wiring:
	// under GitHub Actions, append a markdown rendering (with the delta
	// table, when --against ran) so bench results show up in the job
	// summary alongside the annotations/summary the "summary" subcommand
	// already writes there.
	if tree != nil && os.Getenv("GITHUB_ACTIONS") == "true" {
		if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
			sf, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				var mdOpts []gotestspec.RenderOption
				if againstPath != "" {
					mdOpts = append(mdOpts, gotestspec.WithBenchDeltas(specDeltas))
				}
				gotestspec.RenderMarkdownSummary(sf, tree, mdOpts...)
				sf.Close()
			}
		}
	}

	return code
}

// filterDeltas returns deltas as-is when showAll (set by -v) is true;
// otherwise it returns only the significant rows. Used to decide what to
// display before converting to gotestspec.BenchDelta, since the gate check
// (WorstRegression / worstRegressionKey) always needs the full, unfiltered
// deltas regardless of what's shown.
func filterDeltas(deltas []gotestbench.Delta, showAll bool) []gotestbench.Delta {
	if showAll {
		return deltas
	}
	var out []gotestbench.Delta
	for _, d := range deltas {
		if d.Significant {
			out = append(out, d)
		}
	}
	return out
}

// toSpecDeltas converts gotestbench.Delta rows to gotestspec.BenchDelta,
// the local mirror gotestspec renders via WithBenchDeltas (gotestspec must
// not import gotestbench, so this conversion lives at the call site).
func toSpecDeltas(deltas []gotestbench.Delta) []gotestspec.BenchDelta {
	out := make([]gotestspec.BenchDelta, len(deltas))
	for i, d := range deltas {
		out[i] = gotestspec.BenchDelta{
			Key:           d.Key,
			OldNs:         d.OldNs,
			NewNs:         d.NewNs,
			PercentChange: d.PercentChange,
			Significant:   d.Significant,
		}
	}
	return out
}

// emitBenchReport writes the versioned --json document to stdout. It returns
// a non-zero exit code only when the document itself cannot be produced.
func emitBenchReport(b gotestbench.Baseline, deltas []gotestbench.Delta, gate *gotestbench.Gate) int { //nolint:gocritic // hugeParam: stable API
	data, err := gotestbench.MarshalReport(gotestbench.NewReport(b, deltas, gate))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	fmt.Println(string(data))
	return 0
}

// deltasForReport keeps the report's deltas field absent (nil) when no
// comparison ran, and present-but-complete when one did: the document always
// carries every delta, significant or not — what to display is the
// consumer's call, significance is theirs to respect.
func deltasForReport(deltas []gotestbench.Delta, againstPath string) []gotestbench.Delta {
	if againstPath == "" {
		return nil
	}
	if deltas == nil {
		deltas = []gotestbench.Delta{}
	}
	return deltas
}
