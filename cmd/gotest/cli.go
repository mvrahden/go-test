package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
)

func main() {
	args := os.Args[1:]

	if containsHelpFlag(args) {
		subcmd, _ := ParseSubcommand(args)
		showHelp(subcmd)
		return
	}

	subcmd, remaining := ParseSubcommand(args)

	// version and help need no project: a config that does not parse must
	// not stand between a newcomer and the page that explains it.
	switch subcmd {
	case "version":
		printVersion(remaining)
		return
	case "help":
		topic := ""
		if len(remaining) > 0 {
			topic = remaining[0]
		}
		showHelp(topic)
		return
	case "hook":
		// An agent's hook: it needs no config, and must answer even where
		// .gotest.yml does not parse.
		os.Exit(runHook(remaining, os.Stdin, os.Stdout, os.Stderr))
	}

	if subcmd == "" {
		if hint := commandHint(args); hint != "" {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", hint)
			os.Exit(2)
		}
	}

	projectCfg, err := config.Load(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: loading %s: %s\n", config.FileName, err)
		os.Exit(2)
	}

	inv := Invocation{Args: NormalizeTagsFlag(remaining), Config: projectCfg}

	switch subcmd {
	case "discover":
		os.Exit(runDiscover(inv))
	case "prepare":
		os.Exit(runPrepare(inv))
	case "scaffold":
		os.Exit(runScaffold(inv))
	case "migrate":
		os.Exit(runMigrate(inv))
	case "generate":
		os.Exit(runGenerate(inv))
	case "clean":
		os.Exit(runClean(inv))
	case "spec":
		os.Exit(runSpec(inv))
	case "summary":
		os.Exit(runSummary(inv))
	case "watch":
		os.Exit(runWatch(inv))
	case "bench":
		os.Exit(runBench(inv))
	case "fuzz":
		os.Exit(runFuzz(inv))
	case "refactor":
		os.Exit(runRefactor(inv))
	case "lint":
		os.Exit(runLint(inv))
	case "audit":
		os.Exit(runAudit(inv))
	case "scenarios":
		os.Exit(runScenarios(inv))
	case "skill":
		os.Exit(runSkill(inv))
	default:
		inv.Args = NormalizeTagsFlag(args)
		os.Exit(runTest(inv))
	}
}

// commandHint reads a first word that is neither a flag nor a package as a
// mistyped subcommand: a help topic such as "test" is the run itself, and a
// word within two edits of a subcommand names it. Package patterns carry a
// slash, a dot, a wildcard or name a directory; those are never hinted.
func commandHint(args []string) string {
	if len(args) == 0 {
		return ""
	}
	word := args[0]
	if word == "" || strings.HasPrefix(word, "-") || strings.ContainsAny(word, "/.") {
		return ""
	}
	if fi, err := os.Stat(word); err == nil && fi.IsDir() {
		return ""
	}
	rest := strings.Join(args[1:], " ")
	if word == "test" {
		return fmt.Sprintf("%q is not a subcommand: a run takes packages directly; did you mean: gotest %s", word, rest)
	}
	best, bestDist := "", 3
	for name := range knownSubcommands {
		if d := editDistance(word, name); d < bestDist {
			best, bestDist = name, d
		}
	}
	if best == "" || packageKnown(word) {
		return ""
	}
	return fmt.Sprintf("unknown command %q; did you mean %q? (gotest help lists the subcommands)", word, best)
}

