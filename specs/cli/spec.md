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

### Requirement: Hooks that call t.T() under a fuzz or benchmark wrapper
gotest SHALL refuse with exit 2 a suite with fuzz or benchmark methods whose hooks call t.T(), in one error that names every such hook in source order and is located at the first. The cases are the table in `tests/conformance/cli/rejections_suite_test.go`.

#### Scenario: Several hooks call t.T()
- **WHEN** several hooks call t.T() in a suite with fuzz or benchmark methods
- **THEN** fails with exit 2 and one error on stderr that names every such hook in source order, located at the first
