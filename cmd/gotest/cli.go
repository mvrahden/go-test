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
)

func main() {
	subcmd, remaining := ParseSubcommand(os.Args[1:])

	switch subcmd {
	case "discover":
		os.Exit(runDiscover(remaining))
	case "prepare":
		os.Exit(runPrepare(remaining))
	case "scaffold":
		os.Exit(runScaffold(remaining))
	case "migrate":
		os.Exit(runMigrate(remaining))
	case "generate":
		os.Exit(runGenerate(remaining))
	case "clean":
		os.Exit(runClean(remaining))
	case "spec":
		os.Exit(runSpec(remaining))
	case "watch":
		os.Exit(runWatch(remaining))
	case "refactor":
		os.Exit(runRefactor(remaining))
	case "lint":
		os.Exit(runLint(remaining))
	case "version":
		fmt.Println(about.LongInfo())
		return
	case "help":
		printUsage()
		return
	default:
		args := os.Args[1:]
		ownArgs, goTestArgs := SplitArgs(args)

		jsonMode, goTestArgs := stripJSONFlag(goTestArgs)

		minCoverage, err := parseMinFlag(ownArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			os.Exit(2)
		}
		setupTimeout, err := parseSetupTimeoutFlag(ownArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
			os.Exit(2)
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
					os.Exit(2)
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
			Spec:            slices.Contains(ownArgs, "--spec"),
			UpdateSnapshots: slices.Contains(ownArgs, "--update-snapshots"),
		}

		code := Run(cfg)

		if code == 0 && minCoverage > 0 {
			pct, err := readCoverageTotal(coverProfile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL: reading coverage: %s\n", err)
				os.Exit(2)
			}
			if pct < float64(minCoverage) {
				fmt.Fprintf(os.Stderr, "\nFAIL: %.1f%% coverage (minimum %d%%)\n", pct, minCoverage)
				os.Exit(1)
			}
		}

		os.Exit(code)
	}
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
  gotest [subcommand] [flags] [packages...]

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

Flags:
  --ci                      Enable focus guard (fail on F_ prefixes)
  --debug                   Keep generated overlay for inspection
  --spec                    Append spec view after normal test output
  --update-snapshots        Regenerate snapshot files
  --min=<pct>               Fail if coverage below threshold (enables -coverprofile)
  --setup-timeout=<dur>     Shared fixture setup deadline (default 1m)
  --debounce=<dur>          Watch mode debounce interval (default 200ms)

All other flags and arguments are forwarded to "go test".
`, about.ShortInfo())
}
