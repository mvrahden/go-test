# Fixtures Specification

## Purpose
How package and shared fixtures are set up, handed to the suites that bind them and released, and how a fixture that goes wrong fails the run.

## Requirements

### Requirement: A package fixture hook that goes wrong
A package fixture hook that returns an error, panics or overruns the Timeout SHALL fail the run; a failed BeforeAll SHALL still get its AfterAll, a BeforeEach that returns an error SHALL register no AfterEach while one that overruns and returns SHALL be released by its AfterEach, and a panic SHALL end the suite after the method's own teardown and AfterAll. The cases are the table in `tests/conformance/fixtures/hooks_suite_test.go`.

#### Scenario: Each way a package fixture hook goes wrong
- **WHEN** a package fixture hook returns an error, panics or overruns the Timeout
- **THEN** fails the run, runs the later hooks its row names and gives the verdicts its row names

### Requirement: Where a fixture hook failure is reported
An error a fixture BeforeEach or AfterEach returns SHALL be reported at the hook's own source, not inside the runtime.

#### Scenario: A fixture BeforeEach error
- **WHEN** a fixture BeforeEach returns an error
- **THEN** is reported in the hook's own file, not the runtime's

#### Scenario: A fixture AfterEach error
- **WHEN** a fixture AfterEach returns an error
- **THEN** is reported in the hook's own file, not the runtime's

### Requirement: Selection decides which shared fixtures start
A shared fixture SHALL start only when a selected suite binds it, and a suite with a skipped method SHALL still get its fixture for the methods that run.

#### Scenario: Skipping the only suite that binds a shared fixture
- **WHEN** -skip names the only suite binding a shared fixture
- **THEN** the fixture is never started and the other suite passes

#### Scenario: Skipping one method of a suite that binds a shared fixture
- **WHEN** -skip names one method of the suite binding a shared fixture
- **THEN** the fixture starts and the suite's other method passes against it

### Requirement: A guard does not hold back a shared fixture
A shared fixture bound by a guard-skipped suite SHALL still be started, and its setup failure SHALL fail the run and that suite without the guard being evaluated.

#### Scenario: A guarded suite whose shared fixture fails
- **WHEN** a guard-skipped suite binds a shared fixture whose setup fails
- **THEN** the fixture is started for it and its failure fails the run and the suite, whose guard never runs

### Requirement: A failed shared setup is released
A shared fixture whose setup fails on its last attempt SHALL fail the run and SHALL run its AfterAll after every attempt, before the next one starts.

#### Scenario: The last setup attempt fails
- **WHEN** a shared fixture's setup fails on its last attempt
- **THEN** fails the run
- **AND** runs AfterAll after each of its 2 attempts, releasing what each reached before the next

### Requirement: A failed release between attempts
A fixture whose AfterAll fails between two setup attempts SHALL fail the run and report that AfterAll's error, even when a later attempt comes up.

#### Scenario: A package fixture's release fails
- **WHEN** a package fixture's AfterAll fails between its attempts and the second attempt comes up
- **THEN** fails the run
- **AND** reports that AfterAll's error

#### Scenario: A shared fixture's release fails
- **WHEN** a shared fixture's AfterAll fails between its attempts and the second attempt comes up
- **THEN** fails the run
- **AND** reports that AfterAll's error

### Requirement: A setup that ends before BeforeAll
A shared fixture whose setup ends before its BeforeAll runs SHALL get no AfterAll, while a dependency whose BeforeAll ran SHALL get its AfterAll.

#### Scenario: A shared fixture's setup ends first
- **WHEN** a run is interrupted while a shared fixture's dependency sets up, before the fixture's own BeforeAll runs
- **THEN** runs no AfterAll for the fixture, and the dependency's AfterAll

### Requirement: Shared fixtures need the gotest runner
A suite binding a shared fixture SHALL fail under plain go test before any method runs, naming the fixture and go tool gotest.

#### Scenario: A shared-fixture suite under plain go test
- **WHEN** a suite binding a shared fixture runs under plain go test
- **THEN** fails naming the shared fixture it binds and go tool gotest, before any method runs

### Requirement: A shared fixture's declared setup budget
When no --setup-timeout is given, the wait for a shared fixture's setup SHALL be the budget the fixture declares, counting every attempt and every release between them, for a setup at the start of the run and for one started later for the exclusive tail or a bench slot; a fixture that hangs past that budget SHALL fail the run and the suites that read it SHALL never run.

