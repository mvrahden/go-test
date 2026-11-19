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

### Requirement: A vendored module
When a module's vendor directory lacks pkg/gotestruntime, which only the generated harness imports, the build failure of each suite package SHALL name the go get -tool declaration that makes go mod vendor copy it, and -mod=mod.

#### Scenario: A module vendored without the runtime
- **WHEN** a suite runs in a module whose vendor directory lacks pkg/gotestruntime
- **THEN** fails to build with exit 2, naming the tool declaration to add and -mod=mod

### Requirement: A module that only requires gotest
gotest SHALL run the suites of a module that requires gotest and declares no tool directive, with the go.mod and go.sum go mod tidy leaves it.

#### Scenario: A tidied module without the tool directive
- **WHEN** a tidied module requires gotest and declares no tool directive
- **THEN** runs its suites and exits 0

### Requirement: A broken config
When .gotest.yml does not parse, version and help SHALL still print, and a run SHALL report the config error with exit 2. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`. A value its flag would refuse, or a second YAML document that holds content, SHALL be reported the same way, naming the key and the value or the document's line; an empty one, such as a trailing ---, SHALL be read past.

#### Scenario: A config that does not parse
- **WHEN** .gotest.yml does not parse
- **THEN** still prints version and help, and a run reports the config error with exit 2

#### Scenario: A value its flag would refuse
- **WHEN** .gotest.yml sets parallel, compile-parallel or min-coverage to a value its flag refuses, or holds a second document
- **THEN** a run reports the key and the value with exit 2

### Requirement: Config defaults a command cannot take
A timeout or setup-timeout in .gotest.yml SHALL apply only to the commands that take --timeout or --setup-timeout. gotest fuzz, whose clock is --for, SHALL ignore both and SHALL still refuse --timeout given as a flag.

#### Scenario: A config timeout under gotest fuzz
- **WHEN** .gotest.yml sets timeout and setup-timeout and gotest fuzz runs
- **THEN** starts the session as without them
- **AND** still refuses --timeout given as a flag, with exit 2

### Requirement: Mistyped subcommands
gotest SHALL suggest the command meant when the first word is almost a subcommand or is the word test, and SHALL run any other word as a package pattern, a word the go command resolves to a package included. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`.

#### Scenario: A first word that is almost a command
- **WHEN** the first word is almost a subcommand or is a help topic
- **THEN** suggests the command meant, and runs any other word as a package pattern

