package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/about"
)

var (
	DEBUG bool
	CLEAN bool
	CI    bool
)

func main() {
	subcmd, remaining := ParseSubcommand(os.Args[1:])

	switch subcmd {
	case "scaffold":
		os.Exit(runScaffold(remaining))
	case "migrate":
		os.Exit(runMigrate(remaining))
	case "version":
		fmt.Println(about.LongInfo())
		return
	case "clean":
		// Extract own flags from remaining
		ownArgs, goTestArgs := SplitArgs(remaining)
		DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
		runClean(goTestArgs)
		return
	case "help":
		printUsage()
		return
	default:
		// Default run mode: use original args (no subcommand consumed)
		args := os.Args[1:]
		ownArgs, goTestArgs := SplitArgs(args)
		DEBUG = slices.Contains(ownArgs, "-ƒƒ.internal.debug")
		CLEAN = slices.Contains(ownArgs, "-ƒƒ.clean")
		CI = slices.Contains(ownArgs, "--ci")

		if CLEAN {
			runClean(goTestArgs)
			return
		}

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
  generate    Generate test code (default behavior)
  clean       Remove generated test files
  scaffold    Scaffold a new test suite (not yet implemented)
  migrate     Migrate from testify/suite (not yet implemented)
  version     Print version information
  help        Show this help message

Flags:
  -ƒƒ.clean            Remove generated files only
  -ƒƒ.internal.debug   Keep generated files after test run
  --ci                  Enable focus guard (fail on F_ prefixes)

All other flags and arguments are forwarded to "go test".
`, about.ShortInfo())
}

func runClean(goTestArgs []string) {
	patterns := ExtractPackagePatterns(goTestArgs)
	for _, pattern := range patterns {
		dir := strings.TrimSuffix(pattern, "/...")
		if dir == "" {
			dir = "."
		}
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() && about.PSuiteRegex.MatchString(d.Name()) {
				fmt.Fprintf(os.Stdout, "removing %s\n", path)
				os.Remove(path)
			}
			return nil
		})
	}
}
