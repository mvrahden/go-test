package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSpec(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, goTestArgs, err := SplitArgs(inv.DefaultArgs(), specAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	format := own.ValueOr("--format", "terminal")
	if err := checkFormat(format); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if jsonMode, _ := stripJSONFlag(goTestArgs); jsonMode {
		fmt.Fprintln(os.Stderr, "FAIL: -json cannot be combined with spec; use --format=json for the specification as JSON")
		return 2
	}
	output := own.Value("--output")
	input := own.Value("--input")
	if refuseControl(own, input) {
		return 2
	}
	noColor := reportColorDisabled(own.Bool("--no-color"), output)
	renderOnly := own.Bool("--render-only")

	if own.Bool("--static") && input != "" {
		// One asks for the specification in the source, the other for the
		// specification in a recorded run. Silently honouring --input would
		// hand back a tree the caller did not ask for.
		fmt.Fprintf(os.Stderr, "FAIL: --static and --input are mutually exclusive\n")
		return 2
	}

	if refuseWithoutInput(own, input) {
		return 2
	}

	if own.Bool("--stamp") {
		if input == "" {
			fmt.Fprintf(os.Stderr, "FAIL: --stamp requires --input\n")
			return 2
		}
		return stampInput(input, output)
	}

	view, err := parseSpecView(own, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	if input != "" {
		if refuseGroups(own, "--input") {
			return 2
		}
		var source *declaredSource
		if view.declared {
			cfg, err := parseExecFlags(own, goTestArgs, &inv.Config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
				return 2
			}
			loadFlags := gotestrunner.StripNonLoadFlags(gotestrunner.ClassifyGoTestArgs(cfg.GoTestArgs).BuildFlags)
			source = &declaredSource{patterns: cfg.PackagePatterns, loadFlags: loadFlags}
		}
		return runSpecFromInput(input, format, output, noColor, renderOnly, own.Bool("--check-fresh"), source, view.options)
	}

	if own.Bool("--static") {
		if refuseGroups(own, "--static") {
			return 2
		}
		// The specification is written in the source; reading it should not
		// require executing it.
		return runStaticSpec(own, goTestArgs, &inv.Config, format, output, noColor)
	}

	minCoverage, err := parseMinFlag(own)
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

	cfg, err := parseExecFlags(own, goTestArgs, &inv.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	pkgs, ok := loadReportPackages(cfg, goTestArgs)
	if !ok {
		return 2
	}
	if code, err := focusGate(pkgs.Loaded, cfg.CI); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	} else if code != 0 {
		return code
	}
	plan, ok := planReport(&cfg, &pkgs)
	if !ok {
		return 2
	}
	loaded, broken, noTestFiles := pkgs.Loaded, pkgs.Broken, pkgs.NoTestFiles

	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, broken, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer cleanup()

	ctx, cancel := shutdownContext(cfg.Control)
	defer cancel()

	if cfg.GlobalTimeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer timeoutCancel()
	}

	manifest := cfg.Manifest
	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      gotestrunner.RunCaptureJSON,
		GlobalTimeout:   cfg.GlobalTimeout,
		NoTestFiles:     noTestFiles,
		PackageOrder:    pkgs.Order,
		Groups:          cfg.Groups,
		SkipGroups:      cfg.SkipGroups,
		Manifest:        manifest,
		Only:            cfg.Only,
		Inputs:          plan.inputs.stamp(),
	}, overlay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	code, coverageFailure := coverageVerdict(coverProfile, minCoverage, result.ExitCode)
	saveRun(&result, code, manifest, recordOptions{plan: plan, eventsFile: own.Value("--events-file"), narrowed: narrowedBy(&cfg, plan)})
	events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: parsing test events: %s\n", err)
		return 2
	}
	noteStdlibTests(overlay.Plan.StdlibTests())
	gotestrunner.NoteBenchmarksNotRun(os.Stderr, goTestArgs, len(overlay.Declared.Benchmarks))

	tree := gotestspec.BuildTree(events, gotestspec.WithDeclarations(buildDeclarationIndex(loaded)), gotestspec.WithSkipCalls(skipCallsOf(loaded)))
	if view.declared {
		// The packages the run loaded, read once; the run's verdicts lie over them.
		declared, _ := buildStaticSpec(loaded, nil)
		tree = gotestspec.Overlay(declared, tree)
	}

	if err := gotestspec.CheckView(tree, view.options...); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	w, closeOutput, ok := createOutput(output)
	if !ok {
		return 2
	}
	defer closeOutput()

	renderSpec(w, tree, format, noColor, append(slices.Clone(view.options), runOptions(events, &code)...), liveRun)
	fmt.Fprint(os.Stderr, coverageFailure)
	return code
}