### Requirement: Packages with only stdlib tests
gotest SHALL note on stderr, in every mode, that a package has only stdlib tests. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`. The Markdown summary and the GitHub step summary SHALL carry the note under their headline, and a summary with the note SHALL not say All.

#### Scenario: A package without suites
- **WHEN** a package has only stdlib tests
- **THEN** notes them on stderr in the plain, -json, summary and spec modes

#### Scenario: A suite package that also holds stdlib tests
- **WHEN** gotest summary writes Markdown for a package that holds a suite and a stdlib test
- **THEN** the note of the stdlib tests not run follows the headline
- **AND** the headline does not say All
- **AND** the GitHub step summary carries the note

### Requirement: A suite that ran no tests
In a text run given no -run, -skip, -list or -count=0, nor -test.run, -test.skip, -test.list, -test.bench or -test.count=0 after -args, a suite process whose binary reports that it ran no tests SHALL fail its package and the run with exit 2, naming the suite: the function gotest generated for it was not in the binary. A suite that is guarded, whose methods are all excluded, that focus leaves out or that the user's pattern does not match SHALL NOT fail this way.

#### Scenario: A suite whose binary runs none of its tests
- **WHEN** a suite's binary runs none of its tests in a text run without a pattern
- **THEN** fails the package and exits 2, naming the suite as one that ran no tests

#### Scenario: Suites that legitimately run nothing
- **WHEN** a text run covers a guarded suite, a suite whose methods are all excluded, a run narrowed by focus and a -run that matches nothing
- **THEN** passes each, the last noted as no tests to run

#### Scenario: The binary's own selection flags
- **WHEN** a text run gives the binary -test.count=0, -test.run, -test.skip, -test.list or -test.bench after -args
- **THEN** passes each, never naming a suite as one that ran no tests

### Requirement: Benchmarks a test run leaves out
A test run SHALL note on stderr, in every mode, how many benchmarks its packages declare that it did not run, and that gotest bench runs them. The cases are the table in `tests/conformance/cli/newcomer_suite_test.go`.

#### Scenario: A package that declares a benchmark
- **WHEN** a test run covers a package that declares a benchmark
- **THEN** notes on stderr that 1 benchmark was not run and gotest bench runs it, in the plain, -json, summary and spec modes

### Requirement: Packages with testify suites
gotest SHALL run the gotest suites of a package that holds a testify suite and SHALL leave the testify suite to go test, noting its runner as a stdlib test.

#### Scenario: A testify suite beside a gotest suite
- **WHEN** a package holds a testify suite beside a gotest suite
- **THEN** runs the gotest suite, notes the testify runner as a stdlib test go test runs, and exits 0

### Requirement: Boolean flags
Every boolean flag SHALL take --x, --x=true and --x=false, the last occurrence SHALL win, and any other value SHALL be a usage error. The cases are the table in `tests/conformance/cli/flags_suite_test.go`.

#### Scenario: Each spelling of a boolean flag
- **WHEN** a boolean flag is spelled in each accepted form
- **THEN** reads --x, --x=true and --x=false, lets the last occurrence win and refuses any other value

### Requirement: The focus guard
With --ci, a run SHALL fail with exit 1 before anything runs when a suite or any of its test, benchmark or fuzz methods carries the F_ prefix, naming each where it is declared.

#### Scenario: A focused benchmark method
- **WHEN** --ci runs a package whose only focused method is a benchmark
- **THEN** fails with exit 1, naming the benchmark where it is declared

#### Scenario: A focused fuzz method
- **WHEN** --ci runs a package whose only focused method is a fuzz target
- **THEN** fails with exit 1, naming the fuzz target where it is declared

### Requirement: A focused run outside CI mode
Outside CI mode, a run, gotest spec or gotest summary that holds a focused suite or method SHALL say on stderr, before anything runs, that F_ narrows it, how many items carry the prefix and where the first is declared, and SHALL exit by what ran.

#### Scenario: A focused method outside CI mode
- **WHEN** a run outside CI mode holds a focused method
- **THEN** notes on stderr that F_ narrows the run, naming the method where it is declared, in the plain and -json modes
- **AND** exits 0 when what ran passed

#### Scenario: A focused method under spec and summary
- **WHEN** spec or summary runs, outside CI mode, a package holding a focused method
- **THEN** spec notes on stderr that F_ narrows the run, naming the method where it is declared, and exits 0
- **AND** summary notes on stderr that F_ narrows the run, naming the method where it is declared, and exits 0

### Requirement: Value flags and unknown flags
Every command SHALL refuse with exit 2 a value flag given no value, a flag it does not know and a flag another command owns. The cases are the table in `tests/conformance/cli/flags_suite_test.go`.

#### Scenario: A flag a command cannot take
- **WHEN** a value flag or an unknown flag is given to a command
- **THEN** refuses a missing value, an unknown flag and another command's flag with exit 2

### Requirement: The control flag
--control SHALL take one value, stdin, on every command that stops on a SIGINT: a run, spec, summary, watch, bench, fuzz, fuzz triage, fuzz promote and prepare. Any other value SHALL be a usage error with exit 2 naming the flag and stdin, and so SHALL --control=stdin beside --input=-. The cases are the table in `tests/conformance/cli/control_suite_test.go`.

#### Scenario: A value other than stdin
- **WHEN** --control is given a value other than stdin
- **THEN** refuses it with exit 2, naming --control and stdin

#### Scenario: Every command that stops on a SIGINT
- **WHEN** --control=stdin is given to each command that stops on a SIGINT, its standard input at its end
- **THEN** takes the flag and ends, stopped by the input's end where it was still running

#### Scenario: Beside input from standard input
- **WHEN** --control=stdin is given beside --input=-
- **THEN** refuses it with exit 2: both read standard input

### Requirement: Test name patterns
A -run, -skip, -bench or -list pattern the testing package would reject SHALL be refused with exit 2 before anything runs: each alternative and each level of a -run, -skip or -bench pattern is compiled on its own, as the testing package splits the pattern, a -list pattern is compiled whole, as the testing package reads it, and a repeated flag is judged by its last value.

#### Scenario: An alternative that does not compile
- **WHEN** a -skip pattern compiles whole but one of its alternatives does not
- **THEN** is refused with exit 2 before anything runs, naming that alternative as go test does

#### Scenario: A -list pattern that does not compile
- **WHEN** a -list pattern does not compile
- **THEN** is refused with exit 2 before anything runs, naming the pattern as go test does

### Requirement: A -run pattern in each suite process
Each suite process SHALL run only its own suite under a -run pattern: for every alternative whose first level matches the suite's Test function or one of its fuzz wrappers, the process SHALL get that alternative with its first level replaced by the function's exact name, the later levels as written. A suite SHALL so run once however many suites the first level matches, an Exclusive suite only in its own slot, a suite binding a shared fixture only where its state is.

#### Scenario: A subtest pattern whose first level matches two suites
- **WHEN** -run names a method under a first level that matches two suites of a package
- **THEN** runs each suite's hooks and the method once, each suite in its own process

#### Scenario: A subtest pattern beside a suite that binds a shared fixture
- **WHEN** -run /TestState runs a package holding a suite that binds a shared fixture beside one that binds none
- **THEN** passes, the fixture's suite running only in the process given its state

#### Scenario: A pattern naming only a suite's Test function
- **WHEN** -run names only the Test function of a suite that also has a fuzz method
- **THEN** runs the test methods and replays no fuzz seed, as go test does

### Requirement: Go test flags gotest implements
A go test flag gotest implements itself SHALL keep the meaning go test gives it, or SHALL be refused with a reason. The cases are the table in `tests/conformance/cli/goflags_suite_test.go`.

#### Scenario: A go test flag gotest takes over
- **WHEN** a go test flag gotest implements itself is given
- **THEN** keeps the meaning go test gives the flag, or refuses it with a reason

#### Scenario: A user overlay
- **WHEN** -overlay is given to a run
- **THEN** refuses it with exit 2 before anything is built, saying gotest compiles its harness through an overlay of its own
- **AND** still compiles its harness when GOFLAGS carries an -overlay

#### Scenario: A build-only mode or the fuzz engine
- **WHEN** -c, -n or -fuzz is given to a run
- **THEN** refuses each with exit 2, naming what does that job instead

#### Scenario: A profile flag over several suite processes
- **WHEN** -cpuprofile, -memprofile, -blockprofile, -mutexprofile or -trace is given to a run that starts two suite processes
- **THEN** refuses it with exit 2, naming the flag and how many processes would write its file
- **AND** a run of one suite writes the profile

#### Scenario: Listing a package's tests
- **WHEN** -list is given a pattern over a package with two suites, one binding a shared fixture
- **THEN** prints each matching name once and exits 0
- **AND** starts no suite and no shared fixture

### Requirement: Test binary flags follow the testing package
gotest SHALL take as a go test flag every flag the testing package it was built with gives a test binary for its user to set, with the value the testing package gives it, and SHALL refuse a single-dash flag ahead of -- and -args that is neither such a flag nor a build flag of go test, nor a flag gotest implements itself, with exit 2 before anything is built. A gotest built with a newer Go than the one that builds the test binary can take a flag that binary does not declare.

#### Scenario: A flag the testing package declares
- **WHEN** -artifacts and -mutexprofilefraction=5 are given to a run
- **THEN** takes both, and the run passes with exit 0

#### Scenario: A flag the testing package does not declare
- **WHEN** -mutexprofilerate=5 is given to a run
- **THEN** refuses it with exit 2 as an unknown flag, before anything is built

### Requirement: Flags for the test binary
A flag only the test binary declares SHALL reach the binary after -- or -args. Every word after -args SHALL reach the binary as written: gotest reads none of them as a flag of its own, as a go test flag or as a package. After a bare -- gotest SHALL read no flag of its own, while go test flags and packages still count. The cases are the table in `tests/conformance/cli/passthrough_suite_test.go`.

#### Scenario: A flag gotest does not know
- **WHEN** a flag only the test binary declares is passed
- **THEN** reaches the test binary after -- or -args, and the binary fails its check without it

#### Scenario: Words after -args that gotest also knows
- **WHEN** -json, --spec, -o or -h follows -args
- **THEN** each reaches the test binary, which refuses it as a flag it does not define, and gotest keeps its text mode

### Requirement: Coverage profile spellings
gotest SHALL find a coverage profile in every spelling go test accepts, for the run's --min gate and for summary. The cases are the table in `tests/conformance/cli/coverage_suite_test.go`.

#### Scenario: A profile in each spelling
- **WHEN** a fully covered package runs with a minimum and a profile
- **THEN** finds the profile in every spelling go test accepts, for the --min gate and for summary

### Requirement: Merged coverage counts
A -coverprofile under -covermode=count or atomic SHALL hold, for each block, the sum of the counts of the package's suite processes that ran it: a block a test runs is counted as go test's one process counts it, but code that runs at package initialisation is counted once per suite process. Under set a block any process ran SHALL read 1.

#### Scenario: A block two suites run
- **WHEN** two suites of a package each run a block once under -covermode=count
- **THEN** the profile counts the block twice

### Requirement: Scaffold paths
The scaffold subcommand SHALL write the scaffold beside the source file it is given, or into the directory of the package a type or function target names, by a relative or an absolute path, even when the path runs through a symlinked directory and when the working directory is outside the module. The relative and absolute cases are the tables in `tests/conformance/cli/scaffold_suite_test.go`.

#### Scenario: A relative or an absolute path
- **WHEN** a source file is named by path
- **THEN** scaffolds beside the file, by a relative or an absolute path

#### Scenario: A path through a symlinked directory
- **WHEN** a source file is named by an absolute path through a symlinked directory
- **THEN** scaffolds beside the file

#### Scenario: A package named by an absolute path
- **WHEN** a type or a function is named by an absolute package path from outside its module
- **THEN** scaffolds into the package's directory

#### Scenario: A package path through a symlinked directory
- **WHEN** a type is named by an absolute package path through a symlinked directory
- **THEN** scaffolds into the package's directory

#### Scenario: A symlinked working directory
- **WHEN** a type is named by a relative package path from a symlinked working directory
- **THEN** scaffolds into the package's directory, named relative to the working directory

### Requirement: Scaffolds that compile
The scaffold subcommand SHALL write only code that compiles. It SHALL refuse with exit 1 a generic type, a generic function and a method of a generic type, a suite name the package already declares in any of its files or its external test package's, and a type without an exported method or a file without an exported function, whose suite would declare no case, writing nothing. The target shapes are the tables in `tests/conformance/cli/scaffold_compiles_suite_test.go`.

#### Scenario: A generic target
- **WHEN** a generic type, a generic function or a method of a generic type is scaffolded
- **THEN** refuses it on stderr with exit 1 and writes nothing

#### Scenario: A suite name the package declares
- **WHEN** the package already declares the suite name scaffold would write
- **THEN** refuses it on stderr with exit 1, naming the declaration and its file, and writes nothing

#### Scenario: A target without a case
- **WHEN** a type without an exported method or a file without an exported function is scaffolded
- **THEN** refuses it on stderr with exit 1, saying its suite would declare no case, and writes nothing

#### Scenario: Each target shape
- **WHEN** each target shape is scaffolded
- **THEN** writes a file go vet passes

### Requirement: The go test -json stream
The go test -json stream SHALL hold JSON lines only, inside GitHub Actions too. Every event of a package the run books for itself rather than for an import path (`shared fixtures`, `interrupted`, `global --timeout`, `go build`) SHALL carry `"RunLevel":true`, its start line included, and no event of another package SHALL carry it; spec's JSON document SHALL mark such a package `"runLevel":true`. A consumer that decodes the events with encoding/json into go test's own event type ignores the field.

#### Scenario: Asking for the stream
- **WHEN** the go test -json stream is asked for
- **THEN** holds JSON lines only, inside GitHub Actions too

#### Scenario: A package the run books for itself
- **WHEN** a -json run's shared fixture does not come up
- **THEN** marks every event of the shared fixtures package RunLevel, its start line included, and no event of another package
- **AND** spec's JSON document of that stream marks the shared fixtures package runLevel, and no other

### Requirement: Where a test event's test sits
A test event of a suite process in the go test -json stream SHALL carry an object `"Gotest"` before its final `}` when one of its fields applies: `Subtest`, the own level as reported, on every `run`; `Repeat`, the NN of testing's `#NN`, on a run that has it; `Instance` on every event of the second or later run of a top-level name, the 1-based run. A field that does not apply is left out, and an event none applies to carries no object. Every other byte SHALL be go test's.

