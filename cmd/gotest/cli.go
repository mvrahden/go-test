package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/config"
)

func main() {
	args := os.Args[1:]

	if containsHelpFlag(args) {
		subcmd, _ := ParseSubcommand(args)
		showHelp(subcmd)
		return
	}

	projectCfg, err := config.Load(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: loading %s: %s\n", config.FileName, err)
		os.Exit(2)
	}

	subcmd, remaining := ParseSubcommand(args)
	inv := Invocation{Args: remaining, Config: projectCfg}

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
	case "refactor":
		os.Exit(runRefactor(inv))
	case "lint":
		os.Exit(runLint(inv))
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
	default:
		inv.Args = args
		os.Exit(runTest(inv))
	}
}

func runTest(inv Invocation) int {
	if slices.Contains(inv.Args, "--spec") {
		var specArgs []string
		for _, a := range inv.Args {
			if a != "--spec" {
				specArgs = append(specArgs, a)
			}
		}
		return runSpec(Invocation{Args: specArgs, Config: inv.Config})
	}

	args := inv.DefaultArgs()
	ownArgs, goTestArgs, err := SplitArgs(args, testAllowed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}

	jsonMode, goTestArgs := stripJSONFlag(goTestArgs)

	minCoverage, err := parseMinFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if minCoverage == 0 {
		minCoverage = inv.Config.MinCoverage
	}
	setupTimeout, err := parseSetupTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	globalTimeout, err := parseGlobalTimeoutFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	globalTimeout = resolveGlobalTimeout(globalTimeout)
	parallel, err := parseParallelFlag(ownArgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		return 2
	}
	if parallel == 0 {
		parallel = inv.Config.Parallel
	}

	var coverProfile string
	if minCoverage > 0 {
		for _, arg := range goTestArgs {
			if v, ok := strings.CutPrefix(arg, "-coverprofile="); ok {
				coverProfile = v
			}
		}
		if coverProfile == "" {
			f, err := os.CreateTemp("", "gotest-cover-*.out")
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
				return 2
			}
			coverProfile = f.Name()
			f.Close()
			defer os.Remove(coverProfile)
			goTestArgs = append(goTestArgs, "-coverprofile="+coverProfile)
		}
	}

	patterns := ExtractPackagePatterns(goTestArgs)
	cfg := ExecConfig{
		GoTestArgs:      goTestArgs,
		PackagePatterns: patterns,
		SetupTimeout:    setupTimeout,
		GlobalTimeout:   globalTimeout,
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		JSON:            jsonMode,
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
		NoCache:         slices.Contains(ownArgs, "--no-cache"),
		Parallel:        parallel,
	}

	code := Run(cfg)

	if code == 0 && minCoverage > 0 {
		pct, err := readCoverageTotal(coverProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: reading coverage: %s\n", err)
			return 2
		}
		if pct < float64(minCoverage) {
			fmt.Fprintf(os.Stderr, "\nFAIL: %.1f%% coverage (minimum %d%%)\n", pct, minCoverage)
			return 1
		}
	}

	return code
}

func parseMinFlag(args []string) (int, error) {
	for i, arg := range args {
		var raw string
		if v, ok := strings.CutPrefix(arg, "--min="); ok {
			raw = v
		} else if arg == "--min" && i+1 < len(args) {
			raw = args[i+1]
		} else {
			continue
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid --min value %q: must be an integer percentage", raw)
		}
		if v < 0 || v > 100 {
			return 0, fmt.Errorf("invalid --min value %d: must be 0-100", v)
		}
		return v, nil
	}
	return 0, nil
}

func parseSetupTimeoutFlag(args []string) (time.Duration, error) {
	for i, arg := range args {
		var raw string
		if v, ok := strings.CutPrefix(arg, "--setup-timeout="); ok {
			raw = v
		} else if arg == "--setup-timeout" && i+1 < len(args) {
			raw = args[i+1]
		} else {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid --setup-timeout value %q: %w", raw, err)
		}
		if d <= 0 {
			return -1, nil
		}
		return d, nil
	}
	return 0, nil
}

func parseParallelFlag(args []string) (int, error) {
	raw := extractStringFlag(args, "--parallel", "")
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --parallel value %q: must be a positive integer", raw)
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid --parallel value %d: must be positive", v)
	}
	return v, nil
}

func readCoverageTotal(profilePath string) (float64, error) {
	out, err := exec.Command("go", "tool", "cover", "-func="+profilePath).Output()
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

func parseGlobalTimeoutFlag(args []string) (time.Duration, error) {
	for i, arg := range args {
		var raw string
		if v, ok := strings.CutPrefix(arg, "--timeout="); ok {
			raw = v
		} else if arg == "--timeout" && i+1 < len(args) {
			raw = args[i+1]
		} else {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid --timeout value %q: %w", raw, err)
		}
		if d <= 0 {
			return -1, nil
		}
		return d, nil
	}
	return 0, nil
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