// packageKnown reports whether the go command resolves word to a package,
// such as a standard-library one; asked only for a word near a subcommand.
var packageKnown = func(word string) bool {
	return exec.Command("go", "list", "--", word).Run() == nil //nolint:gosec // G204: a word the user typed, as a package
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func runTest(inv Invocation) int { //nolint:gocritic // hugeParam: stable API
	if specRequested(inv.Args) {
		var specArgs []string
		for _, a := range inv.Args {
			if name, _, _ := strings.Cut(a, "="); name != "--spec" {
				specArgs = append(specArgs, a)
			}
		}
		return runSpec(Invocation{Args: specArgs, Config: inv.Config})
	}

	own, goTestArgs, err := SplitArgs(inv.DefaultArgs(), testAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	jsonMode, goTestArgs := stripJSONFlag(goTestArgs)
	agent, why := agentOutput(own, os.Getenv)
	// A -json stream is already for machines, and -list's names are the answer.
	agent = agent && !jsonMode && !hasFlag(goTestArgs, "-list")
	if !agent {
		return runText(inv, own, goTestArgs, jsonMode, nil)
	}
	announceAgentOutput(os.Stderr, why)
	out := gotestrunner.NewAgentOutput(os.Stdout)
	code := runText(inv, own, goTestArgs, jsonMode, out)
	if !out.Closed() {
		// The run ended before its stream opened: the line still comes last.
		fmt.Fprintln(os.Stdout, gotestrunner.ClosingOf(nil, code, false))
	}
	return code
}

// runText runs the tests of a text or -json run, rendered for an agent
// when agent is not nil, and leaves the run's record.
func runText(inv Invocation, own OwnFlags, goTestArgs []string, jsonMode bool, agent *gotestrunner.AgentOutput) int { //nolint:gocritic // hugeParam: stable API
	if own.Given("--events-file") && !jsonMode {
		fmt.Fprintln(os.Stderr, "FAIL: --events-file saves the go test -json event stream, which a text run does not produce; add -json, or use gotest summary")
		return 2
	}
	events, err := openEventsFile(own)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if events != nil {
		defer events.Close()
	}

	minCoverage, err := parseMinFlag(own)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if minCoverage == 0 {
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
	cfg.JSON = jsonMode
	cfg.Events = eventsWriter(events)
	cfg.Agent = agent
	// A text run's manifest is only recorded, so it is read beside the run.
	manifest := make(chan *protocol.Manifest, 1)
	if jsonMode || agent != nil {
		cfg.Manifest = runManifest()
		manifest <- cfg.Manifest
	} else {
		go func() { manifest <- runManifest() }()
	}

	code, result, phases := execRun(cfg)
	code = enforceCoverage(coverProfile, minCoverage, code)
	failed := recordRun(result, code, <-manifest)
	if agent != nil && len(failed) > 0 {
		if dir, err := os.Getwd(); err == nil {
			agent.SetRerun(rerunCommand(dir, inv.Args))
		}
	}
	gotestrunner.EndRun(code)
	if !jsonMode && agent == nil && result != nil && result.Judged != nil {
		// stdout stays go test's text; the verdict closes stderr. A -list
		// run judges nothing.
		fmt.Fprintln(os.Stderr, gotestrunner.ClosingOf(result.Judged, code, false))
	}
	if phases != "" {
		fmt.Fprintln(os.Stderr, phases)
	}
	return code
}

func parseMinFlag(own OwnFlags) (int, error) {
	if !own.Given("--min") {
		return 0, nil
	}
	raw := own.Value("--min")
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --min value %q: must be an integer percentage", raw)
	}
	if v < 0 || v > 100 {
		return 0, fmt.Errorf("invalid --min value %d: must be 0-100", v)
	}
	return v, nil
}

func parseSetupTimeoutFlag(own OwnFlags) (time.Duration, error) {
	return parseDeadlineFlag(own, "--setup-timeout")
}

// parseDeadlineFlag reads a duration flag whose zero or negative value opts
// out of the deadline (-1); an absent flag is 0, the default.
func parseDeadlineFlag(own OwnFlags, name string) (time.Duration, error) {
	if !own.Given(name) {
		return 0, nil
	}
	raw := own.Value(name)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: %w", name, raw, err)
	}
	if d <= 0 {
		return -1, nil
	}
	return d, nil
}

func parseParallelFlag(own OwnFlags) (int, error) {
	return parsePositiveIntFlag(own, "--parallel")
}

// parsePositiveIntFlag reads an integer flag that must be positive; an
// absent flag is 0, the default, and an empty one is refused.
func parsePositiveIntFlag(own OwnFlags, name string) (int, error) {
	if !own.Given(name) {
		return 0, nil
	}
	raw := own.Value(name)
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value %q: must be a positive integer", name, raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid %s value %d: must be positive", name, v)
	}
	return v, nil
}

