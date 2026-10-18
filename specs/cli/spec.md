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

### Requirement: Scaffold paths
The scaffold subcommand SHALL write the scaffold beside the source file it is given, even when the path to that file runs through a symlinked directory.

#### Scenario: A path through a symlinked directory
- **WHEN** a source file is named by an absolute path through a symlinked directory
- **THEN** scaffolds beside the file

### Requirement: The go test -json stream
The go test -json stream SHALL hold JSON lines only, inside GitHub Actions too.

#### Scenario: Asking for the stream
- **WHEN** the go test -json stream is asked for
- **THEN** holds JSON lines only, inside GitHub Actions too

### Requirement: Version
The version subcommand SHALL exit 0 and print the version on stdout.

#### Scenario: Asking for the version
- **WHEN** the version subcommand runs
- **THEN** exits 0 and prints the version on stdout
