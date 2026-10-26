# CLI Specification

## Purpose
How the `gotest` command line builds, scaffolds, reports and introduces itself, on the invocations a user gives it.

## Requirements

### Requirement: Build rules
gotest SHALL build a package under go test's rules, failing to build with exit 2 where go test refuses the build.

#### Scenario: A linkname the linker refuses
- **WHEN** a package references a runtime symbol through a linkname the linker refuses
- **THEN** fails to build with exit 2, as under go test

### Requirement: A module without gotest
gotest SHALL name the go get -tool command to run when a suite imports gotest in a module that does not require it.

#### Scenario: gotest is not required
- **WHEN** a suite imports gotest in a module that does not require it
- **THEN** names the go get -tool command to run

### Requirement: A broken config
When .gotest.yml does not parse, version and help SHALL still print, and a run SHALL report the config error with exit 2. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`.

#### Scenario: A config that does not parse
- **WHEN** .gotest.yml does not parse
- **THEN** still prints version and help, and a run reports the config error with exit 2

### Requirement: Mistyped subcommands
gotest SHALL suggest the command meant when the first word is almost a subcommand or is the word test, and SHALL run any other word as a package pattern. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`.

#### Scenario: A first word that is almost a command
- **WHEN** the first word is almost a subcommand or is a help topic
- **THEN** suggests the command meant, and runs any other word as a package pattern