func parseCompileParallelFlag(own OwnFlags) (int, error) {
	return parsePositiveIntFlag(own, "--compile-parallel")
}

// parseControlFlag reads --control: standard input for stdin, its one
// value, and nil without the flag, so standard input is left alone.
func parseControlFlag(own OwnFlags) (io.Reader, error) {
	if !own.Given("--control") {
		return nil, nil
	}
	if v := own.Value("--control"); v != "stdin" {
		return nil, fmt.Errorf("invalid --control value %q: its one value is stdin", v)
	}
	return os.Stdin, nil
}

// refuseControl reports, on stderr, a --control value other than stdin, and
// --control=stdin beside --input=-, as both would read standard input.
func refuseControl(own OwnFlags, input string) bool {
	if _, err := parseControlFlag(own); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return true
	}
	if input != "-" || !own.Given("--control") {
		return false
	}
	fmt.Fprintln(os.Stderr, "FAIL: --control=stdin and --input=- both read standard input; save the stream to a file for --input")
	return true
}

func parseExecFlags(own OwnFlags, goTestArgs []string, projCfg *config.ProjectConfig) (ExecConfig, error) {
	control, err := parseControlFlag(own)
	if err != nil {
		return ExecConfig{}, err
	}
	setupTimeout, err := parseSetupTimeoutFlag(own)
	if err != nil {
		return ExecConfig{}, err
	}
	globalTimeout, err := parseGlobalTimeoutFlag(own)
	if err != nil {
		return ExecConfig{}, err
	}
	globalTimeout = resolveGlobalTimeout(globalTimeout)
	parallel, err := parseParallelFlag(own)
	if err != nil {
		return ExecConfig{}, err
	}
	if parallel == 0 {
		parallel = projCfg.Parallel
	}
	compileParallel, err := parseCompileParallelFlag(own)
	if err != nil {
		return ExecConfig{}, err
	}
	if compileParallel == 0 {
		compileParallel = projCfg.CompileParallel
	}
	if hasFlag(goTestArgs, "-o") {
		return ExecConfig{}, fmt.Errorf("-o is not supported: gotest builds each suite's binary itself; use `gotest generate` to see the sources, or -exec to run the binaries under a command")
	}
	if hasFlag(goTestArgs, "-overlay") {
		return ExecConfig{}, fmt.Errorf("-overlay is not supported: gotest compiles its generated harness through an overlay of its own, and the go command reads only one; put the replacement files in the tree, or run with --debug to inspect the overlay gotest writes")
	}
	if hasFlag(goTestArgs, "-c") {
		return ExecConfig{}, fmt.Errorf("-c is not supported: gotest builds and runs each suite's binary itself; gotest generate writes the harness for a go test -c of your own")
	}
	if hasFlag(goTestArgs, "-n") {
		return ExecConfig{}, fmt.Errorf("-n is not supported: gotest has no dry run; --debug keeps the generated sources and the work dir of a real one")
	}
	if hasFlag(goTestArgs, "-fuzz") {
		return ExecConfig{}, fmt.Errorf("-fuzz is not a run flag: gotest fuzz --target=<FuzzSuite_Method> fuzzes a target, and a run replays its seeds")
	}
	patterns, err := ExtractPackagePatterns(goTestArgs)
	if err != nil {
		return ExecConfig{}, err
	}
	var only map[string]string
	if own.Bool("--failed") {
		if only, patterns, err = rerunFailed(goTestArgs, patterns); err != nil {
			return ExecConfig{}, err
		}
	}
	return ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		GlobalTimeout:   globalTimeout,
		Debug:           own.Bool("--debug"),
		CI:              own.Bool("--ci") || detectCIEnv(),
		UpdateSnapshots: own.Bool("--update-snapshots"),
		NoCache:         own.Bool("--no-cache"),
		Parallel:        parallel,
		CompileParallel: compileParallel,
		HarvestSeeds:    !own.Bool("--no-harvest") && projCfg.Fuzz.HarvestSeeds(),
		Groups:          own.Values("--group"),
		SkipGroups:      own.Values("--skip-group"),
		Control:         control,
		Only:            only,
	}, nil
}

