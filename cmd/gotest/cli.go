package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestrunner"
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
		fmt.Println(about.LongInfo())
		return
	case "help":
		topic := ""
		if len(remaining) > 0 {
			topic = remaining[0]
		}
		showHelp(topic)
		return
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
	if best == "" {
		return ""
	}
	return fmt.Sprintf("unknown command %q; did you mean %q? (gotest help lists the subcommands)", word, best)
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

	return enforceCoverage(coverProfile, minCoverage, Run(cfg))
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
// absent or empty flag is 0, the default.
func parsePositiveIntFlag(own OwnFlags, name string) (int, error) {
	raw := own.Value(name)
	if raw == "" {
		return 0, nil
	}
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

func parseExecFlags(own OwnFlags, goTestArgs []string, projCfg *config.ProjectConfig) (ExecConfig, error) {
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
	patterns, err := ExtractPackagePatterns(goTestArgs)
	if err != nil {
		return ExecConfig{}, err
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
	}, nil
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
	f, err := os.CreateTemp("", "gotest-cover-*.out")
	if err != nil {
		return goTestArgs, "", noop, err
	}
	profile := f.Name()
	f.Close()
	return gotestrunner.InsertBeforeArgs(goTestArgs, "-coverprofile="+profile), profile, func() { os.Remove(profile) }, nil
}

func enforceCoverage(coverProfile string, minCoverage, code int) int {
	if code != 0 || minCoverage <= 0 || coverProfile == "" {
		return code
	}
	pct, err := readCoverageTotal(coverProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: reading coverage: %s\n", err)
		return 2
	}
	if pct < float64(minCoverage) {
		fmt.Fprintf(os.Stderr, "\nFAIL: %.1f%% coverage (minimum %d%%)\n", pct, minCoverage)
		return 1
	}
	return code
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
		if a == "--" {
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
	for _, arg := range args {
		if arg == "-json" {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return found, out
}