### Requirement: Packages with only stdlib tests
gotest SHALL note on stderr, in every mode, that a package has only stdlib tests. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`.

#### Scenario: A package without suites
- **WHEN** a package has only stdlib tests
- **THEN** notes them on stderr in the plain, -json, summary and spec modes

### Requirement: Boolean flags
Every boolean flag SHALL take --x, --x=true and --x=false, the last occurrence SHALL win, and any other value SHALL be a usage error. The cases are the table in `tests/conformance/cli/flags_suite_test.go`.

#### Scenario: Each spelling of a boolean flag
- **WHEN** a boolean flag is spelled in each accepted form
- **THEN** reads --x, --x=true and --x=false, lets the last occurrence win and refuses any other value

### Requirement: Value flags and unknown flags
Every command SHALL refuse with exit 2 a value flag given no value, a flag it does not know and a flag another command owns. The cases are the table in `tests/conformance/cli/flags_suite_test.go`.

#### Scenario: A flag a command cannot take
- **WHEN** a value flag or an unknown flag is given to a command
- **THEN** refuses a missing value, an unknown flag and another command's flag with exit 2

### Requirement: Test name patterns
A -run, -skip or -bench pattern the testing package would reject SHALL be refused with exit 2 before anything runs: each alternative and each level is compiled on its own, as the testing package splits the pattern, and a repeated flag is judged by its last value.

#### Scenario: An alternative that does not compile
- **WHEN** a -skip pattern compiles whole but one of its alternatives does not
- **THEN** is refused with exit 2 before anything runs, naming that alternative as go test does

### Requirement: Go test flags gotest implements
A go test flag gotest implements itself SHALL keep the meaning go test gives it, or SHALL be refused with a reason. The cases are the table in `tests/conformance/cli/goflags_suite_test.go`.

#### Scenario: A go test flag gotest takes over
- **WHEN** a go test flag gotest implements itself is given
- **THEN** keeps the meaning go test gives the flag, or refuses it with a reason

### Requirement: Flags for the test binary
A flag only the test binary declares SHALL reach the binary after -- or -args, as under go test. The cases are the table in `tests/conformance/cli/passthrough_suite_test.go`.

#### Scenario: A flag gotest does not know
- **WHEN** a flag only the test binary declares is passed
- **THEN** reaches the test binary after -- or -args, and the binary fails its check without it

### Requirement: Coverage profile spellings
gotest SHALL find a coverage profile in every spelling go test accepts, for the run's --min gate and for summary. The cases are the table in `tests/conformance/cli/coverage_suite_test.go`.

#### Scenario: A profile in each spelling
- **WHEN** a fully covered package runs with a minimum and a profile
- **THEN** finds the profile in every spelling go test accepts, for the --min gate and for summary

### Requirement: Scaffold paths
The scaffold subcommand SHALL write the scaffold beside the source file it is given, by a relative or an absolute path, even when the path to that file runs through a symlinked directory. The relative and absolute cases are the table in `tests/conformance/cli/scaffold_suite_test.go`.

#### Scenario: A relative or an absolute path
- **WHEN** a source file is named by path
- **THEN** scaffolds beside the file, by a relative or an absolute path

#### Scenario: A path through a symlinked directory
- **WHEN** a source file is named by an absolute path through a symlinked directory
- **THEN** scaffolds beside the file

### Requirement: The go test -json stream
The go test -json stream SHALL hold JSON lines only, inside GitHub Actions too.

#### Scenario: Asking for the stream
- **WHEN** the go test -json stream is asked for
- **THEN** holds JSON lines only, inside GitHub Actions too

### Requirement: JSON documents
A JSON document gotest is asked for SHALL be one document on stdout, with no color codes and no annotations, inside GitHub Actions too. The cases are the table in `tests/conformance/cli/streams_suite_test.go`.

#### Scenario: Asking for a document
- **WHEN** a JSON document is asked for
- **THEN** prints one JSON document with no color codes or annotations, inside GitHub Actions too

### Requirement: Color follows the terminal
gotest SHALL print no color codes to a stdout that is a pipe, whatever --no-color says. The cases are the table in `tests/conformance/cli/streams_suite_test.go`.

#### Scenario: Output into a pipe
- **WHEN** stdout is a pipe
- **THEN** prints no color codes, even with --no-color=false

### Requirement: One output mode
gotest SHALL refuse with exit 2 two output modes asked for at once, and an unknown --format. The cases are the table in `tests/conformance/cli/streams_suite_test.go`.

#### Scenario: Two output modes
- **WHEN** two output modes are asked for at once
- **THEN** refuses them, and an unknown --format, with exit 2

### Requirement: Version
The version subcommand SHALL exit 0 and print the version on stdout.

#### Scenario: Asking for the version
- **WHEN** the version subcommand runs
- **THEN** exits 0 and prints the version on stdout

### Requirement: The setup budget's default in the help
Every help text that lists --setup-timeout SHALL give its default as the fixtures' declared budgets, 30s until the first reports. The cases are the table in `tests/conformance/cli/help_suite_test.go`.

#### Scenario: A help text that lists --setup-timeout
- **WHEN** a help text lists --setup-timeout
- **THEN** gives its default as the fixtures' declared budgets, 30s until the first reports

### Requirement: A shared fixture with per-test hooks
gotest SHALL refuse with exit 2 a shared fixture that declares BeforeEach or AfterEach, in the suite's package or another: shared fixtures have no per-test hooks, so the hook would never run. The error is located at the hook and says where the per-test work goes instead.

#### Scenario: A shared fixture declares a per-test hook
- **WHEN** a shared fixture declares BeforeEach or AfterEach, in the suite's package or another
- **THEN** fails with exit 2 and one error on stderr, located at the hook, that says shared fixtures have no per-test hooks and where the work goes instead

### Requirement: Hooks that call t.T() where it is nil
gotest SHALL refuse with exit 2 a suite whose hook calls t.T() where its wrapper makes it nil: every hook under a benchmark method, BeforeAll and AfterAll under a fuzz method. One error names every such hook in source order, at the first. It reads the hook's own body, closures and aliases of t included, not the helpers it calls, and accepts a call whose result is checked for nil. The cases are the table in `tests/conformance/cli/rejections_suite_test.go`.

#### Scenario: Several hooks call t.T()
- **WHEN** several hooks call t.T() where their wrapper makes it nil
- **THEN** fails with exit 2 and one error on stderr that names every such hook in source order, located at the first

#### Scenario: t.T() reached other than by a plain call
- **WHEN** a hook reaches t.T() through parentheses, an alias of t or a method value
- **THEN** refuses it with exit 2 like a direct call

#### Scenario: A fuzz suite's per-input hooks
- **WHEN** a fuzz suite's BeforeEach and AfterEach call t.T()
- **THEN** runs the suite, each input with a real *testing.T

#### Scenario: A guarded call
- **WHEN** a hook checks t.T() for nil before it uses it
- **THEN** runs the suite

### Requirement: Generated files are the run's own
The files a run generates for its packages SHALL be the run's own: runs that share a cache SHALL never share or remove each other's, and a --debug run SHALL keep its own for inspection.

#### Scenario: Runs sharing a cache
- **WHEN** 16 runs of one package start at once and share one cache
- **THEN** passes every run

#### Scenario: A run ends
- **WHEN** a run ends
- **THEN** leaves none of the files it generated behind

#### Scenario: A --debug run ends
- **WHEN** a --debug run ends
- **THEN** names its overlay on stderr and keeps it

### Requirement: The test binary cache
A run's start SHALL evict from the test binary cache the binaries of a checkout that no longer exists, those unused for 7 days, then the least recently used until the cache is at most 5 GiB, and an eviction SHALL never fail a run or take a binary a run is linking or copying.

#### Scenario: A checkout that no longer exists
- **WHEN** a cache key records a checkout that no longer exists
- **THEN** a run removes its binaries

#### Scenario: A binary unused for 7 days
- **WHEN** a cached binary has not been used for 7 days
- **THEN** a run removes it and keeps the binaries used since

### Requirement: Saving the event stream
A run given --events-file SHALL write to that file byte for byte the go test -json stream it prints, and SHALL change none of its other output, coverage or exit code. Each event SHALL reach the file when it reaches stdout, a suite's events when its process ends, so a run stopped or killed leaves every event written before, each line whole. summary --input SHALL render the file as the run's summary and exit code, a --min limit excepted: the live run judges it.

#### Scenario: A -json run with --events-file
- **WHEN** a -json run is given --events-file
- **THEN** writes the bytes it prints to the file, making its directories

#### Scenario: Reading the file back
- **WHEN** a summary run's --events-file is read back with summary --input
- **THEN** renders the summary the run rendered, with its exit code

#### Scenario: Nothing else changes
- **WHEN** a summary with annotations and coverage is run with and without --events-file
- **THEN** prints, annotates, covers and exits the same

#### Scenario: A path that cannot be written
- **WHEN** --events-file names a path that cannot be written
- **THEN** exits 2 naming the flag before any package is built

#### Scenario: A run that does not pass
- **WHEN** a -json run that does not pass is given --events-file
- **THEN** writes its stream to the file, verdict included, and exits as without it

#### Scenario: A stopped run
- **WHEN** a -json run with --events-file is stopped while a suite holds
- **THEN** keeps the events of the suites that ended before the stop, each line whole
- **AND** writes the stopped suite's events too, as -json prints them

#### Scenario: A killed run
- **WHEN** a -json run with --events-file is killed while a suite holds
- **THEN** keeps the events of the suites that ended before the kill, each line whole
