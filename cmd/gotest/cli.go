package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/about"
)

var (
	DEBUG            bool
	CI               bool
	SPEC             bool
	UPDATE_SNAPSHOTS bool
)

func main() {
	subcmd, remaining := ParseSubcommand(os.Args[1:])

	switch subcmd {
	case "discover":
		os.Exit(runDiscover(remaining))
	case "overlay":
		os.Exit(runOverlay(remaining))
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
	case "version":
		fmt.Println(about.LongInfo())
		return
	case "help":
		printUsage()
		return
	default:
		// Default run mode: use original args (no subcommand consumed)
		args := os.Args[1:]
		ownArgs, goTestArgs := SplitArgs(args)
		DEBUG = slices.Contains(ownArgs, "--debug")
		CI = slices.Contains(ownArgs, "--ci")
		SPEC = slices.Contains(ownArgs, "--spec")
		UPDATE_SNAPSHOTS = slices.Contains(ownArgs, "--update-snapshots")
		minCoverage := parseMinFlag(ownArgs)

		var coverProfile string
		if minCoverage > 0 {
			// Check if user already provided -coverprofile
			for _, arg := range goTestArgs {
				if strings.HasPrefix(arg, "-coverprofile=") {
					coverProfile = strings.TrimPrefix(arg, "-coverprofile=")
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

func parseMinFlag(args []string) int {
	for i, arg := range args {
		if strings.HasPrefix(arg, "--min=") {
			v, _ := strconv.Atoi(strings.TrimPrefix(arg, "--min="))
			return v
		}
		if arg == "--min" && i+1 < len(args) {
			v, _ := strconv.Atoi(args[i+1])
			return v
		}
	}
	return 0
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

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s — test suite runner for Go

Usage:
  gotest [subcommand] [flags] [packages...]

Subcommands:
  discover    Discover test suites and output JSON metadata
  overlay     Generate overlay files and output overlay path as JSON
  generate    Run code generation only (no test execution)
  clean       Remove orphaned generated files
  spec        Render behavioral specification from test suites
  watch       Watch for file changes and re-run tests
  scaffold    Generate test suite skeleton from a Go type
  migrate     Convert testify/suite tests to go-test format
  version     Print version information
  help        Show this help message

Flags:
  --ci                  Enable focus guard (fail on F_ prefixes)
  --debug               Keep generated overlay for inspection
  --spec                Append spec view after normal test output
  --update-snapshots    Regenerate snapshot files
  --min=<pct>           Fail if coverage below threshold (enables -coverprofile)

All other flags and arguments are forwarded to "go test".
`, about.ShortInfo())
}