#### Scenario: Where a subtest sits
- **WHEN** gotest -json runs a subtest
- **THEN** its run event carries its own level in Gotest.Subtest
- **AND** every field go test wrote is kept as go test wrote it

#### Scenario: A suite of both test packages in the stream
- **WHEN** gotest -json runs a suite of both test packages
- **THEN** every event of the external suite carries Gotest.Instance 2

### Requirement: Stamping a saved stream
`gotest spec --input=<file> --stamp` SHALL write the stream back with the object `"Gotest"` added to each test event that carries none, as gotest -json adds it, placed by the stream's own order, every other byte kept. It SHALL exit 0, and 2 when the input cannot be read or --input is missing.

#### Scenario: A stream saved without its stamps
- **WHEN** a stream saved without its stamps is passed through spec --stamp
- **THEN** comes back as gotest -json wrote it

### Requirement: JSON documents
A JSON document gotest is asked for SHALL be one document on stdout, with no color codes and no annotations, inside GitHub Actions too. The cases are the table in `tests/conformance/cli/streams_suite_test.go`.

#### Scenario: Asking for a document
- **WHEN** a JSON document is asked for
- **THEN** prints one JSON document with no color codes or annotations, inside GitHub Actions too

### Requirement: Color follows the terminal
gotest SHALL print no color codes to a stdout that is a pipe, whatever --no-color says, nor into a file --output names, whatever stdout is. The cases are the table in `tests/conformance/cli/streams_suite_test.go`.

