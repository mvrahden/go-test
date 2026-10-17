package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

// runBench runs BenchmarkX wrapper functions for suites containing
// BenchmarkX methods, always dispatching them serially (never concurrently)
// so timing results stay meaningful. It mirrors runTest's compact flow
// (SplitArgs -> ClassifyGoTestArgs -> LoadPackages -> GenerateOverlay ->
// RunPipeline) with Bench: true.
func runBench(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, goTestArgs, err := SplitArgs(inv.DefaultArgs(), benchAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	specRequested := own.Bool("--spec")
	noColor := colorDisabled(own.Bool("--no-color"))
	jsonRequested := own.Bool("--json")

	// -v is forced further down for benchmark result visibility; capture
	// whether the caller actually asked for it first, since that's also our
	// signal for showing every delta row (vs. only significant ones) below.
	verboseRequested := gotestrunner.HasVerboseFlag(goTestArgs)

	saveTarget := own.Value("--save")
	if saveTarget == "" && own.Given("--save") {
		// `--save=` with no value asks for the configured baseline path —
		// the same fallback --against already has. Config resolution stays
		// in the CLI so tooling never parses .gotest.yml itself.
		saveTarget = inv.Config.Bench.Baseline
		if saveTarget == "" {
			fmt.Fprintln(os.Stderr, "FAIL: --save needs a path (or bench.baseline in .gotest.yml)")
			return 2
		}
	}

	againstGiven := own.Given("--against")
	againstPath := own.Value("--against")
	if againstPath == "" {
		againstPath = inv.Config.Bench.Baseline
	}

	gateGiven := own.Given("--gate")
	gatePct := inv.Config.Bench.Gate
	if gateGiven {
		raw := own.Value("--gate")
		v, perr := strconv.ParseFloat(raw, 64)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: invalid --gate value %q: %s\n", raw, perr)
			return 2
		}
		gatePct = v
	}
	// 0 disables, on the flag as in the config.
	gateActive := gatePct > 0
	if saveTarget != "" && againstPath != "" && sameFile(saveTarget, againstPath) {
		if againstGiven {
			// Saving first and comparing after would judge the run against
			// itself: every delta zero, every gate green, the old baseline gone.
			fmt.Fprintf(os.Stderr, "FAIL: --save and --against name the same file (%s); the run would be compared against itself\n", saveTarget)
			return 2
		}
		// Saving to the configured baseline promotes this run to be the
		// baseline; there is nothing older to compare it against, and a
		// configured gate has nothing to judge.
		againstPath = ""
		if !gateGiven {
			gateActive = false
		}
	}
	if gateActive && againstPath == "" {
		fmt.Fprintln(os.Stderr, "FAIL: --gate requires --against (or bench.baseline in .gotest.yml)")
		return 2
	}
	if count := gotestrunner.ExtractCountValue(goTestArgs); gateActive && count < gotestbench.MinSamplesForTTest {
		// Below four samples a side there is no significance test, only a
		// rule of thumb; a verdict built on it would be a guess with an exit code.
		fmt.Fprintf(os.Stderr, "FAIL: --gate needs -count=%d or more: a significance test needs %d samples a side, and this run would take %d\n", gotestbench.MinSamplesForTTest, gotestbench.MinSamplesForTTest, count)
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
		goTestArgs = gotestrunner.InsertBeforeArgs(goTestArgs, "-v")
	}

	cfg, err := parseExecFlags(own, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	classified := gotestrunner.ClassifyGoTestArgs(goTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	loaded, broken, noTestFiles := pkgs.Loaded, pkgs.Broken, pkgs.NoTestFiles
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// Timing numbers from a partially built tree are meaningless: fail fast
	// like generate/prepare rather than book-and-continue like run.
	if reportBrokenPackages(broken) {
		return 2
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	if len(overlay.BenchesByPkg) == 0 {
		if jsonRequested {
			// Machine consumers get a valid empty document, not prose.
			return emitBenchReport(gotestbench.FromPackages(nil), nil, nil, nil, nil, nil)
		}
		fmt.Println("no benchmarks found")
		return 0
	}

	ctx, stop := shutdownContext()
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

	pipelineStart := time.Now()
	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      mode,
		Bench:           true,
		BenchesByPkg:    overlay.BenchesByPkg,
		GlobalTimeout:   cfg.GlobalTimeout,
		NoTestFiles:     noTestFiles,
		PackageOrder:    pkgs.Order,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	code := result.ExitCode
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

	// The comparison reads the baseline as it was before this run; only then
	// may the run overwrite it.
	var oldBaseline gotestbench.Baseline
	if againstPath != "" {
		var err error
		oldBaseline, err = gotestbench.Load(againstPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	if saveTarget != "" {
		if err := gotestbench.Save(saveTarget, newBaseline); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	var deltas []gotestbench.Delta
	var specDeltas []gotestspec.BenchDelta
	var envMismatch []gotestbench.EnvDiff
	var missing, added []string

	if againstPath != "" {
		deltas = gotestbench.Compare(oldBaseline, newBaseline)
		specDeltas = toSpecDeltas(filterDeltas(deltas, verboseRequested))
		missing, added = gotestbench.Unmatched(oldBaseline, newBaseline)
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "WARN: %d benchmark(s) in the baseline did not run: %s\n", len(missing), strings.Join(missing, ", "))
		}
		if len(added) > 0 {
			fmt.Fprintf(os.Stderr, "WARN: %d benchmark(s) are new in this run and have no baseline: %s\n", len(added), strings.Join(added, ", "))
		}

		// A baseline from another toolchain or machine class measures the
		// machines, not the change. Say so and carry on: refusing would
		// strand anyone deliberately comparing across an upgrade.
		envMismatch = gotestbench.EnvMismatch(oldBaseline, newBaseline)
		if note := gotestbench.FormatEnvMismatch(envMismatch); note != "" {
			fmt.Fprintf(os.Stderr, "WARN: %s\n", note)
		}
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
		switch {
		case len(deltas) == 0:
			// A gate that judged nothing has not passed; a renamed module or
			// package would otherwise turn every later regression green.
			fmt.Fprintln(os.Stderr, "FAIL: bench gate: nothing to compare; no benchmark is in both the baseline and this run")
			if code == 0 {
				code = 1
			}
		case verdict.Breached:
			fmt.Fprintf(os.Stderr, "bench gate: %s +%.1f%% exceeds %g%% gate\n", verdict.WorstKey, verdict.WorstPct, gatePct)
			if code == 0 {
				code = 1
			}
		}
	}

	if jsonRequested {
		if jsonCode := emitBenchReport(newBaseline, deltasForReport(deltas, againstPath), missing, added, gateVerdict, envMismatch); jsonCode != 0 {
			return jsonCode
		}
	}

	// Mirror gotest summary --github's own $GITHUB_STEP_SUMMARY wiring:
	// under GitHub Actions, append the bench run's own markdown — results,
	// delta table when --against ran, gate verdict when one was set — so
	// it sits in the job summary beside what "summary" writes there.
	if tree != nil && os.Getenv("GITHUB_ACTIONS") == "true" {
		if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
			sf, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				// Wall clock, as summary does: a bench tree's own package
				// durations are near zero and would headline "<1ms".
				mdOpts := []gotestspec.RenderOption{gotestspec.WithElapsed(time.Since(pipelineStart))}
				if againstPath != "" {
					mdOpts = append(mdOpts, gotestspec.WithBenchDeltas(specDeltas))
				}
				if note := gotestbench.FormatEnvMismatch(envMismatch); note != "" {
					mdOpts = append(mdOpts, gotestspec.WithBenchEnvWarning(note))
				}
				if gateVerdict != nil {
					mdOpts = append(mdOpts, gotestspec.WithBenchGate(&gotestspec.BenchGate{
						ThresholdPct: gateVerdict.ThresholdPct,
						WorstPct:     gateVerdict.WorstPct,
						WorstKey:     gateVerdict.WorstKey,
						Breached:     gateVerdict.Breached,
					}))
				}
				gotestspec.RenderMarkdownBenchSummary(sf, tree, mdOpts...)
				sf.Close()
			}
		}
	}

	return code
}

// filterDeltas returns deltas as-is when showAll (set by -v) is true;
// otherwise it returns only the significant rows. Used to decide what to
// display before converting to gotestspec.BenchDelta, since the gate check
// (GateVerdict) always needs the full, unfiltered deltas regardless of
// what's shown.
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
			Key:                d.Key,
			OldNs:              d.OldNs,
			NewNs:              d.NewNs,
			PercentChange:      d.PercentChange,
			Significant:        d.Significant,
			InsufficientSample: d.InsufficientSample,
		}
	}
	return out
}

// emitBenchReport writes the versioned --json document to stdout. It returns
// a non-zero exit code only when the document itself cannot be produced.
func emitBenchReport(b gotestbench.Baseline, deltas []gotestbench.Delta, missing, added []string, gate *gotestbench.Gate, envMismatch []gotestbench.EnvDiff) int { //nolint:gocritic // hugeParam: stable API
	report := gotestbench.NewReport(b, deltas, gate, envMismatch)
	report.Missing, report.Added = missing, added
	data, err := gotestbench.MarshalReport(report)
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

// sameFile reports whether two paths name one file, by their clean absolute
// forms; a path that cannot be resolved is compared as written.
func sameFile(a, b string) bool {
	abs := func(p string) string {
		if r, err := filepath.Abs(p); err == nil {
			return filepath.Clean(r)
		}
		return filepath.Clean(p)
	}
	return abs(a) == abs(b)
}
