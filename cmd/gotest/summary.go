package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

func runSummary(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, goTestArgs, err := SplitArgs(inv.DefaultArgs(), summaryAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	format := own.ValueOr("--format", "terminal")
	if format == formatTestResult {
		return runTestResult(own, goTestArgs)
	}
	if err := checkFormat(format); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if jsonMode, _ := stripJSONFlag(goTestArgs); jsonMode {
		fmt.Fprintln(os.Stderr, "FAIL: -json cannot be combined with summary; use --format=json for the summary as JSON")
		return 2
	}
	output := own.Value("--output")
	input := own.Value("--input")
	if refuseControl(own, input) {
		return 2
	}
	coverageProfile := own.Value("--coverage")
	badgePath := own.Value("--badge")
	noColor := reportColorDisabled(own.Bool("--no-color"), output)
	github := own.Bool("--github") || os.Getenv("GITHUB_ACTIONS") == "true"
	renderOnly := own.Bool("--render-only")

	if refuseWithoutInput(own, input) {
		return 2
	}

	if input != "" {
		if refuseInputFlags(own) {
			return 2
		}
		return runSummaryFromInput(input, format, output, coverageProfile, noColor, github, renderOnly, own.Bool("--check-fresh"), badgePath)
	}

	eventsFile, err := openEventsFile(own)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	defer closeEvents(eventsFile)

	minCoverage, err := parseMinFlag(own)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if minCoverage == 0 {
		minCoverage = inv.Config.MinCoverage
	}

	goTestArgs, coverProfile, coverCleanup, err := ensureCoverProfile(goTestArgs, minCoverage > 0 || badgePath != "")
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

	pipelineStart, manifest := time.Now(), cfg.Manifest
	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		UpdateSnapshots: cfg.UpdateSnapshots,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      gotestrunner.RunCaptureJSON,
		Events:          eventsWriter(eventsFile),
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
		gotestrunner.EndRun(2)
		return 2
	}
	noteStdlibTests(overlay.Plan.StdlibTests())
	gotestrunner.NoteBenchmarksNotRun(os.Stderr, goTestArgs, len(overlay.Declared.Benchmarks))

	tree := gotestspec.BuildTree(events, gotestspec.WithDeclarations(buildDeclarationIndex(loaded)))

	if coverageProfile == "" {
		coverageProfile = gotestrunner.ExtractCoverProfile(goTestArgs)
	}

	elapsed := time.Since(pipelineStart)
	var notes []string
	if note := stdlibNote(overlay.Plan.StdlibTests()); note != "" {
		notes = []string{note}
	}
	if err := writeSummaryOutput(tree, format, output, coverageProfile, noColor, github, elapsed, badgePath, groupRuns(result.Groups), notes, runInfo(events, &code)); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		gotestrunner.EndRun(2)
		return 2
	}

	fmt.Fprint(os.Stderr, coverageFailure)
	gotestrunner.EndRun(code)
	return code
}

// refuseInputFlags says on stderr why a flag of a run does not go with
// --input, and reports whether one was given.
func refuseInputFlags(own OwnFlags) bool { //nolint:gocritic // hugeParam: OwnFlags is passed by value everywhere
	if refuseGroups(own, "--input") {
		return true
	}
	if own.Given("--events-file") {
		fmt.Fprintln(os.Stderr, "FAIL: --events-file saves a run's event stream, and --input runs nothing")
		return true
	}
	return false
}

func runSummaryFromInput(input, format, output, coverageProfile string, noColor, github, renderOnly, checkFresh bool, badgePath string) int {
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

	tree := gotestspec.BuildTree(events, gotestspec.WithDeclarations(declarationsForStream(events, nil)))
	if checkFresh {
		if err := markFreshness(context.Background(), tree, events); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
	}

	if err := writeSummaryOutput(tree, format, output, coverageProfile, noColor, github, 0, badgePath, nil, nil, runInfo(events, nil)); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	return replayExitCode(events, tree, renderOnly)
}

// writeSummaryOutput renders the summary to stdout or --output; an --output
// it cannot create is the command's failure, exit 2 at the callers.
// groupRuns converts the pipeline's group timings for the renderers.
func groupRuns(groups []gotestrunner.GroupRun) []gotestspec.GroupRun {
	out := make([]gotestspec.GroupRun, 0, len(groups))
	for _, g := range groups {
		out = append(out, gotestspec.GroupRun{Name: g.Name, Suites: g.Suites, Wall: g.Wall})
	}
	return out
}

// stdlibNote is the stderr note of the stdlib tests not run, without its
// "note: ", for the Markdown summary; "" when there are none.
func stdlibNote(byPkg map[string]int) string {
	total, names := sumStdlibTests(byPkg)
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d stdlib test(s) in %s not run — gotest runs suites; use 'go test' for stdlib tests", total, names)
}

// notes go under the Markdown headline only: the terminal summary and the
// JSON document leave them on stderr.
func writeSummaryOutput(tree []*gotestspec.Package, format, output, coverageProfile string, noColor, github bool, elapsed time.Duration, badgePath string, groups []gotestspec.GroupRun, notes []string, run *gotestspec.RunInfo) error {
	var w io.Writer = os.Stdout
	var closeFunc func()
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
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
	if len(groups) > 0 {
		renderOpts = append(renderOpts, gotestspec.WithGroups(groups))
	}
	if run != nil {
		renderOpts = append(renderOpts, gotestspec.WithRun(run))
	}

	if coverageProfile != "" {
		report, err := gotestspec.ParseCoverageProfile(coverageProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: reading coverage profile: %s\n", err)
		} else {
			renderOpts = append(renderOpts, gotestspec.WithCoverage(report))
			if badgePath != "" {
				writeCoverageBadge(badgePath, report.Total)
			}
		}
	}

	mdOpts := renderOpts
	if len(notes) > 0 {
		mdOpts = append(append([]gotestspec.RenderOption(nil), renderOpts...), gotestspec.WithNotes(notes...))
	}

	switch format {
	case "md", "markdown":
		gotestspec.RenderMarkdownSummary(w, tree, mdOpts...)
	case "json":
		gotestspec.RenderJSON(w, tree, renderOpts...)
	default:
		gotestspec.RenderSummary(w, tree, renderOpts...)
	}

	// Annotations are workflow commands for a human-readable log; a JSON or
	// markdown document on stdout must stay the document alone.
	if github && format == "terminal" {
		modulePath := gotestspec.ReadModulePath(".")
		annotations := gotestspec.CollectAnnotations(tree, modulePath, ".")
		if a, ok := gotestspec.RunFailureAnnotation(tree, run); ok {
			annotations = append(annotations, a)
		}
		gotestspec.WriteGitHubAnnotations(os.Stdout, annotations)

		if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
			sf, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				gotestspec.RenderMarkdownSummary(sf, tree, mdOpts...)
				sf.Close()
			}
		}
	}
	return nil
}

// writeCoverageBadge renders the badge SVG to path. It never fails the run:
// the badge is a by-product of the summary, so problems are warnings.
func writeCoverageBadge(path string, pct float64) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: writing coverage badge: %s\n", err)
		return
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: writing coverage badge: %s\n", err)
		return
	}
	defer f.Close()
	if err := gotestspec.RenderCoverageBadge(f, pct); err != nil {
		fmt.Fprintf(os.Stderr, "warning: writing coverage badge: %s\n", err)
	}
}