#### Scenario: Output into a pipe
- **WHEN** stdout is a pipe
- **THEN** prints no color codes, even with --no-color=false

#### Scenario: A report written to a file
- **WHEN** spec or summary writes its report to a file --output names while stdout is a character device
- **THEN** the file holds no color codes

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

### Requirement: A group that is not a constant
gotest SHALL refuse with exit 2 a suite whose SuiteConfig sets ExclusiveGroup to anything but a string literal or a string constant, of its own package or another: the runner schedules by the group before anything runs. The error is located at the value and names the forms it accepts.

#### Scenario: A group read at run time
- **WHEN** a suite's ExclusiveGroup is neither a string literal nor a string constant
- **THEN** fails with exit 2, located at the value, naming the forms it accepts

### Requirement: Selecting groups
--group=<name> SHALL narrow a run to the suites of that ExclusiveGroup and --skip-group=<name> SHALL leave that group's suites out; both repeat. Shared fixtures SHALL follow the selection: one no selected suite reads never starts. A name no suite declares SHALL exit 2 before any build, naming the known groups; summary and spec with --input, and spec --static, SHALL refuse both flags with exit 2.

#### Scenario: Only one group
- **WHEN** --group=e2e is given
- **THEN** runs only the e2e group's suites and starts only their fixtures

