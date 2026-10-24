# Fixtures Specification

## Purpose
Which shared fixtures a run starts for the suites it selects, guarded suites included, how a package fixture's per-method hooks run and where their failures are reported, how a fixture whose setup or Hydrate went wrong is released, how long the runner waits for shared fixture setup and that a setup it gives up fails the run, and that shared fixtures need the gotest runner.

## Requirements

### Requirement: Selection decides which shared fixtures start
A shared fixture SHALL start only when a selected suite binds it, and a suite with a skipped method SHALL still get its fixture for the methods that run.

#### Scenario: Skipping the only suite that binds a shared fixture
- **WHEN** -skip names the only suite binding a shared fixture
- **THEN** the fixture is never started and the other suite passes

#### Scenario: Skipping one method of a suite that binds a shared fixture
- **WHEN** -skip names one method of the suite binding a shared fixture
- **THEN** the fixture starts and the suite's other method passes against it

### Requirement: Selection follows go test's matching
A suite SHALL be selected when go test would enter one of its top-level functions: -run enters a function the first level of an alternative matches, -skip keeps go test out of it only when the first alternative matching it has no further level, and a repeated -run or -skip counts by its last value. A bench run SHALL select a benchmark suite by its Benchmark function the same way.

#### Scenario: A repeated -skip
- **WHEN** -skip is given twice, naming the fixture-free suite last
- **THEN** the last one counts: the fixture starts, its suite passes and the other suite does not run

#### Scenario: A -skip whose first matching alternative names a method
- **WHEN** -skip's first alternative matching a suite names one of its methods and a later one names the suite
- **THEN** go test still enters the suite, so its fixture starts and the suite passes

#### Scenario: A -run whose alternatives name two suites
- **WHEN** -run's first alternative names a method of one suite and a later one names the other suite
- **THEN** both suites run, the one binding the fixture against it

#### Scenario: Skipping the only benchmark suite that binds a shared fixture
- **WHEN** -skip names the only benchmark suite binding a shared fixture
- **THEN** a bench run never starts the fixture and benchmarks the other suite

### Requirement: A guard does not hold back a shared fixture
A shared fixture bound by a guard-skipped suite SHALL still be started, and its setup failure SHALL fail the run and that suite without the guard being evaluated.

#### Scenario: A guarded suite whose shared fixture fails
- **WHEN** a guard-skipped suite binds a shared fixture whose setup fails
- **THEN** the fixture is started for it and its failure fails the run and the suite, whose guard never runs

### Requirement: A package fixture's BeforeAll or AfterAll going wrong
A package fixture whose BeforeAll returns an error or panics SHALL fail the run and its suite, which never opens, and SHALL still get its AfterAll; one whose AfterAll returns an error or panics SHALL fail the run after every other hook ran. The cases are the table in `tests/conformance/fixtures/hooks_suite_test.go`.

#### Scenario: Each way a package fixture's BeforeAll or AfterAll goes wrong
- **WHEN** a package fixture's BeforeAll or AfterAll returns an error or panics
- **THEN** fails the run with exit 1, runs the hooks its row names and gives the verdicts its row names

### Requirement: Where a fixture hook failure is reported
An error a fixture BeforeEach or AfterEach returns SHALL be reported at the hook's declaration, not inside the runtime, wherever it is declared: in the suite's package, in another package, or promoted from another package's type. The file is named by its base name, or by its full path under -fullpath.

#### Scenario: A fixture BeforeEach error
- **WHEN** a fixture BeforeEach returns an error
- **THEN** is reported at the hook's declaration, not in the runtime's file

#### Scenario: A fixture AfterEach error
- **WHEN** a fixture AfterEach returns an error
- **THEN** is reported at the hook's declaration, not in the runtime's file

#### Scenario: A hook declared in another package
- **WHEN** a hook another package declares returns an error, a fixture's of that package or one promoted from its type
- **THEN** is reported at the hook's declaration, named by its file's base name

#### Scenario: A hook declared in another package, under -fullpath
- **WHEN** a hook another package declares returns an error under -fullpath
- **THEN** names the hook's file by its full path, as go test -fullpath does

### Requirement: Fixture per-method hooks in dependency order
A suite's package fixtures SHALL take their per-method turns in dependency order, a fixture's turn coming only once every BeforeEach before it returned without an error inside the Timeout. In its turn a fixture runs its BeforeEach, if it has one; its AfterEach SHALL run, in reverse, exactly when its turn came and that BeforeEach returned without an error.

#### Scenario: A BeforeEach overruns ahead of another fixture's
- **WHEN** a fixture's BeforeEach overruns the Timeout ahead of another fixture's
- **THEN** starts no later fixture's BeforeEach, fails the method once, at the hook that overran, and runs that fixture's AfterEach

#### Scenario: An AfterEach-only fixture before the one that fails
- **WHEN** a fixture with only an AfterEach comes before a fixture whose BeforeEach fails
- **THEN** runs its AfterEach, as every BeforeEach before it returned

#### Scenario: An AfterEach-only fixture after the one that fails
- **WHEN** a fixture with only an AfterEach comes after a fixture whose BeforeEach fails
- **THEN** does not run its AfterEach, as its turn never came

