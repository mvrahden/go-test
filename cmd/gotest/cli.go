package main

import (
	"fmt"
	"os"
	"slices"

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
	case "scaffold":
		os.Exit(runScaffold(remaining))
	case "migrate":
		os.Exit(runMigrate(remaining))
	case "coverage":
		os.Exit(runCoverage(remaining))
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

		patterns := ExtractPackagePatterns(goTestArgs)
		cfg := ExecConfig{
			GoTestArgs:      goTestArgs,
			PackagePatterns: patterns,
		}

		os.Exit(Run(cfg))
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `%s — test suite runner for Go

Usage:
  gotest [subcommand] [flags] [packages...]

Subcommands:
  coverage    Report semantic test coverage (--min=<pct> for threshold)
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

All other flags and arguments are forwarded to "go test".
`, about.ShortInfo())
}