#### Scenario: All but one group
- **WHEN** --skip-group=e2e is given
- **THEN** runs every suite outside the e2e group and starts none of its fixtures

#### Scenario: A group nobody declares
- **WHEN** --group or --skip-group names a group no suite declares
- **THEN** exits 2 before any build, naming the known groups

#### Scenario: A command that runs nothing
- **WHEN** --group or --skip-group is given to a command that runs no suite
- **THEN** exits 2, saying the command runs nothing

### Requirement: Groups in the summary
gotest summary SHALL print one line per named ExclusiveGroup that ran, with its suites and its wall time from its first suite's start to its last suite's end, in its terminal and Markdown summaries and the GitHub step summary, and its JSON document SHALL carry a matching groups entry. A group none of whose suites ran, all stopped or given up, SHALL have neither.

#### Scenario: A summary of grouped suites
- **WHEN** gotest summary runs suites of named groups
- **THEN** prints one line per named group with its suites and wall time
- **AND** prints the same lines in the Markdown summary and the GitHub step summary
- **AND** puts a groups entry with each group's suites and wall time in the JSON summary

### Requirement: The summary's headline
gotest summary's first line SHALL count as passed only the tests that passed and SHALL name beside them the tests that were skipped and those that were not run, in its terminal and Markdown summaries and the GitHub step summary. It SHALL say All only when every test passed, and SHALL claim no pass of a run in which none did. A run with failed tests SHALL open with their count, then the passed, skipped and not-run counts that are not zero.

