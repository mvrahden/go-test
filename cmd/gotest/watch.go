package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/pkgpattern"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mvrahden/go-test/internal/gotestbench"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func parseDebounceFlag(own OwnFlags) (time.Duration, error) {
	if !own.Given("--debounce") {
		return 200 * time.Millisecond, nil
	}
	raw := own.Value("--debounce")
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --debounce value %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid --debounce value %q: must be positive", raw)
	}
	return d, nil
}

func runWatch(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	args := inv.DefaultArgs()
	if inv.Config.Debounce != nil && !hasFlag(args, "--debounce") {
		args = append([]string{"--debounce=" + inv.Config.Debounce.Duration().String()}, args...)
	}
	own, goTestArgs, err := SplitArgs(args, watchAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	bench := own.Bool("--bench")
	jsonMode, goTestArgs := stripJSONFlag(goTestArgs)
	specMode := own.Bool("--spec")
	if specMode && jsonMode {
		fmt.Fprintln(os.Stderr, "FAIL: --spec cannot be combined with -json")
		return 2
	}
	debounceDuration, err := parseDebounceFlag(own)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	cfg, err := parseExecFlags(own, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	// Watch is the interactive focus-iteration tool: never let a CI-ish
	// environment auto-enable the F_ guard mid-iteration; --ci stays explicit.
	cfg.CI = own.Bool("--ci")

	ctx, stop := shutdownContext()
	defer stop()

	if !jsonMode {
		fmt.Println(dim("  running tests..."))
	}
	var benchNs map[string]float64
	var firstCode int
	firstCode, benchNs = watchRunOnce(ctx, cfg, jsonMode, specMode, bench, benchNs)
	emitWatchEnd(jsonMode, cfg.PackagePatterns, firstCode)
	if !jsonMode {
		fmt.Println("\n" + dim("  watching for changes..."))
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating watcher: %s\n", err)
		return 2
	}
	defer watcher.Close()
	src := fsnotifySource{watcher}

	loadFlags := gotestrunner.StripNonLoadFlags(gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs).BuildFlags)
	watchDirsFor(src, cfg.PackagePatterns, loadFlags)
	reverse := reverseDepDirs(cfg.PackagePatterns, loadFlags)

	rerun := func(dirs []string) {
		if !jsonMode {
			clearTerminal()
		}
		pkgPatterns := dirsToPatterns(dirs)
		changedCfg := cfg
		changedCfg.GoTestArgs = replacePatterns(goTestArgs, pkgPatterns)
		changedCfg.PackagePatterns = pkgPatterns
		var code int
		code, benchNs = watchRunOnce(ctx, changedCfg, jsonMode, specMode, bench, benchNs)
		emitWatchEnd(jsonMode, pkgPatterns, code)
		// The dependency graph may have changed with the code.
		reverse = reverseDepDirs(cfg.PackagePatterns, loadFlags)
		if !jsonMode {
			fmt.Println("\n" + dim("  watching for changes..."))
		}
	}
	return watchLoop(ctx, src, debounceDuration, func() map[string][]string { return reverse }, rerun)
}

// watchSource is the file system as the loop sees it: fsnotify in the
// command, a fake in tests.
type watchSource interface {
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Add(path string) error
}

type fsnotifySource struct{ w *fsnotify.Watcher }

func (f fsnotifySource) Events() <-chan fsnotify.Event { return f.w.Events }
func (f fsnotifySource) Errors() <-chan error          { return f.w.Errors }
func (f fsnotifySource) Add(path string) error         { return f.w.Add(path) }

// watchLoop reruns what changes until ctx ends. A Go file written, created,
// renamed or removed marks its directory; after the debounce the marked
// directories and, through deps, every package depending on them are handed
// to run. A directory created under a watched one is watched too. The
// interrupt that ends the loop is its exit code, 130.
func watchLoop(ctx context.Context, src watchSource, debounce time.Duration, deps func() map[string][]string, run func(dirs []string)) int {
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}
	var changed map[string]bool
	for {
		select {
		case <-ctx.Done():
			return 130

		case event, ok := <-src.Events():
			if !ok {
				return 130
			}
			if event.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					watchTree(src, event.Name)
					continue
				}
			}
			if !isGoFile(event.Name) || event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}
			if changed == nil {
				changed = map[string]bool{}
			}
			changed[filepath.Dir(event.Name)] = true
			timer.Reset(debounce)

		case <-timer.C:
			dirs := rerunDirs(changed, deps())
			changed = nil
			run(dirs)

		case err, ok := <-src.Errors():
			if !ok {
				return 130
			}
			fmt.Fprintf(os.Stderr, "watch error: %s\n", err)
		}
	}
}

