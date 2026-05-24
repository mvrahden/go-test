package main

import (
	"fmt"
	"os"

	"github.com/mvrahden/go-test/about"
)

func containsHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" || a == "-help" {
			return true
		}
	}
	return false
}

func showHelp(topic string) {
	switch topic {
	case "", "help":
		printUsage()
	case "test":
		printTestHelp()
	case "spec":
		printSpecHelp()
	case "watch":
		printWatchHelp()
	case "discover":
		printDiscoverHelp()
	case "scaffold":
		printScaffoldHelp()
	case "lint":
		printLintHelp()
	case "migrate":
		printMigrateHelp()
	case "refactor":
		printRefactorHelp()
	case "generate":
		printGenerateHelp()
	case "prepare":
		printPrepareHelp()
	case "clean":
		printCleanHelp()
	case "config":
		printConfigHelp()
	case "version":
		fmt.Println(about.LongInfo())
	default:
		fmt.Fprintf(os.Stderr, "unknown help topic: %s\n\n", topic)
		printUsage()
	}
}

func printUsage() {
	fmt.Printf(`%s — test suite runner for Go

Usage:
  gotest [flags] [--] [go-test-flags] [packages...]
  gotest <subcommand> [flags] [packages...]

Subcommands:
  spec        Render behavioral specification from test output
  watch       Watch for file changes and re-run tests
  discover    Discover test suites and output JSON
  scaffold    Generate test suite skeleton
  lint        Run gotest-specific linter checks
  migrate     Convert testify/suite tests to go-test format
  refactor    Source code refactoring tools
  generate    Run code generation only (no test execution)
  prepare     Start shared fixtures for debug (blocks until SIGTERM)
  clean       Remove orphaned generated files
  version     Print version information

Run "gotest help <subcommand>" for subcommand-specific help.

Flags (gotest — use --double-dash):
  --ci                    Fail on focused tests (F_ prefixes)
  --debug                 Keep generated overlay for inspection
  --spec                  Render spec view instead of default output
  --update-snapshots      Regenerate snapshot files
  --min=<pct>             Fail if coverage < pct%% (0-100)
  --setup-timeout=<dur>   Shared fixture deadline (-1 to disable)

Flags (go test — use -single-dash, forwarded automatically):
  -v                      Verbose output
  -run=<regexp>           Run only matching tests
  -count=<n>              Run each test n times (default: cached)
  -race                   Enable data race detector
  -timeout=<dur>          Per-test timeout (default: 10m)
  -coverprofile=<file>    Write coverage profile
  -tags=<tags>            Build tags (comma-separated)
  -json                   Machine-readable JSON event stream

Use a bare "--" to separate gotest flags from go test flags,
or to pass unrecognized flags to go test without validation.

Configuration:
  Place a .gotest.yml in your project root. Run "gotest help config" for details.

Examples:
  gotest ./...                              Run all test suites
  gotest --ci --min=80 ./... -race          CI with coverage gate and race detection
  gotest spec --no-color -count=1 ./...     Spec report for CI artifacts
  gotest watch ./pkg/auth/...               Re-run on file changes
  gotest scaffold ./pkg/auth.ServiceImpl    Generate test suite skeleton
  gotest lint ./...                         Check for common suite mistakes
`, about.ShortInfo())
}

func printTestHelp() {
	fmt.Print(`gotest test — run test suites (default subcommand)

Usage:
  gotest [flags] [--] [go-test-flags] [packages...]

When no subcommand is given, gotest discovers test suites in the target
packages, generates an overlay with lifecycle wiring, and runs "go test".

Flags:
  --ci                    Fail on focused tests (F_ prefixes)
  --debug                 Keep generated overlay for inspection
  --spec                  Render spec view instead of default output
  --update-snapshots      Regenerate snapshot files
  --min=<pct>             Fail if coverage < pct% (0-100, enables -coverprofile)
  --setup-timeout=<dur>   Shared fixture deadline (-1 to disable)

All standard go test flags (-single-dash) are forwarded automatically.
Use a bare "--" to pass unrecognized flags without validation.

Examples:
  gotest ./...                              Run all suites
  gotest ./pkg/auth/... -v                  Verbose output for one package
  gotest --ci --min=80 ./... -race          CI pipeline with coverage gate
  gotest --debug ./pkg/auth/...             Keep overlay for inspection
  gotest -- -customflag ./...               Escape hatch for custom flags
`)
}

