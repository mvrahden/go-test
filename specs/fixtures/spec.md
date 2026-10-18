# Fixtures Specification

## Purpose
How package and shared fixtures are set up, handed to the suites that bind them and released, and how a fixture that goes wrong fails the run.

## Requirements

### Requirement: A package fixture hook that goes wrong
A package fixture hook that returns an error, panics or overruns the Timeout SHALL fail the run; a failed BeforeAll SHALL still get its AfterAll, a failed BeforeEach SHALL register no AfterEach, and a panic SHALL end the suite after the method's own teardown and AfterAll. The cases are the table in `tests/conformance/fixtures/hooks_suite_test.go`.

#### Scenario: Each way a package fixture hook goes wrong
- **WHEN** a package fixture hook returns an error, panics or overruns the Timeout
- **THEN** fails the run, runs the later hooks its row names and gives the verdicts its row names

### Requirement: Where a fixture hook failure is reported
A fixture hook's failure SHALL be reported at the hook's own source, not inside the runtime.

#### Scenario: A fixture BeforeEach error
- **WHEN** a fixture BeforeEach returns an error
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
A shared fixture whose setup fails on its last attempt SHALL fail the run and SHALL still run its AfterAll.

#### Scenario: The last setup attempt fails
- **WHEN** a shared fixture's setup fails on its last attempt
- **THEN** fails the run and still runs the fixture's AfterAll, releasing what the attempt reached

### Requirement: Shared fixtures need the gotest runner
A suite binding a shared fixture SHALL fail under plain go test before any method runs, naming the fixture and go tool gotest.

#### Scenario: A shared-fixture suite under plain go test
- **WHEN** a suite binding a shared fixture runs under plain go test
- **THEN** fails naming the shared fixture it binds and go tool gotest, before any method runs

### Requirement: A shared fixture's declared setup budget
When no --setup-timeout is given, the wait for a shared fixture's setup SHALL be the budget the fixture declares.

#### Scenario: A container fixture slower than the default budget
- **WHEN** a shared fixture declaring ContainerFixtureConfig takes 130s to come up and no --setup-timeout is given
- **THEN** the run passes: the wait is the fixture's declared budget

### Requirement: Shared fixture state reaches its suites
The state a shared fixture's setup produces SHALL reach the suites that bind it whatever the setup writes to stdout or its log and however large the state is, and nothing the setup writes SHALL become an event of the run.

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

#### Scenario: Every such fixture came up
- **WHEN** every such fixture came up
- **THEN** the run passes
