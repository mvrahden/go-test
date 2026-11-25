package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestscenarios"
	"github.com/mvrahden/go-test/internal/gotestspec"
)

var scenariosAllowed = flagSet(
	"--specs", "--verify", "--unstated", "--input", "--format", "--output", "--fail-on",
	"--timeout", "--setup-timeout", "--parallel", "--compile-parallel", "--no-cache", "--control",
)

// scenariosArgs is one scenarios invocation, read and checked.
type scenariosArgs struct {
	specs          []string
	specsBase      string
	verify         bool
	input          string
	unstated       bool
	format, output string
	failOn         []string
	failOnGiven    bool
	exec           ExecConfig
	loadFlags      []string
}

// runScenarios binds the scenarios of the spec files to the suites' When and
// It labels, read from source, and judges them by a stream or a run when
// asked to.
func runScenarios(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	own, rest, err := SplitArgs(inv.DefaultArgsFor(scenariosAllowed), scenariosAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if refuseControl(own, own.Value("--input")) {
		return 2
	}
	root, err := scenariosModuleRoot(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	args, err := parseScenariosArgs(own, rest, &inv.Config, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	specs, err := gotestscenarios.Load(root, args.specsBase, args.specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: reading the spec files: %s\n", err)
		return 2
	}
	pkgs, err := loadRunPackages(args.exec.PackagePatterns, args.loadFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	tree, notes := buildStaticSpec(pkgs.Loaded, nil)
	opt := gotestscenarios.Options{
		ModuleRoot: root,
		Evidence:   gotestscenarios.EvidenceNone,
		Unstated:   args.unstated,
		Patterns:   args.exec.PackagePatterns,
		Incomplete: relativeNotes(root, notes),
	}
	for i := range pkgs.Broken {
		fmt.Fprintf(os.Stderr, "scenarios: %s cannot be read: %s\n", pkgs.Broken[i].PkgPath, firstLine(pkgs.Broken[i].Errors))
		opt.Unreadable = append(opt.Unreadable, pkgs.Broken[i].PkgPath)
	}
	doc := gotestscenarios.Bind(specs, tree, &opt)

	switch {
	case args.input != "":
		events, err := readEvents(args.input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		opt.Evidence = gotestscenarios.EvidenceInput
		if run, ok := gotestspec.RunOf(events); ok {
			opt.Run = &run
		} else {
			opt.Run = &gotestspec.RunInfo{}
		}
		opt.RunPackages = streamPackages(events)
		doc = gotestscenarios.Bind(specs, overlayEvents(pkgs.Loaded, tree, events), &opt)
	case args.verify:
		events, run, err := runBound(&args, pkgs, boundPackages(&doc))
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			return 2
		}
		opt.Evidence, opt.Run, opt.RunPackages = gotestscenarios.EvidenceRun, run, streamPackages(events)
		if run == nil {
			opt.Run = &gotestspec.RunInfo{}
		}
		doc = gotestscenarios.Bind(specs, overlayEvents(pkgs.Loaded, tree, events), &opt)
	}

	failOn := args.failOn
	if !args.failOnGiven && opt.Evidence != gotestscenarios.EvidenceNone {
		failOn = gotestscenarios.DefaultFailOn
	}
	verdict := gotestscenarios.Verdict{ExitCode: gotestscenarios.ExitCode(&doc, failOn), FailOn: failOn}
	err = writeAuditOutput(args.output, func(w io.Writer) error {
		switch args.format {
		case "json":
			return gotestscenarios.WriteJSON(w, &doc)
		case "md":
			gotestscenarios.RenderMarkdown(w, &doc, verdict)
		default:
			gotestscenarios.RenderText(w, &doc, verdict)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	return verdict.ExitCode
}

func parseScenariosArgs(own OwnFlags, rest []string, cfg *config.ProjectConfig, root string) (scenariosArgs, error) {
	a := scenariosArgs{
		verify:   own.Bool("--verify"),
		input:    own.Value("--input"),
		unstated: own.Bool("--unstated"),
		output:   own.Value("--output"),
	}
	switch f := own.ValueOr("--format", "text"); f {
	case "text", "json":
		a.format = f
	case "md", "markdown":
		a.format = "md"
	default:
		return a, fmt.Errorf("invalid --format %q: scenarios writes text, json or md", f)
	}
	switch {
	case a.verify && a.input != "":
		return a, errors.New("--verify runs the bound packages and --input reads a run's stream; give one")
	case own.Given("--input") && a.input == "":
		return a, errors.New("--input needs a file: --input=<events file>, or - for stdin")
	}
	if own.Given("--specs") {
		for _, p := range strings.Split(own.Value("--specs"), ",") {
			if p = strings.TrimSpace(p); p != "" {
				a.specs = append(a.specs, p)
			}
		}
		if len(a.specs) == 0 {
			return a, errors.New("--specs needs a spec file or directory")
		}
		a.specsBase = "."
	} else if len(cfg.Scenarios.Specs) > 0 {
		a.specs, a.specsBase = cfg.Scenarios.Specs, root
	}
	if own.Given("--fail-on") {
		classes, err := gotestscenarios.ParseClasses(own.Value("--fail-on"), a.verify || a.input != "")
		if err != nil {
			return a, fmt.Errorf("--fail-on: %w", err)
		}
		a.failOn, a.failOnGiven = classes, true
		a.unstated = a.unstated || slices.Contains(classes, gotestscenarios.ClassUnstated)
	}
	exec, err := parseExecFlags(own, rest, cfg)
	if err != nil {
		return a, err
	}
	classified := gotestrunner.ClassifyGoTestArgs(exec.GoTestArgs)
	if !a.verify {
		if len(classified.RunFlags) > 0 {
			return a, fmt.Errorf("%s is a flag of a run, and only --verify runs", classified.RunFlags[0])
		}
		for _, f := range []string{"--timeout", "--setup-timeout", "--parallel", "--compile-parallel", "--no-cache", "--control"} {
			if own.Given(f) && !configDefault(cfg, f) {
				return a, fmt.Errorf("%s is a flag of a run, and only --verify runs", f)
			}
		}
	}
	if len(gotestrunner.PatternIndices(rest, ownFlag)) == 0 {
		exec.PackagePatterns = []string{"./..."}
	}
	a.exec = exec
	a.loadFlags = gotestrunner.StripNonLoadFlags(classified.BuildFlags)
	return a, nil
}

// configDefault reports a flag DefaultArgsFor put in from .gotest.yml.
func configDefault(cfg *config.ProjectConfig, flag string) bool {
	return flag == "--timeout" && cfg.Timeout != nil || flag == "--setup-timeout" && cfg.SetupTimeout != nil
}

// scenariosModuleRoot is the directory of the go.mod at or above dir.
func scenariosModuleRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("scenarios reads a module: no go.mod at or above %s", abs)
		}
	}
}

// relativeNotes names the files of notes relative to the module root.
func relativeNotes(root string, notes []string) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = filepath.ToSlash(strings.ReplaceAll(n, root+string(filepath.Separator), ""))
	}
	return out
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return "it does not compile"
	}
	return strings.TrimSpace(lines[0])
}

// readEvents reads a saved go test -json stream, - for stdin.
func readEvents(input string) ([]gotestspec.TestEvent, error) {
	var r io.Reader = os.Stdin
	if input != "-" {
		f, err := os.Open(input) //nolint:gosec // G304: the stream the user named
		if err != nil {
			return nil, fmt.Errorf("opening input file: %w", err)
		}
		defer f.Close()
		r = f
	}
	events, err := gotestspec.ParseEvents(r)
	if err != nil {
		return nil, fmt.Errorf("parsing test events: %w", err)
	}
	if gap, _ := replayVerdict(events); strings.HasPrefix(gap, "input holds no test events") {
		return nil, errors.New(strings.TrimSpace(gap))
	}
	return events, nil
}

// overlayEvents lays a stream's verdicts over the declared tree, as spec
// --declared does.
func overlayEvents(loaded []*gotestgen.LoadResult, declared []*gotestspec.Package, events []gotestspec.TestEvent) []*gotestspec.Package {
	return gotestspec.Overlay(declared, gotestspec.BuildTree(events,
		gotestspec.WithDeclarations(buildDeclarationIndex(loaded)), gotestspec.WithSkipCalls(skipCallsOf(loaded))))
}

// streamPackages are the packages a stream reports, sorted, the run's own
// left out.
func streamPackages(events []gotestspec.TestEvent) []string {
	seen := map[string]bool{}
	var out []string
	for i := range events {
		if p := events[i].Package; p != "" && !events[i].RunLevel && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// boundPackages are the packages that hold an It a clause binds.
func boundPackages(doc *gotestscenarios.Document) map[string]bool {
	out := map[string]bool{}
	for i := range doc.Scenarios {
		for _, c := range doc.Scenarios[i].Clauses {
			for _, p := range c.Its {
				out[p.Package] = true
			}
		}
	}
	return out
}

// runBound runs the bound packages and no other, as spec runs them, and
// returns the run's stream and the run it names.
func runBound(a *scenariosArgs, pkgs gotestgen.RunPackages, bound map[string]bool) ([]gotestspec.TestEvent, *gotestspec.RunInfo, error) { //nolint:gocritic // hugeParam: once per run
	var loaded []*gotestgen.LoadResult
	for _, lr := range pkgs.Loaded {
		if bound[lr.PkgPath] {
			loaded = append(loaded, lr)
		}
	}
	if len(loaded) == 0 {
		return nil, nil, nil
	}
	var order []string
	for _, p := range pkgs.Order {
		if bound[p] {
			order = append(order, p)
		}
	}
	cfg := a.exec
	overlay, cleanup, err := gotestrunner.GenerateOverlay(loaded, nil, cfg.Debug, cfg.NoCache, cfg.HarvestSeeds)
	if err != nil {
		return nil, nil, err
	}
	defer cleanup()
	ctx, cancel := shutdownContext(cfg.Control)
	defer cancel()
	if cfg.GlobalTimeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, cfg.GlobalTimeout)
		defer timeoutCancel()
	}
	manifest := runManifest()
	result, err := gotestrunner.RunPipeline(ctx, gotestrunner.PipelineConfig{
		GoTestArgs:      cfg.GoTestArgs,
		SetupTimeout:    cfg.SetupTimeout,
		CI:              cfg.CI,
		Parallel:        cfg.Parallel,
		CompileParallel: cfg.CompileParallel,
		OutputMode:      gotestrunner.RunCaptureJSON,
		GlobalTimeout:   cfg.GlobalTimeout,
		PackageOrder:    order,
		Manifest:        manifest,
	}, overlay)
	if err != nil {
		return nil, nil, err
	}
	events, err := gotestspec.ParseEvents(bytes.NewReader(result.CapturedJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("parsing test events: %w", err)
	}
	code := result.ExitCode
	return events, &gotestspec.RunInfo{Manifest: *manifest, ExitCode: &code}, nil
}

func printScenariosHelp() {
	fmt.Print(`gotest scenarios — the scenarios of OpenSpec spec files, bound to the suites by text

Usage:
  gotest scenarios [--specs=<paths>] [flags] [packages]
  gotest scenarios --input=<events file> [flags] [packages]
  gotest scenarios --verify [run flags] [packages] [go test flags]

Reads every spec.md under specs/ and openspec/specs/, or the files and
directories --specs names (comma-separated, from the current directory) or
the scenarios.specs key of .gotest.yml (from the module root), and binds each
scenario to the suites of the packages by text, read from source: its GIVEN
and WHEN texts equal a When's label and the Whens directly around it, the
last innermost, and each THEN and AND text equals an It's label directly
under that When. Texts compare with whitespace collapsed, backticks, ** and
__ dropped, a trailing . , ; or : dropped and the first letter's case folded.

Without evidence a scenario is bound, unbound or ambiguous. With --input
(a saved go test -json stream) or --verify (which runs the packages that
hold a bound It), a bound scenario is verified, failing, not-run or vacuous.

Packages default to ./... .

Flags:
  --specs=<paths>      Spec files or directories to read instead of the defaults
  --input=<file>       Judge the scenarios by a saved go test -json stream (- reads stdin)
  --verify             Run the packages that hold a bound It and judge by that run
  --unstated           Also list the Its under bound Whens that no clause states
  --fail-on=<statuses> Exit 1 when a scenario has one of the comma-separated
                       statuses: ambiguous, failing, unbound, not-run, vacuous,
                       or unstated for an It no clause states; all is every
                       status the document can hold. Default with evidence:
                       ambiguous,failing; without it, none
  --format=<fmt>       text (default), json or md
  --output=<file>      Write to a file instead of stdout
  --timeout, --setup-timeout, --parallel, --compile-parallel, --no-cache,
  --control            As for a run, with --verify

Exit codes: 0 the policy holds; 1 a scenario has a status --fail-on names;
2 a flag is wrong, no spec file is found or one cannot be read, or a package
does not compile.

Examples:
  gotest scenarios
  gotest scenarios --verify --format=md --output=scenarios.md
  gotest scenarios --input=events.json --fail-on=all ./...
`)
}