func printSpecHelp() {
	fmt.Print(`gotest spec — render behavioral specification from test suites

Usage:
  gotest spec [flags] [--] [go-test-flags] [packages...]
  gotest spec --input=<file> [--format=<fmt>] [--output=<file>]

Runs test suites and renders a BDD-style specification tree showing
pass/fail/skip status for each suite, method, and subtest.

Flags:
  --format=<fmt>          Output format: terminal (default), md, json
  --output=<file>         Write to file instead of stdout
  --input=<file>          Render from saved JSON ("-" for stdin)
  --no-color              Disable ANSI color codes
  --ci                    Fail on focused tests (F_ prefixes)
  --debug                 Keep generated overlay
  --update-snapshots      Regenerate snapshot files
  --min=<pct>             Fail if coverage < pct% (0-100)
  --setup-timeout=<dur>   Shared fixture deadline (-1 to disable)

Examples:
  gotest spec ./...                                Run and render spec
  gotest spec --format=md --output=spec.md ./...   Markdown report to file
  gotest spec --no-color --output=spec.txt ./...   Plain text for CI artifacts
  gotest spec --input=- < events.json              Render from piped JSON input
  gotest spec --format=json ./... -count=1         JSON tree, no caching
`)
}

func printWatchHelp() {
	fmt.Print(`gotest watch — watch for file changes and re-run tests

Usage:
  gotest watch [flags] [--] [go-test-flags] [packages...]

Watches .go files under the target packages and re-runs affected tests
on every change. The initial run covers all target packages; subsequent
runs are scoped to the packages containing changed files.

Flags:
  --debounce=<dur>        Re-run delay after change (default: 200ms)
  --ci                    Fail on focused tests (F_ prefixes)
  --debug                 Keep generated overlay
  --update-snapshots      Regenerate snapshot files
  --setup-timeout=<dur>   Shared fixture deadline (-1 to disable)

Examples:
  gotest watch ./...                         Watch all packages
  gotest watch ./pkg/auth/... -v             Watch with verbose output
  gotest watch --debounce=500ms ./...        Slower debounce for large projects
`)
}

func printDiscoverHelp() {
	fmt.Print(`gotest discover — discover test suites and output JSON metadata

Usage:
  gotest discover [packages...]

Loads test packages, discovers suite types and their methods, and outputs
structured JSON. Used by IDE extensions for test explorer integration.

Output schema:
  {
    "packages": [{
      "importPath": "example.com/pkg",
      "dir":        "/absolute/path",
      "suites": [{
        "name":     "UserTestSuite",
        "file":     "user_test.go",
        "line":     10,
        "col":      1,
        "focused":  false,
        "excluded": false,
        "guarded":  false,
        "methods": [{
          "name": "TestCreate",
          "file": "user_test.go",
          "line": 15,
          "col":  1,
          "focused":  false,
          "excluded": false,
          "parallel": false
        }]
      }]
    }],
    "warnings": [{"importPath": "...", "message": "..."}]
  }

Examples:
  gotest discover ./...                      All packages
  gotest discover ./pkg/auth/...             Single package tree
`)
}

func printScaffoldHelp() {
	fmt.Print(`gotest scaffold — generate test suite skeleton

Usage:
  gotest scaffold <target>

Target is one of:
  ./pkg/path.TypeName      Generate suite for a specific type
  ./pkg/path/file.go       Generate suites for all types in a file

Creates a new _test.go file with a suite struct, a runner function,
and stub methods for each exported method on the target type.

Examples:
  gotest scaffold ./pkg/auth.UserService        Suite for UserService type
  gotest scaffold ./pkg/auth/service.go         Suites for all types in file
`)
}