#### Scenario: A run in which every test passed
- **WHEN** gotest summary runs a package whose tests all pass
- **THEN** the Markdown headline says All and the count of tests

#### Scenario: A run with skipped tests
- **WHEN** gotest summary runs a package with passed and skipped tests
- **THEN** the headline counts the passed tests and names the skipped ones
- **AND** the Markdown headline does not say All
- **AND** the GitHub step summary carries the Markdown headline

#### Scenario: A run in which nothing passed
- **WHEN** gotest summary runs a package whose tests are all skipped
- **THEN** the headline says no tests passed, names the skipped ones and exits 0

#### Scenario: A run with failed, passed and skipped tests
- **WHEN** gotest summary runs a package with failed, passed and skipped tests
- **THEN** the headline opens with the failed count, then the passed and the skipped

### Requirement: Annotations of a summary
gotest summary --github SHALL annotate a failed test at its file in its package's directory, and a package that failed to build at the path the compiler printed, which already names the file from the working directory.

#### Scenario: A package that fails to build
- **WHEN** gotest summary --github runs a package that fails to build
- **THEN** annotates the compiler's error at the file's path from the working directory

#### Scenario: A failed test
- **WHEN** gotest summary --github runs a package with a failed test
- **THEN** annotates the failure at the test file in its package's directory

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
The files a run generates for its packages SHALL be the run's own: runs that share a cache SHALL never share or remove each other's, and a --debug run SHALL keep its own for inspection. A coverage profile the run needs and the user did not name SHALL lie in a work directory stamped as the run's own, so a later run removes it if this one is killed.

#### Scenario: Runs sharing a cache
- **WHEN** 16 runs of one package start at once and share one cache
- **THEN** passes every run

#### Scenario: A run ends
- **WHEN** a run ends
- **THEN** leaves none of the files it generated behind

#### Scenario: A --debug run ends
- **WHEN** a --debug run ends
- **THEN** names its overlay on stderr and keeps it

#### Scenario: A run with a coverage minimum ends
- **WHEN** a run given --min and no -coverprofile ends
- **THEN** leaves no coverage profile in the temp directory

### Requirement: The test binary cache
A run's start SHALL evict from the test binary cache the binaries of a checkout that no longer exists, those unused for 7 days, then the least recently used until the cache is at most 5 GiB, and an eviction SHALL never fail a run or take a binary a run is linking or copying. A run SHALL reuse a cached binary only when it still has the size and checksum recorded when it was last linked; one that does not, or has no record, SHALL be linked again.

#### Scenario: A checkout that no longer exists
- **WHEN** a cache key records a checkout that no longer exists
- **THEN** a run removes its binaries

#### Scenario: A binary unused for 7 days
- **WHEN** a cached binary has not been used for 7 days
- **THEN** a run removes it and keeps the binaries used since

#### Scenario: A cached binary damaged since it was linked
- **WHEN** a cached test binary is truncated, or zero-filled at its end, after a run linked it
- **THEN** the next run links it again, says so on stderr, and passes

#### Scenario: A cached binary no run recorded
- **WHEN** a cached test binary has no record beside it
- **THEN** the next run links it again without a word and records it

### Requirement: Packages a run builds
A run SHALL build a suite package only when it dispatches one of its suites. A package whose suites -run, -skip, a group flag or focus all leave out SHALL be reported as go test reports a package that ran no tests, without being built, so a build or vet failure in it is not that run's.

#### Scenario: A package the filter leaves out
- **WHEN** -run names a suite of one package and another package's suite fails vet
- **THEN** exits 0 and reports the other package as having no tests to run

#### Scenario: The same packages without the filter
- **WHEN** the two packages are run without -run
- **THEN** exits 2 and reports the vet finding under its package

### Requirement: Suite processes at once
A run SHALL start at most GOMAXPROCS suite processes at once, half of that under -race, -msan or -asan, and exactly the number -p gives when -p is given, whatever GOMAXPROCS is. The default --parallel budget SHALL be twice that number.

