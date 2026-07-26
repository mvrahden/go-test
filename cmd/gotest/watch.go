package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func parseDebounceFlag(args []string) (time.Duration, error) {
	for i, arg := range args {
		var raw string
		if v, ok := strings.CutPrefix(arg, "--debounce="); ok {
			raw = v
		} else if arg == "--debounce" && i+1 < len(args) {
			raw = args[i+1]
		} else {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid --debounce value %q: %w", raw, err)
		}
		if d <= 0 {
			return 0, fmt.Errorf("invalid --debounce value %q: must be positive", raw)
		}
		return d, nil
	}
	return 200 * time.Millisecond, nil
}

func runWatch(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	args := inv.DefaultArgs()
	if inv.Config.Debounce != nil && !hasFlag(args, "--debounce") {
		args = append([]string{"--debounce=" + inv.Config.Debounce.Duration().String()}, args...)
	}
	ownArgs, goTestArgs, err := SplitArgs(args, watchAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	bench := hasFlag(ownArgs, "--bench")
	jsonMode, goTestArgs := stripJSONFlag(goTestArgs)
	specMode := hasFlag(ownArgs, "--spec")
	if specMode && jsonMode {
		fmt.Fprintln(os.Stderr, "FAIL: --spec cannot be combined with -json")
		return 2
	}
	debounceDuration, err := parseDebounceFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	cfg, err := parseExecFlags(ownArgs, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// Watch is the interactive focus-iteration tool: never let a CI-ish
	// environment auto-enable the F_ guard mid-iteration; --ci stays explicit.
	cfg.CI = hasFlag(ownArgs, "--ci")

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	if !jsonMode {
		fmt.Printf("\033[2m  running tests...\033[0m\n")
	}
	var benchNs map[string]float64
	_, benchNs = watchRunOnce(ctx, cfg, jsonMode, specMode, bench, benchNs)
	if !jsonMode {
		fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating watcher: %s\n", err)
		return 2
	}
	defer watcher.Close()

	for _, pattern := range cfg.PackagePatterns {
		addWatchDirs(watcher, pattern)
	}

	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}
	var changedDirs map[string]bool

	for {
		select {
		case <-ctx.Done():
			return 0

		case event, ok := <-watcher.Events:
			if !ok {
				return 0
			}
			if !isGoFile(event.Name) {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if changedDirs == nil {
				changedDirs = map[string]bool{}
			}
			changedDirs[filepath.Dir(event.Name)] = true
			debounce.Reset(debounceDuration)

		case <-debounce.C:
			if !jsonMode {
				clearTerminal()
			}
			pkgPatterns := dirsToPatterns(changedDirs)
			pkgArgs := replacePatterns(goTestArgs, pkgPatterns)
			changedCfg := cfg
			changedCfg.GoTestArgs = pkgArgs
			changedCfg.PackagePatterns = pkgPatterns
			_, benchNs = watchRunOnce(ctx, changedCfg, jsonMode, specMode, bench, benchNs)
			changedDirs = nil
			if !jsonMode {
				fmt.Printf("\n\033[2m  watching for changes...\033[0m\n")
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return 0
			}
			fmt.Fprintf(os.Stderr, "watch error: %s\n", err)
		}
	}
}

// watchRunOnce runs one watch iteration. benchNs carries the previous
// iteration's per-benchmark mean ns/op (nil on the first run) and the
// returned map becomes the caller's benchNs for the next iteration; it is
// only ever populated/consulted when bench is true.
func watchRunOnce(ctx context.Context, cfg ExecConfig, jsonMode, specMode, bench bool, benchNs map[string]float64) (int, map[string]float64) { //nolint:gocritic // hugeParam: stable API
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripCoverBuildFlags(classified.BuildFlags)
	loaded, broken, err := gotestgen.LoadPackages(cfg.PackagePatterns, loadFlags)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2, benchNs
	}

	if cfg.CI {
		if code, err := enforceFocusGuard(loaded); err != nil {
			if jsonMode {
				fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			}
			return 2, benchNs
		} else if code != 0 {
			return code, benchNs
		}
	}

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		if jsonMode {
			fmt.Printf("{\"Action\":\"watch-error\",\"Output\":%q}\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		}
		return 2, benchNs
	}
	defer cleanup()

	if jsonMode {
		fmt.Printf("{\"Action\":\"watch-start\",\"Package\":%q}\n", strings.Join(cfg.PackagePatterns, ","))
		cfg.JSON = true
	}

	// Bench mode needs -v injected the same way runBench forces it (see
	// cmd/gotest/bench.go): our batch collector otherwise suppresses a
	// passing suite's stdout, hiding the ns/op lines go test would
	// normally always print for -bench.
	if bench && !gotestrunner.HasVerboseFlag(cfg.GoTestArgs) {
		cfg.GoTestArgs = append(cfg.GoTestArgs, "-v")
	}

	mode := gotestrunner.RunBatchText
	if cfg.JSON {
		mode = gotestrunner.RunStreamJSON
	}
	if specMode || bench {
		// Both renders below need the result tree, which only RunCaptureJSON
		// produces (RunBatchText prints go test's own text output directly;
		// RunStreamJSON streams JSON live instead of returning it). This also
		// matches runBench's own capture path.
		mode = gotestrunner.RunCaptureJSON
	}

	runCtx := ctx
	if cfg.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer cancel()
	}

	result, err := gotestrunner.RunPipeline(runCtx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		Streaming:       false,
		OutputMode:      mode,
		Bench:           bench,
		BenchesByPkg:    overlay.BenchesByPkg,
		FuzzFuncsByPkg:  overlay.FuzzFuncsByPkg,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, benchNs
	}

	if bench {
		benchNs = renderBenchRun(result.CapturedJSON, jsonMode, benchNs)
	}

	if cfg.GlobalTimeout > 0 && runCtx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(os.Stderr, "FAIL: global --timeout exceeded after %v\n", cfg.GlobalTimeout)
		if result.ExitCode == 0 {
			return 1, benchNs
		}
	}
	if bench {
		// Bench rendering already draws the full tree, so it subsumes --spec.
		benchNs = renderBenchRun(result.CapturedJSON, jsonMode, benchNs)
	} else if specMode {
		events, perr := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
		if perr != nil {
			fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", perr)
			return 2, benchNs
		}
		gotestspec.RenderTerminal(os.Stdout, gotestspec.BuildTree(events))
	}
	return result.ExitCode, benchNs
}

