# Lifecycle Specification

## Purpose
How a suite's hooks, methods, goroutines and fixtures run, end and release, on every exit path: a failure, a panic, an overrun, a stop from outside.

## Requirements

### Requirement: A hook or body going wrong
A suite hook or body that fails an assertion, panics or overruns the Timeout SHALL fail the run and the method or suite it belongs to, SHALL still run the later hooks that release what ran, and after a panic in a method's hooks or body SHALL start no further method. The cases are the table in `tests/conformance/lifecycle/hooks_suite_test.go`.

#### Scenario: Each hook and each way it goes wrong
- **WHEN** a suite hook or the body fails an assertion, panics or overruns the Timeout
- **THEN** fails the run with exit 1 and the method or suite whose hook went wrong, running the later hooks the row lists and, after a panic in a method's hooks or body, no further method

### Requirement: A panic among parallel methods
A panic in one parallel method SHALL fail the run, cut its running siblings short and still run AfterAll and the fixture teardown.

#### Scenario: A parallel method panics
- **WHEN** a parallel method panics while a sibling is still running
- **THEN** AfterAll and the fixture teardown run, the sibling is cut short and its AfterEach never runs, and the run fails

### Requirement: A benchmark that panics
A benchmark that panics SHALL fail the run and print the panic with its stack, SHALL report no result for itself while the benchmarks after it still run, and SHALL run AfterAll before the fixture is released.

#### Scenario: A benchmark panics
- **WHEN** a benchmark panics
- **THEN** fails the run
- **AND** prints the panic with its stack on stderr
- **AND** reports no result for it and a result for the benchmark after it
- **AND** runs AfterAll before the fixture is released

### Requirement: FailFast
A FailFast suite SHALL stop at the first failing method, failing it and the run and skipping every method not yet started with that method as the reason; a failure BeforeAll records without stopping SHALL NOT stop it.

#### Scenario: The first method fails
- **WHEN** the first method of a FailFast suite fails
- **THEN** fails the method
- **AND** fails the run
- **AND** skips every method that has not started, with the failing method as the reason

#### Scenario: BeforeAll records a failure and goes on
- **WHEN** BeforeAll of a FailFast suite records a failure without stopping and no method fails
- **THEN** runs every method

### Requirement: Each rows
Every row of an Each table SHALL be its own subtest with its own verdict; a row that fails or skips SHALL end the method there, and a loop that breaks SHALL leave the method running.

#### Scenario: A row fails
- **WHEN** one row of an Each table fails
- **THEN** fails that row and the method, and the rows after it never run

#### Scenario: A row skips
- **WHEN** one row of an Each table skips
- **THEN** skips that row and the method from there, and the rows after it never run

#### Scenario: The loop breaks
- **WHEN** the loop over an Each table breaks
- **THEN** stops at the break and the method goes on

### Requirement: Goroutines started by gotest.Go
The wait on a goroutine started by gotest.Go SHALL come after AfterAll when the goroutine is started in BeforeAll, and a fatal assertion on that goroutine SHALL fail the method at the wait.

#### Scenario: A goroutine only AfterAll stops
- **WHEN** BeforeAll starts a goroutine with gotest.Go that only AfterAll stops
- **THEN** the suite passes: the wait comes after AfterAll

#### Scenario: A goroutine fails a fatal assertion
- **WHEN** a goroutine started by gotest.Go fails a fatal assertion
- **THEN** the wait on it fails the method there, and nothing after the wait runs

### Requirement: Async methods
An async method SHALL fail at once, not at the end of its budget, when the goroutine that owns done fails a fatal assertion.

#### Scenario: The owner of done fails
- **WHEN** the goroutine that owns done fails a fatal assertion
- **THEN** fails the method at once, not at the end of its budget

### Requirement: Stopping a run
A run interrupted while a method holds SHALL exit 130, end the holding method's context, skip the methods that have not started with the reason run stopping, and run AfterAll before the fixture is released.

#### Scenario: Interrupted while a method holds
- **WHEN** the run is interrupted while a method holds
- **THEN** exits 130
- **AND** ends the holding method's context
- **AND** skips the methods that have not started with the reason run stopping
- **AND** runs AfterAll before the fixture is released

### Requirement: Every launcher stops cleanly
A run stopped by SIGINT, SIGTERM or a second SIGINT SHALL end with its launcher's exit code, SHALL finish the fixture teardown unless a second signal abandons it and says so, and SHALL leave no suite process behind, whether gotest runs directly, through go run or through go tool. The cases are the table in `tests/conformance/lifecycle/launcher_unix_test.go`.

#### Scenario: Each launcher and each signal
- **WHEN** a run is stopped by a signal, however gotest was launched
- **THEN** stops with its launcher's exit code, finishes the fixture teardown unless a second signal abandons it saying so, and leaves no suite process behind

### Requirement: No process outlives the run
A run SHALL leave no process a test started behind, even one the test never waits for.

#### Scenario: A process nobody waits for
- **WHEN** a test starts sleep 4242 and never waits for it
- **THEN** the test passes and no such process outlives the run