#### Scenario: -p above GOMAXPROCS
- **WHEN** three suites that each wait for the other two are run with -p 3 and GOMAXPROCS=1
- **THEN** all three pass

#### Scenario: No -p
- **WHEN** the same suites are run with GOMAXPROCS=1 and no -p
- **THEN** each fails waiting, since one process runs at a time

### Requirement: Building several packages
A run SHALL build its packages together and report each build failure under the package it belongs to, with the diagnostics go test prints for that package alone. A package that built SHALL run whether or not another failed, and SHALL start as soon as its binary is built or confirmed up to date where the platform lets gotest see that. Two packages whose import paths end in the same element SHALL both be built and run.

#### Scenario: Two packages that fail vet beside one that passes
- **WHEN** two packages each fail vet and a third passes
- **THEN** exits 2, reports each finding under its own package and runs the third

#### Scenario: Packages that share their last path element
- **WHEN** two packages named util are run together
- **THEN** both run and pass

### Requirement: How test binaries are linked
A run SHALL link its test binaries without the symbol table and debug information, as go test does, unless -ldflags is given as an argument or in GOFLAGS, a -cpuprofile, -memprofile, -blockprofile or -mutexprofile flag is given, or --debug keeps the binaries.

#### Scenario: A plain run
- **WHEN** a package is run with no link or profile flag
- **THEN** its test binary carries no debug information

#### Scenario: A run that writes a CPU profile
- **WHEN** the package is run with -cpuprofile
- **THEN** its test binary carries debug information

#### Scenario: Link flags in GOFLAGS
- **WHEN** GOFLAGS sets -ldflags=-X for a variable the test prints
- **THEN** the test prints the value and the binary carries debug information

### Requirement: A run never executes a stale binary
A run SHALL execute a test binary built from the sources, build tags and link flags it was given, whatever an earlier run left in the cache.

#### Scenario: A dependency changes and changes back
- **WHEN** a dependency's source is changed, run, and restored
- **THEN** each run prints the value its sources hold

#### Scenario: A test file changes and changes back
- **WHEN** a test file is changed, run, and restored
- **THEN** each run prints the value its sources hold

#### Scenario: Build tags as an argument and in GOFLAGS
- **WHEN** a run is given a build tag by -tags, then by GOFLAGS, then none
- **THEN** each run executes the binary built for its tags

#### Scenario: Link flags as an argument and in GOFLAGS
- **WHEN** a run is given -ldflags=-X as an argument, then in GOFLAGS, then none
- **THEN** each run prints the value it was given

#### Scenario: Vet after a run without it
- **WHEN** a package with a vet finding is run with -vet=off and then without
- **THEN** the second run reports the finding

#### Scenario: Runs at once over one cache
- **WHEN** three runs of one package start at once on an empty cache
- **THEN** all pass and leave no temporary file in the cache

#### Scenario: A filtered run before a full one
- **WHEN** a run that -run narrows to one package is followed by an unfiltered run
- **THEN** the second run builds and runs the other package

### Requirement: Clearing the test binary cache
gotest clean --cache SHALL remove every cached binary no run is linking or copying, with its lock and record, and say how many.

#### Scenario: Clearing the cache
- **WHEN** gotest clean --cache runs
- **THEN** removes every cached binary no run is using and says how many
- **AND** refuses a package beside the flag with exit 2

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
- **AND** ends with the run booked as interrupted

#### Scenario: A killed run
- **WHEN** a -json run with --events-file is killed while a suite holds
- **THEN** keeps the events of the suites that ended before the kill, each line whole

### Requirement: A suite cut short by --timeout
A suite process an expired --timeout stopped SHALL fail its package on the text, -json and spec pipelines, whatever its tests reported: a method that returned when its context ended did not finish. The FAIL line of the run SHALL name the suite when no method was still running. An interrupt SHALL keep the tests' own verdicts, as the lifecycle spec states. A method running when the stop came SHALL be booked failed in the stream with the line that names the deadline.

#### Scenario: A method that returns when its context ends
- **WHEN** --timeout expires while a method waits for its context to end, and the method then returns
- **THEN** fails the package and the run with exit 1, on the text and -json pipelines
- **AND** names the suite in the run's FAIL line, on stderr and in the stream