func printLintHelp() {
	fmt.Print(`gotest lint — run gotest-specific linter checks

Usage:
  gotest lint [flags] [packages...]

Checks for common mistakes in gotest test suites. Defaults to ./... if
no packages are specified.

Rules:
  focus           Focused (F_) suites/methods should not be committed
  receiver        Suite methods should use pointer receivers
  lifecycle-typo  Methods similar to lifecycle hooks (likely typos)
  lifecycle-pair  BeforeAll without matching AfterAll (resource leak)
  generated-file  Generated overlay files should not be in version control
  stdlib-test     Stdlib test functions — consider using gotest suites
  testify         testify imports — consider migrating to gotest

Flags:
  -skip-stdlib-test       Disable the stdlib-test rule
  -skip-testify           Disable the testify rule
  -fix                    Apply suggested fixes

Suppress individual diagnostics with //nolint comments:
  //nolint:stdlib-test             Suppress on same line or line above
  package foo //nolint:stdlib-test  Suppress for entire file

Examples:
  gotest lint ./...                          Lint all packages
  gotest lint -skip-testify ./...            Skip testify rule
  gotest lint -fix ./...                     Auto-fix where possible
`)
}

func printMigrateHelp() {
	fmt.Print(`gotest migrate — convert testify/suite tests to go-test format

Usage:
  gotest migrate [packages...]

Scans for testify/suite patterns and rewrites them into gotest suite
format. Defaults to the current package if no patterns are given.

Examples:
  gotest migrate ./...                       Migrate all packages
  gotest migrate ./pkg/auth                  Migrate one package
`)
}

func printRefactorHelp() {
	fmt.Print(`gotest refactor — source code refactoring tools

Usage:
  gotest refactor <command> [args]

Commands:
  toggle-focus <file> <identifier>

    Toggle the F_ prefix on a suite type or method. The identifier is
    either "SuiteName" (toggles the suite) or "SuiteName.MethodName"
    (toggles a single method).

Examples:
  gotest refactor toggle-focus user_test.go UserTestSuite
  gotest refactor toggle-focus user_test.go UserTestSuite.TestCreate
`)
}

func printGenerateHelp() {
	fmt.Print(`gotest generate — run code generation only

Usage:
  gotest generate [-tags=<tags>] [packages...]

Generates the overlay files that gotest normally creates transparently
during test runs. Useful for inspecting generated code or integrating
with custom build pipelines.

Examples:
  gotest generate ./...                      Generate for all packages
  gotest generate -tags=integration ./...    With build tags
`)
}

func printPrepareHelp() {
	fmt.Print(`gotest prepare — prepare debug session

Usage:
  gotest prepare [packages...]

Generates the test overlay and starts shared fixtures, then blocks until
SIGTERM. Used by IDE extensions to prepare Delve debug sessions.

Outputs JSON to stdout:
  {"overlayFile": "...", "dir": "...", "stateFile": "..."}

Examples:
  gotest prepare ./pkg/auth/...              Prepare auth package for debug
`)
}

func printCleanHelp() {
	fmt.Print(`gotest clean — remove orphaned generated files

Usage:
  gotest clean [packages...]

Removes generated overlay files (ƒƒ_*_test.go) that are no longer
needed. These files are normally ephemeral but may be left behind
by --debug runs or interrupted processes.

Examples:
  gotest clean ./...                         Clean all packages
`)
}

func printConfigHelp() {
	fmt.Print(`gotest configuration — .gotest.yml project settings

gotest reads project settings from a .gotest.yml file. The file is found
by walking up from the working directory, stopping at the first match or
at a go.mod boundary. CLI flags override config values.

Fields:
  tags: <string>            Build tags, comma-separated (e.g., "integration,e2e")
  setup-timeout: <duration> Shared fixture deadline (e.g., "2m", -1 to disable)
  min-coverage: <int>       Minimum coverage percentage, 0-100
  debounce: <duration>      Watch mode re-run delay (e.g., "500ms", default: 200ms)
  lint:
    skip: [<rule>, ...]     Lint rules to disable globally

Skippable lint rules: stdlib-test, testify

Example .gotest.yml:

  tags: integration
  setup-timeout: 2m
  min-coverage: 80
  lint:
    skip:
      - testify
`)
}