// runInfo is the run events record, with code as its exit code when not
// nil; nil for a stream with no manifest.
func runInfo(events []gotestspec.TestEvent, code *int) *gotestspec.RunInfo {
	run, ok := gotestspec.RunOf(events)
	if !ok {
		return nil
	}
	if code != nil {
		c := *code
		run.ExitCode = &c
	}
	return &run
}

// runOptions name the run events record, with code as its exit code when
// not nil; none for a stream with no manifest.
func runOptions(events []gotestspec.TestEvent, code *int) []gotestspec.RenderOption {
	run := runInfo(events, code)
	if run == nil {
		return nil
	}
	return []gotestspec.RenderOption{gotestspec.WithRun(run)}
}

// renderSpec renders tree in format, under the view's options; live says
// whether the run was made here.
func renderSpec(w io.Writer, tree []*gotestspec.Package, format string, noColor bool, view []gotestspec.RenderOption, live bool) {
	renderOpts := slices.Clone(view)
	if noColor {
		renderOpts = append(renderOpts, gotestspec.WithNoColor())
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdown(w, tree, append(slices.Clone(view), reportOptions(live)...)...)
	case "json":
		gotestspec.RenderJSON(w, tree, view...)
	default:
		gotestspec.RenderTerminal(w, tree, append(renderOpts, terminalOptions(w, live)...)...)
	}
}

// declaredSource is the source a stream's verdicts are laid over (--declared).
type declaredSource struct {
	patterns  []string
	loadFlags []string
}

// overlay reads the specification the source declares, as --static does,
// and lays the stream's verdicts over it. A package that cannot be read is
// named on stderr and left to the stream.
func (s *declaredSource) overlay(events []gotestspec.TestEvent) ([]*gotestspec.Package, error) {
	pkgs, err := loadRunPackages(s.patterns, s.loadFlags)
	if err != nil {
		return nil, err
	}
	for i := range pkgs.Broken {
		fmt.Fprintf(os.Stderr, "declared: %s could not be read; its declarations are left out\n", pkgs.Broken[i].PkgPath)
	}
	declared, _ := buildStaticSpec(pkgs.Loaded, nil)
	// The labels of what the stream holds outside the patterns are read as
	// a plain --input reads them.
	idx := buildDeclarationIndex(pkgs.Loaded)
	loaded := map[string]bool{}
	for _, lr := range pkgs.Loaded {
		loaded[lr.PkgPath] = true
	}
	calls := skipCallsOf(pkgs.Loaded)
	outside, outsideCalls := streamSource(events, loaded)
	for pkg, paths := range outside {
		if idx == nil {
			idx = gotestspec.DeclarationIndex{}
		}
		idx[pkg] = paths
	}
	for pkg, lines := range outsideCalls {
		calls[pkg] = lines
	}
	return gotestspec.Overlay(declared, gotestspec.BuildTree(events, gotestspec.WithDeclarations(idx), gotestspec.WithSkipCalls(calls))), nil
}

// stampInput writes a saved stream back with the stamps gotest -json adds,
// placed by the stream's own order; a stamped event is kept as it is.
func stampInput(input, output string) int {
	var data []byte
	var err error
	if input == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(input) //nolint:gosec // G304: the file the caller names
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: reading input: %s\n", err)
		return 2
	}
	stamped := gotestrunner.StampStream(data)
	if output == "" {
		_, err = os.Stdout.Write(stamped)
	} else {
		err = os.WriteFile(output, stamped, 0o600)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: writing output: %s\n", err)
		return 2
	}
	return 0
}