// emitWatchEnd closes a cycle in the JSON stream, so a consumer knows the
// verdict is complete without waiting for the next cycle to start.
func emitWatchEnd(jsonMode bool, patterns []string, code int) {
	if jsonMode {
		fmt.Printf("{\"Action\":\"watch-end\",\"Package\":%q,\"Exit\":%d}\n", strings.Join(patterns, ","), code)
	}
}

// rerunDirs is the sorted set of directories to rerun: the changed ones and
// every package that depends on them.
func rerunDirs(changed map[string]bool, reverse map[string][]string) []string {
	set := map[string]bool{}
	for dir := range changed {
		set[dir] = true
		for _, dependent := range reverse[dir] {
			set[dependent] = true
		}
	}
	dirs := make([]string, 0, len(set))
	for dir := range set {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

// watchDirsFor watches the directories the patterns name: the packages they
// resolve to, every directory under a /... pattern so a package written
// there later is seen too, and, for a pattern matching nothing yet, the
// nearest existing directory on its path.
func watchDirsFor(src watchSource, patterns, loadFlags []string) {
	pkgs, unmatched, err := gotestgen.ListPackageDirs("", patterns, loadFlags)
	if err == nil {
		for _, p := range pkgs {
			_ = src.Add(p.Dir)
		}
	}
	for _, pattern := range patterns {
		if dir, ok := strings.CutSuffix(pattern, "/..."); ok {
			watchTree(src, dirOrDot(dir))
		}
	}
	for _, pattern := range unmatched {
		if dir := nearestDir(dirOrDot(strings.TrimSuffix(pattern, "/..."))); dir != "" {
			watchTree(src, dir)
		}
	}
}

func dirOrDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// nearestDir is dir or its closest existing ancestor; "" for an import path.
func nearestDir(dir string) string {
	if !strings.ContainsAny(dir, `/\`) && dir != "." {
		return ""
	}
	for {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// watchTree watches dir and every directory below it, skipping the ones no
// package lives in.
func watchTree(src watchSource, dir string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != dir && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			_ = src.Add(path)
		}
		return nil
	})
}

// reverseDepDirs maps a directory to the directories of the test packages
// among the patterns that depend on it, within the main module(s), so a
// change to a dependency reruns what it can break. Best effort: on any go
// list error the map is empty and only the changed directory reruns.
func reverseDepDirs(patterns, loadFlags []string) map[string][]string {
	mods, err := goListLines("list", "-m", "-f", "{{.Path}}")
	if err != nil || len(mods) == 0 {
		return nil
	}
	inModule := func(path string) bool {
		for _, m := range mods {
			if path == m || strings.HasPrefix(path, m+"/") {
				return true
			}
		}
		return false
	}
	args := append([]string{"list", "-e", "-test", "-f", "{{.Dir}}|{{join .Deps \" \"}}"}, loadFlags...)
	lines, err := goListLines(append(args, pkgpattern.Clean(patterns)...)...)
	if err != nil {
		return nil
	}
	depsOf := map[string]map[string]bool{}
	allDeps := map[string]bool{}
	for _, line := range lines {
		dir, deps, ok := strings.Cut(line, "|")
		if !ok || dir == "" {
			continue
		}
		for _, dep := range strings.Fields(deps) {
			if inModule(dep) {
				if depsOf[dir] == nil {
					depsOf[dir] = map[string]bool{}
				}
				depsOf[dir][dep] = true
				allDeps[dep] = true
			}
		}
	}
	if len(allDeps) == 0 {
		return nil
	}
	depList := make([]string, 0, len(allDeps))
	for dep := range allDeps {
		depList = append(depList, dep)
	}
	sort.Strings(depList)
	dirLines, err := goListLines(append([]string{"list", "-e", "-f", "{{.ImportPath}}|{{.Dir}}"}, depList...)...)
	if err != nil {
		return nil
	}
	dirOf := map[string]string{}
	for _, line := range dirLines {
		if path, dir, ok := strings.Cut(line, "|"); ok && dir != "" {
			dirOf[path] = dir
		}
	}
	reverse := map[string][]string{}
	for pkgDir, deps := range depsOf {
		for dep := range deps {
			depDir := dirOf[dep]
			if depDir == "" || depDir == pkgDir || slices.Contains(reverse[depDir], pkgDir) {
				continue
			}
			reverse[depDir] = append(reverse[depDir], pkgDir)
		}
	}
	for dir := range reverse {
		sort.Strings(reverse[dir])
	}
	return reverse
}

// goListLines runs the go command and returns its non-empty stdout lines.
func goListLines(args ...string) ([]string, error) {
	out, err := exec.Command("go", args...).Output() //nolint:gosec // G204: the go tool with the command's own patterns
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// watchRunOnce runs one watch iteration. benchNs carries the previous
// iteration's per-benchmark mean ns/op (nil on the first run) and the
// returned map becomes the caller's benchNs for the next iteration; it is
// only ever populated/consulted when bench is true.
func watchRunOnce(ctx context.Context, cfg ExecConfig, jsonMode, specMode, bench bool, benchNs map[string]float64) (int, map[string]float64) { //nolint:gocritic // hugeParam: stable API
	classified := gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs)
	loadFlags := gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	pkgs, err := loadRunPackages(cfg.PackagePatterns, loadFlags)
	loaded, broken, noTestFiles := pkgs.Loaded, pkgs.Broken, pkgs.NoTestFiles
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

	// Only the spec view renders a tree, and reading the declarations costs a
	// second walk of every test file — not something to repeat on each save
	// when nothing will consume it.
	var decls gotestspec.DeclarationIndex
	if specMode {
		decls = buildDeclarationIndex(loaded)
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
		cfg.GoTestArgs = gotestrunner.InsertBeforeArgs(cfg.GoTestArgs, "-v")
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
		OutputMode:      mode,
		Bench:           bench,
		BenchesByPkg:    overlay.BenchesByPkg,
		FuzzFuncsByPkg:  overlay.FuzzFuncsByPkg,
		GlobalTimeout:   cfg.GlobalTimeout,
		NoTestFiles:     noTestFiles,
		PackageOrder:    pkgs.Order,
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2, benchNs
	}

	benchNs, err = renderWatchRun(os.Stdout, result.CapturedJSON, jsonMode, specMode, bench, decls, benchNs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2, benchNs
	}
	return result.ExitCode, benchNs
}

// renderWatchRun draws one iteration's result exactly once: the bench tree
// with its deltas when bench is set (it subsumes --spec), else the spec tree.
func renderWatchRun(w io.Writer, capturedJSON []byte, jsonMode, specMode, bench bool, decls gotestspec.DeclarationIndex, benchNs map[string]float64) (map[string]float64, error) {
	if bench {
		return renderBenchRun(w, capturedJSON, jsonMode, benchNs), nil
	}
	if specMode {
		events, err := gotestspec.ParseEvents(bytes.NewReader(capturedJSON))
		if err != nil {
			return benchNs, err
		}
		gotestspec.RenderTerminal(w, gotestspec.BuildTree(events, gotestspec.WithDeclarations(decls)))
	}
	return benchNs, nil
}

// renderBenchRun parses a bench run's captured test2json output into a
// gotestspec tree, prints the results (in text mode) followed by any
// per-benchmark "Δ" delta lines vs prevNs, and returns the updated ns/op
// map for the next watch iteration. In JSON mode the raw captured events
// are written through as-is instead (mirroring what RunStreamJSON would
// have streamed live) and no delta lines are printed, since JSON consumers
// expect only test2json-shaped lines on stdout.
func renderBenchRun(w io.Writer, capturedJSON []byte, jsonMode bool, prevNs map[string]float64) map[string]float64 {
	if jsonMode {
		w.Write(capturedJSON) //nolint:errcheck // best-effort watch output
		return prevNs
	}

	events, err := gotestspec.ParseEvents(bytes.NewReader(capturedJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing bench events: %s\n", err)
		return prevNs
	}
	tree := gotestspec.BuildTree(events)
	gotestspec.RenderTerminal(w, tree)

	baseline := gotestbench.FromPackages(tree)
	lines, nextNs := benchDeltaLines(baseline.Results, prevNs)
	for _, line := range lines {
		fmt.Fprintln(w, line)
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

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func clearTerminal() {
	if !colorDisabled(false) {
		fmt.Print("\033[2J\033[H")
	}
}

func dirsToPatterns(dirs []string) []string {
	patterns := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		patterns = append(patterns, dirPattern(dir))
	}
	return patterns
}

// dirPattern is the package pattern for a directory: an absolute or an
// explicitly relative path is one already; a bare relative one needs ./ so
// go does not read it as an import path. A slash-rooted path counts as
// absolute on every platform: Windows would call it relative for lacking a
// volume, and "./" in front of it names nothing.
func dirPattern(dir string) string {
	dir = filepath.ToSlash(dir)
	if dir == "." || filepath.IsAbs(dir) || strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, "./") || strings.HasPrefix(dir, "../") {
		return dir
	}
	return "./" + dir
}

func replacePatterns(originalArgs []string, newPatterns []string) []string {
	drop := map[int]bool{}
	for _, i := range gotestrunner.PatternIndices(originalArgs, ownFlag) {
		drop[i] = true
	}
	var args []string
	for i, arg := range originalArgs {
		if !drop[i] {
			args = append(args, arg)
		}
	}
	args = append(args, newPatterns...)
	return args
}

// dim renders s faintly on a terminal and plainly everywhere else.
func dim(s string) string {
	if colorDisabled(false) {
		return s
	}
	return "\033[2m" + s + "\033[0m"
}