### Requirement: A failed shared setup is released
A shared fixture whose setup fails on its last attempt SHALL fail the run and SHALL run its AfterAll after every attempt, before the next one starts.

#### Scenario: The last setup attempt fails
- **WHEN** a shared fixture's setup fails on its last attempt
- **THEN** fails the run
- **AND** runs AfterAll after each of its 2 attempts, releasing what each reached before the next

### Requirement: A failed release between attempts
A fixture whose AfterAll fails between two setup attempts, or outlives there the Timeout the fixture declares, SHALL fail the run and report why, even when a later attempt comes up.

#### Scenario: A package fixture's release fails
- **WHEN** a package fixture's AfterAll fails between its attempts and the second attempt comes up
- **THEN** fails the run
- **AND** reports that AfterAll's error

#### Scenario: A shared fixture's release fails
- **WHEN** a shared fixture's AfterAll fails between its attempts and the second attempt comes up
- **THEN** fails the run
- **AND** reports that AfterAll's error

#### Scenario: A package fixture's release outlives its Timeout
- **WHEN** a package fixture's AfterAll between its attempts outlives the fixture's declared Timeout and the second attempt comes up
- **THEN** fails the run
- **AND** reports that the release exceeded the Timeout

### Requirement: A stop during the release between attempts
A fixture whose AfterAll runs between two setup attempts when the run is stopped SHALL finish that AfterAll under a context the stop does not cancel, bounded by the fixture's Timeout, and SHALL then start no further attempt and run no second AfterAll.

#### Scenario: A stop while a shared fixture releases a failed attempt
- **WHEN** a run is stopped while a shared fixture's AfterAll releases a failed attempt before its retry
- **THEN** lets that AfterAll finish under a context the stop does not cut short
- **AND** starts no further attempt and releases the fixture once

### Requirement: A setup that ends before BeforeAll
A shared fixture whose setup ends before its BeforeAll runs SHALL get no AfterAll, while a dependency whose BeforeAll ran SHALL get its AfterAll.

#### Scenario: A shared fixture's setup ends first
- **WHEN** a run is interrupted while a shared fixture's dependency sets up, before the fixture's own BeforeAll runs
- **THEN** runs no AfterAll for the fixture, and the dependency's AfterAll

### Requirement: A failed Hydrate is dehydrated
A shared fixture whose Hydrate fails or panics in a suite's process SHALL fail the run before any of the suite's methods runs, and SHALL still get its Dehydrate there.

#### Scenario: Hydrate fails partway
- **WHEN** a shared fixture's Hydrate fails after reaching part of what it opens
- **THEN** fails the run, runs no method and still runs its Dehydrate

### Requirement: Shared fixtures need the gotest runner
A suite binding a shared fixture SHALL fail under plain go test before any method runs, naming the fixture and go tool gotest.

#### Scenario: A shared-fixture suite under plain go test
- **WHEN** a suite binding a shared fixture runs under plain go test
- **THEN** fails naming the shared fixture it binds and go tool gotest, before any method runs

### Requirement: A shared fixture's declared setup budget
When no --setup-timeout is given, the wait for a shared fixture's setup SHALL be the budget the fixture declares, counting every attempt and every release between them; a fixture that hangs past that budget SHALL fail the run, saying the setup timed out.

#### Scenario: A container fixture slower than the default budget
- **WHEN** a shared fixture declaring ContainerFixtureConfig takes 35s to come up and no --setup-timeout is given
- **THEN** the run passes: the wait is the fixture's declared budget

#### Scenario: A fixture that hangs past its budget
- **WHEN** a shared fixture declaring a 2s Timeout hangs in BeforeAll and no --setup-timeout is given
- **THEN** fails the run within 60s, saying the setup timed out

### Requirement: Shared fixture state reaches its suites
The state a shared fixture's setup produces SHALL reach the suites that bind it when the setup prints to stdout, with or without a trailing newline and in a JSON-shaped line, and when the state is 2 MiB, and nothing the setup prints SHALL become an event of the run.

#### Scenario: Setup output without a trailing newline
- **WHEN** a shared fixture's setup prints to stdout without a newline
- **THEN** its suite receives the state

#### Scenario: Setup logs a JSON-shaped line
- **WHEN** a shared fixture's setup logs a JSON-shaped line
- **THEN** its suite receives the state
- **AND** the line is not an event of the run

#### Scenario: Large state
- **WHEN** a shared fixture carries 2 MiB of state
- **THEN** its suite receives the state

#### Scenario: Every protocol fixture comes up
- **WHEN** every protocol fixture comes up despite its output
- **THEN** the run passes

### Requirement: A process a shared fixture's setup starts
A process a shared fixture's setup starts SHALL get neither the protocol descriptor nor the variable naming it, and the run SHALL NOT wait for it, on Unix.

#### Scenario: A process that outlives the setup
- **WHEN** a shared fixture's setup starts a process that outlives it
- **THEN** the run passes before that process ends
- **AND** the process sees no protocol variable, and what it writes to descriptor 3 is not read

### Requirement: Siblings of a setup that is given up
When the run gives up on a shared fixture's setup while a sibling that came up is still to be released, the run SHALL fail.

#### Scenario: A sibling still to be released
- **WHEN** a shared fixture's setup is given up while a sibling that came up is still to be released
- **THEN** fails the run
