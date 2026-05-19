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
	projectCfg, err := config.Load(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: loading %s: %s\n", config.FileName, err)
		os.Exit(2)
	}

	subcmd, remaining := ParseSubcommand(os.Args[1:])
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
		printUsage()
		return
	default:
		inv.Args = os.Args[1:]
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
		Debug:           slices.Contains(ownArgs, "--debug"),
		CI:              slices.Contains(ownArgs, "--ci"),
		JSON:            jsonMode,
		UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
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
			return 0, fmt.Errorf("invalid --setup-timeout value %q: must be positive", raw)
		}
		return d, nil
	}
	return time.Minute, nil
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

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s — test suite runner for Go

Usage:
  gotest [gotest-flags] [--] [go-test-flags] [packages...]
  gotest <subcommand> [flags] [packages...]

Subcommands:
  discover    Discover test suites and output JSON metadata
  prepare     Generate overlay and start shared fixtures for debug (blocks until SIGTERM)
  generate    Run code generation only (no test execution)
  clean       Remove orphaned generated files
  spec        Render behavioral specification from test suites
  watch       Watch for file changes and re-run tests
  scaffold    Generate test suite skeleton from a type or file
  migrate     Convert testify/suite tests to go-test format
  refactor    Source code refactoring tools (toggle-focus)
  lint        Run gotest-specific linter checks
  version     Print version information
  help        Show this help message

Flags (gotest):
  --ci                      Enable focus guard (fail on F_ prefixes)
  --debug                   Keep generated overlay for inspection
  --spec                    Append spec view after normal test output
  --update-snapshots        Regenerate snapshot files
  --min=<pct>               Fail if coverage below threshold (enables -coverprofile)
  --setup-timeout=<dur>     Shared fixture setup deadline (default 1m)
  --debounce=<dur>          Watch mode debounce interval (default 200ms)

gotest flags use --double-dash; go test flags use -single-dash.
Use a bare "--" to pass unrecognized flags to go test without validation.
`, about.ShortInfo())
}