func runSpecFromInput(input, format, output string, noColor, renderOnly, checkFresh bool, source *declaredSource, view []gotestspec.RenderOption) int {
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

	var tree []*gotestspec.Package
	if source == nil {
		idx, calls := streamSource(events, nil)
		tree = gotestspec.BuildTree(events, gotestspec.WithDeclarations(idx), gotestspec.WithSkipCalls(calls))
	} else if tree, err = source.overlay(events); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if checkFresh {
		if err := markFreshness(context.Background(), tree, events); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	if err := gotestspec.CheckView(tree, view...); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	w, closeOutput, ok := createOutput(output)
	if !ok {
		return 2
	}
	defer closeOutput()

	renderSpec(w, tree, format, noColor, append(slices.Clone(view), runOptions(events, nil)...), savedStream)

	// The same exit rule as `summary --input`: a renderer fed a failing stream
	// must not exit 0 — in a pipe without pipefail this exit code is the only
	// verdict CI ever sees. A client that renders rather than gates opts out
	// with --render-only; every operational failure above still exits 2.
	return replayExitCode(events, tree, renderOnly)
}

// replayExitCode is the verdict of a replayed stream: 2 when it holds no
// verdict to report (see replayVerdict); else the greatest of the code its
// closed runs ended with and what its verdicts say, 2 for a failed build,
// 1 on failures, else 0. renderOnly drops the verdict.
func replayExitCode(events []gotestspec.TestEvent, tree []*gotestspec.Package, renderOnly bool) int {
	if renderOnly {
		return 0
	}
	gap, buildFailed := replayVerdict(events)
	if gap != "" {
		fmt.Fprintf(os.Stderr, "FAIL: %s", gap)
		return 2
	}
	code := 0
	switch {
	case buildFailed:
		code = 2
	case gotestspec.HasFailures(tree):
		code = 1
	}
	if run := runInfo(events, nil); run != nil && run.ExitCode != nil {
		code = max(code, *run.ExitCode)
	}
	return code
}

// replayVerdict says why a replayed stream carries no verdict: it holds no
// test events at all (empty or not go test -json), or it ends before a
// package reported its own pass/fail/skip (cut short). gap is "" when the
// stream is whole. buildFailed reports a build-fail event or a verdict
// naming a FailedBuild.
func replayVerdict(events []gotestspec.TestEvent) (gap string, buildFailed bool) {
	seen := false
	var order []string
	finished := map[string]bool{}
	open := func(pkg string) {
		if _, ok := finished[pkg]; !ok {
			order = append(order, pkg)
		}
		finished[pkg] = false
	}
	var builds []string
	reported := map[string]bool{}
	for i := range events {
		ev := &events[i]
		switch ev.Action {
		case "":
			continue
		case gotestspec.ActionBuildFail:
			// Answered by the verdict of the package it failed.
			seen, buildFailed = true, true
			builds = append(builds, ev.ImportPath)
			continue
		case gotestspec.ActionBuildOutput:
			seen = true
			continue
		}
		seen = true
		if ev.Package == "" {
			continue
		}
		if ev.Test == "" && (ev.Action == gotestspec.ActionPass || ev.Action == gotestspec.ActionFail || ev.Action == gotestspec.ActionSkip) {
			if _, ok := finished[ev.Package]; !ok {
				order = append(order, ev.Package)
			}
			finished[ev.Package] = true
			if ev.FailedBuild != "" {
				reported[ev.FailedBuild], buildFailed = true, true
			}
			continue
		}
		// Anything after a verdict is a further run of the package
		// (concatenated streams): it reopens it.
		open(ev.Package)
	}
	if !seen {
		return "input holds no test events (expected a go test -json stream)\n", false
	}
	for _, build := range builds {
		if pkg := builtPackage(build); !reported[build] && !finished[pkg] {
			open(pkg)
		}
	}
	var b strings.Builder
	for _, pkg := range order {
		if !finished[pkg] {
			fmt.Fprintf(&b, "  %s\n", pkg)
		}
	}
	if b.Len() == 0 {
		return "", buildFailed
	}
	return "input ends before these packages reported a verdict (stream cut short?):\n" + b.String(), buildFailed
}

// builtPackage is the package a build's ImportPath belongs to:
// "p_test [p.test]" and "p [p.test]" are p's test builds; a plain path is
// itself.
func builtPackage(importPath string) string {
	if _, variant, ok := strings.Cut(importPath, " ["); ok {
		return strings.TrimSuffix(strings.TrimSuffix(variant, "]"), ".test")
	}
	return importPath
}

// createOutput is where a report goes: stdout, or the --output file it
// creates, closed by the func it returns. It says on stderr when it cannot.
func createOutput(output string) (io.Writer, func(), bool) {
	if output == "" {
		return os.Stdout, func() {}, true
	}
	f, err := os.Create(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: creating output file: %s\n", err)
		return nil, nil, false
	}
	return f, func() { _ = f.Close() }, true
}

// specView is how a spec is shown: the rows --hide leaves out, and whether
// the source's declarations lie under the stream's verdicts (--declared).
type specView struct {
	declared bool
	options  []gotestspec.RenderOption
}

// hideWords are the statuses --hide accepts, in the words the JSON uses.
var hideWords = map[string]gotestspec.Status{
	"pass": gotestspec.StatusPass,
	"fail": gotestspec.StatusFail,
	"skip": gotestspec.StatusSkip,
	"none": gotestspec.StatusNone,
}

// parseSpecView reads --declared, --hide, --match and --only. Either gives each JSON package
// its own stats. --hide= with nothing after it hides nothing.
func parseSpecView(own OwnFlags, input string) (specView, error) {
	var view specView
	static := own.Bool("--static")
	view.declared = own.Bool("--declared")
	if view.declared && static {
		return view, fmt.Errorf("--static and --declared are mutually exclusive")
	}
	if own.Given("--hide") {
		if static {
			return view, fmt.Errorf("--hide leaves out rows by their verdict, and --static has none")
		}
		var hidden []gotestspec.Status
		for _, word := range strings.Split(own.Value("--hide"), ",") {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			status, ok := hideWords[word]
			if !ok {
				return view, fmt.Errorf("unknown --hide status %q: use pass, fail, skip or none", word)
			}
			hidden = append(hidden, status)
		}
		view.options = append(view.options, gotestspec.WithHidden(hidden...))
	}
	if own.Given("--match") || own.Given("--only") {
		if static {
			return view, fmt.Errorf("--match and --only select rows of a run, and --static has none")
		}
	}
	if own.Given("--match") {
		view.options = append(view.options, gotestspec.WithMatch(own.Value("--match")))
	}
	if own.Given("--only") {
		pkg, position, err := parseOnly(own.Value("--only"))
		if err != nil {
			return view, err
		}
		view.options = append(view.options, gotestspec.WithOnly(pkg, position))
	}
	if view.declared || own.Given("--hide") {
		view.options = append(view.options, gotestspec.WithPackageStats())
	}
	return view, nil
}

// parseOnly reads --only=<import path> or --only=<import path>:<i>.<j>…
func parseOnly(value string) (pkg string, position []int, err error) {
	pkg, path, hasPath := strings.Cut(value, ":")
	if pkg == "" {
		return "", nil, fmt.Errorf("--only needs an import path: --only=<import path>[:<i>.<j>…]")
	}
	if !hasPath || path == "" {
		return pkg, nil, nil
	}
	for _, part := range strings.Split(path, ".") {
		at, convErr := strconv.Atoi(part)
		if convErr != nil || at < 0 {
			return "", nil, fmt.Errorf("--only: %q is not a position: use <import path>:<i>.<j>… with whole numbers", path)
		}
		position = append(position, at)
	}
	return pkg, position, nil
}