// renderBenchRun parses a bench run's captured test2json output into a
// gotestspec tree, prints the results (in text mode) followed by any
// per-benchmark "Δ" delta lines vs prevNs, and returns the updated ns/op
// map for the next watch iteration. In JSON mode the raw captured events
// are written through as-is instead (mirroring what RunStreamJSON would
// have streamed live) and no delta lines are printed, since JSON consumers
// expect only test2json-shaped lines on stdout.
func renderBenchRun(capturedJSON []byte, jsonMode bool, prevNs map[string]float64) map[string]float64 {
	if jsonMode {
		os.Stdout.Write(capturedJSON) //nolint:errcheck // best-effort watch output
		return prevNs
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(capturedJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing bench events: %s\n", err)
		return prevNs
	}
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(os.Stdout, tree)

	baseline := gotestbench.FromPackages(tree)
	lines, nextNs := benchDeltaLines(baseline.Results, prevNs)
	for _, line := range lines {
		fmt.Println(line)
	}
	return nextNs
}

// benchDeltaKey uniquely identifies a benchmark result across watch runs,
// scoped by package and suite so same-named benchmarks in different
// packages/suites don't collide.
func benchDeltaKey(r gotestbench.Result) string {
	return r.Package + "\x00" + r.Suite + "\x00" + r.Name
}

// meanNsPerOp returns the mean ns/op across r's samples, or 0 if it has
// none.
func meanNsPerOp(r gotestbench.Result) float64 {
	if len(r.Samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range r.Samples {
		sum += s.NsPerOp
	}
	return sum / float64(len(r.Samples))
}

// benchDeltaLines computes the current run's mean ns/op for each of
// results and, for every benchmark that also appeared in prevNs (the
// previous run's ns/op, keyed by benchDeltaKey), formats a "Δ" delta line
// comparing the two. Benchmarks with no prior entry — the watcher's first
// bench run, or a benchmark that's new this run — produce no line, since
// there's nothing yet to compare against. The returned map becomes prevNs
// on the caller's next invocation, so deltas always compare against the
// immediately preceding in-memory run, never a persisted baseline.
func benchDeltaLines(results []gotestbench.Result, prevNs map[string]float64) (lines []string, nextNs map[string]float64) {
	nextNs = make(map[string]float64, len(results))
	for _, r := range results {
		mean := meanNsPerOp(r)
		key := benchDeltaKey(r)
		if old, ok := prevNs[key]; ok && old != 0 {
			pct := (mean - old) / old * 100
			lines = append(lines, fmt.Sprintf("%s  %.2f ns/op  (Δ %+.1f%%)", r.Name, mean, pct))
		}
		nextNs[key] = mean
	}
	return lines, nextNs
}

func addWatchDirs(w *fsnotify.Watcher, pattern string) {
	dir := strings.TrimSuffix(pattern, "/...")
	if dir == "" || dir == "." {
		dir = "."
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})
}

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func clearTerminal() {
	fmt.Print("\033[2J\033[H")
}

func dirsToPatterns(dirs map[string]bool) []string {
	patterns := make([]string, 0, len(dirs))
	for dir := range dirs {
		patterns = append(patterns, "./"+filepath.ToSlash(dir))
	}
	return patterns
}

func replacePatterns(originalArgs []string, newPatterns []string) []string {
	var args []string
	for _, arg := range originalArgs {
		if gotestrunner.LooksLikePackagePattern(arg) {
			continue
		}
		args = append(args, arg)
	}
	args = append(args, newPatterns...)
	return args
}
