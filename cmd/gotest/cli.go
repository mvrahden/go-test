package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/mvrahden/go-test/about"
)

var (
	DEBUG bool
	CI    bool
	SPEC  bool
)

func main() {
	subcmd, remaining := ParseSubcommand(os.Args[1:])

	switch subcmd {
	case "scaffold":
		os.Exit(runScaffold(remaining))
	case "migrate":
		os.Exit(runMigrate(remaining))
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
		DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
		CI = slices.Contains(ownArgs, "--ci")
		SPEC = slices.Contains(ownArgs, "--spec")

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
  spec        Render behavioral specification from test suites
  watch       Watch for file changes and re-run tests
  scaffold    Generate test suite skeleton from a Go type
  migrate     Convert testify/suite tests to go-test format
  version     Print version information
  help        Show this help message

Flags:
  --ci                  Enable focus guard (fail on F_ prefixes)
  --spec                Append spec view after normal test output

All other flags and arguments are forwarded to "go test".
`, about.ShortInfo())
}