#### Scenario: A container fixture slower than the default budget
- **WHEN** a shared fixture declaring ContainerFixtureConfig takes 130s to come up and no --setup-timeout is given
- **THEN** the run passes: the wait is the fixture's declared budget

#### Scenario: A fixture that hangs past its budget
- **WHEN** a shared fixture declaring a 2s Timeout hangs in BeforeAll and no --setup-timeout is given
- **THEN** fails the run within 60s, saying the setup timed out
- **AND** never runs the suite reading the fixture

#### Scenario: Releases between attempts
- **WHEN** a shared fixture with a 2s Timeout and 1 retry spends 1.6s on each attempt and on the release between them, and no --setup-timeout is given
- **THEN** the run passes: the wait counts the release

#### Scenario: A fixture started for the exclusive tail
- **WHEN** a shared fixture started for the exclusive tail hangs past its declared 2s Timeout and no --setup-timeout is given
- **THEN** the wait ends at the declared budget and the run fails
- **AND** never runs the suite reading it

#### Scenario: A fixture started for a bench slot
- **WHEN** a shared fixture started for a bench slot hangs past its declared 2s Timeout and no --setup-timeout is given
- **THEN** the wait ends at the declared budget and the run fails
- **AND** never runs the benchmark reading it

### Requirement: A late fixture's dependencies
A shared fixture started for the exclusive tail together with a dependency only the tail needs SHALL come up after that dependency, serve the suites that read it, and be torn down before it.

#### Scenario: A tail fixture with a tail-only dependency
- **WHEN** a shared fixture started for the exclusive tail depends on a shared fixture only the tail needs
- **THEN** both come up, the dependency first, and the suite reading them passes
- **AND** both are torn down, the dependent first

### Requirement: A late fixture's teardown budget
A shared fixture started for the exclusive tail or a bench slot SHALL be released under the teardown budget its declared Timeout gives, that Timeout and 30s more, not the default's; an AfterAll that never returns SHALL be given up once that budget has passed and SHALL fail the run.

#### Scenario: A tail fixture whose AfterAll never returns
- **WHEN** a shared fixture started for the exclusive tail declares a 2s Timeout and its AfterAll never returns
- **THEN** its process is force-killed once its 32s teardown budget has passed, and the run fails

#### Scenario: A bench slot's fixture whose AfterAll never returns
- **WHEN** a shared fixture started for a bench slot declares a 2s Timeout and its AfterAll never returns
- **THEN** the slot's release is given up once its 32s teardown budget has passed, and the run fails

### Requirement: A stopped run ends the wait for a late fixture
A run whose --timeout expires while a shared fixture started for the exclusive tail hangs SHALL end the wait, fail, and leave no fixture process behind.

#### Scenario: The run's --timeout expires during the wait
- **WHEN** the run's --timeout expires while a shared fixture started for the exclusive tail hangs with no Timeout
- **THEN** the wait ends with the run and the run fails
- **AND** leaves no fixture process behind

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
When the run gives up on a shared fixture's setup, a sibling that came up SHALL be released under the teardown budget the shared fixture process reports once its setups return.

#### Scenario: A sibling still to be released
- **WHEN** a shared fixture's setup is given up while a sibling that came up is still to be released
- **THEN** fails the run
- **AND** the sibling's 8s AfterAll runs to its end under the teardown budget the process reports

### Requirement: A shared fixture process whose CLI is killed
When the CLI is killed, its shared fixture process SHALL end the setup's context, run the AfterAll of every fixture whose BeforeAll ran, and exit, on Unix; on Linux, a process whose hook never returns SHALL exit with status 1 once it has had its setup budget and the teardown budget it reported.

#### Scenario: Killed while a fixture is up
- **WHEN** the CLI is killed while a shared fixture is up
- **THEN** the fixture's AfterAll runs to its end, then its process exits

#### Scenario: Killed while a setup waits on its context
- **WHEN** the CLI is killed while a shared fixture's BeforeAll waits on its context
- **THEN** the setup's context ends, the fixture's AfterAll runs, then its process exits

#### Scenario: An AfterAll that never returns
- **WHEN** the CLI is killed and the shared fixture's AfterAll never returns
- **THEN** the fixture process exits with status 1 once the 31s teardown budget it reported has passed

#### Scenario: A BeforeAll that ignores its context
- **WHEN** the CLI is killed while a shared fixture's BeforeAll ignores its context
- **THEN** the fixture process exits with status 1 once its 1s setup budget and 31s teardown budget have passed