// rerunFailed is --failed's selection and the packages it runs: those given,
// else the packages of the last run's failures.
func rerunFailed(goTestArgs, patterns []string) (map[string]string, []string, error) {
	for _, flag := range []string{"-run", "-list"} {
		if hasFlag(goTestArgs, flag) {
			return nil, nil, fmt.Errorf("--failed selects the tests it runs itself; drop %s", flag)
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	only, failedPkgs, err := failedSelection(dir)
	if err != nil {
		return nil, nil, err
	}
	if len(gotestrunner.PatternIndices(goTestArgs, ownFlag)) == 0 {
		patterns = failedPkgs
	}
	return only, patterns, nil
}

// ensureCoverProfile makes sure a -coverprofile is in play when a coverage
// consumer (--min, --badge) needs one: the caller's own profile wins, else
// a temporary one is added and removed by the returned cleanup.
func ensureCoverProfile(goTestArgs []string, needed bool) ([]string, string, func(), error) {
	noop := func() {}
	if !needed {
		return goTestArgs, "", noop, nil
	}
	if v := gotestrunner.ExtractCoverProfile(goTestArgs); v != "" {
		return goTestArgs, v, noop, nil
	}
	dir, err := gotestrunner.NewStampedWorkDir()
	if err != nil {
		return goTestArgs, "", noop, err
	}
	profile := filepath.Join(dir, "cover.out")
	return gotestrunner.InsertBeforeArgs(goTestArgs, "-coverprofile="+profile), profile, func() { _ = os.RemoveAll(dir) }, nil
}

func enforceCoverage(coverProfile string, minCoverage, code int) int {
	code, failure := coverageVerdict(coverProfile, minCoverage, code)
	fmt.Fprint(os.Stderr, failure)
	return code
}

// coverageVerdict is the exit code a --min limit leaves code at, and what
// stderr says when the limit fails the run; "" when it does not.
func coverageVerdict(coverProfile string, minCoverage, code int) (int, string) {
	if code != 0 || minCoverage <= 0 || coverProfile == "" {
		return code, ""
	}
	pct, err := readCoverageTotal(coverProfile)
	if err != nil {
		return 2, fmt.Sprintf("FAIL: reading coverage: %s\n", err)
	}
	if pct < float64(minCoverage) {
		return 1, fmt.Sprintf("\nFAIL: %.1f%% coverage (minimum %d%%)\n", pct, minCoverage)
	}
	return code, ""
}

func readCoverageTotal(profilePath string) (float64, error) {
	out, err := exec.Command("go", "tool", "cover", "-func="+profilePath).Output() //nolint:gosec // G204: go tool with controlled arguments
	if err != nil {
		return 0, fmt.Errorf("go tool cover: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("empty coverage output")
	}
	last := lines[len(lines)-1]
	fields := strings.Fields(last)
	if len(fields) == 0 {
		return 0, fmt.Errorf("unexpected coverage format")
	}
	pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
	return strconv.ParseFloat(pctStr, 64)
}

const DefaultGlobalTimeout = 15 * time.Minute

func resolveGlobalTimeout(d time.Duration) time.Duration {
	switch {
	case d > 0:
		return d
	case d < 0:
		return 0
	default:
		return DefaultGlobalTimeout
	}
}

func parseGlobalTimeoutFlag(own OwnFlags) (time.Duration, error) {
	return parseDeadlineFlag(own, "--timeout")
}

// specRequested reports whether --spec is on among args in any spelling; a
// malformed value is left for SplitArgs to refuse.
func specRequested(args []string) bool {
	on := false
	for _, a := range args {
		if a == "--" || a == "-args" {
			break
		}
		name, value, hasEquals := strings.Cut(a, "=")
		if name != "--spec" {
			continue
		}
		if !hasEquals {
			on = true
			continue
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return false
		}
		on = b
	}
	return on
}

func stripJSONFlag(args []string) (bool, []string) {
	found := false
	var out []string
	for i, arg := range args {
		if arg == "-args" {
			// The test binary's words, as written.
			return found, append(out, args[i:]...)
		}
		if arg == "-json" {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return found, out
}
