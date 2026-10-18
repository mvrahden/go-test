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
The wait on a goroutine started by gotest.Go SHALL come after AfterAll when the goroutine is started in BeforeAll, on the hook's T or on one NewTWithDeadline derives from it, SHALL cover every goroutine BeforeAll started before the fixture is released even when the last one fails or panics, and a fatal assertion on that goroutine SHALL fail the method at the wait. The cases of the last goroutine are the table in `tests/conformance/lifecycle/goroutines_suite_test.go`.

#### Scenario: A goroutine only AfterAll stops
- **WHEN** BeforeAll starts a goroutine with gotest.Go that only AfterAll stops
- **THEN** the suite passes: the wait comes after AfterAll

#### Scenario: A goroutine on a derived T only AfterAll stops
- **WHEN** BeforeAll starts a goroutine with gotest.Go on a T from NewTWithDeadline that only AfterAll stops
- **THEN** the suite passes: the wait comes after AfterAll

#### Scenario: The last of several goroutines goes wrong
- **WHEN** the last of several goroutines BeforeAll started with gotest.Go fails or panics
- **THEN** fails the suite and waits for every other one before the fixture is released

#### Scenario: A goroutine fails a fatal assertion
- **WHEN** a goroutine started by gotest.Go fails a fatal assertion
- **THEN** the wait on it fails the method there, and nothing after the wait runs

### Requirement: Async methods
An async method SHALL wait for done until done is called, the goroutine that owns done fails, the run is stopped or the declared Timeout passes; a fatal failure of another of its goroutines SHALL fail the method without ending the wait, a failure that leaves none of the method's goroutines SHALL end the wait at once, and its AfterEach SHALL be held to the declared Timeout. The cases of another goroutine failing are the table in `tests/conformance/lifecycle/async_suite_test.go`.

#### Scenario: The owner of done fails
- **WHEN** the goroutine that owns done fails a fatal assertion and no other goroutine of the method is left
- **THEN** fails the method at once, not at the end of its budget

#### Scenario: Another goroutine fails
- **WHEN** a goroutine of an async method other than the one that owns done fails a fatal assertion
- **THEN** fails the method, and the wait ends only when the owner calls done, fails or outlasts the Timeout

#### Scenario: A stop during the wait
- **WHEN** the run is stopped while an async method waits for done after another of its goroutines failed
- **THEN** ends the wait at the stop, then runs AfterEach and AfterAll, and fails the method

#### Scenario: AfterEach overruns
- **WHEN** an async method calls done in time and its AfterEach overruns the declared Timeout
- **THEN** fails the method for the overrun

### Requirement: The grace a stopped method gets
When a run is stopped, a method body that has not returned 5s after its context ended SHALL be cut short with a message saying so, its fixture released and its AfterAll not run; the 5s SHALL bound only the body, so an AfterEach that takes longer SHALL run to its end, followed by AfterAll and the fixture release.

#### Scenario: A body that ignores its context
- **WHEN** the run is stopped and a method body ignores its context
- **THEN** cuts the body short after 5s, saying so
- **AND** releases the fixture without running AfterAll

#### Scenario: A slow AfterEach
- **WHEN** the run is stopped and a method's AfterEach outlasts the 5s grace its body gets
- **THEN** runs AfterEach to its end, then AfterAll, then releases the fixture: the grace bounds only the body

### Requirement: Stopping a run
A run interrupted while a method holds SHALL exit 130 when gotest runs directly or through go tool, end the holding method's context, skip the methods that have not started with the reason run stopping, and run AfterAll before the fixture is released.

#### Scenario: Interrupted while a method holds
- **WHEN** the run is interrupted while a method holds
- **THEN** exits 130 when gotest runs directly
- **AND** ends the holding method's context
- **AND** skips the methods that have not started with the reason run stopping
- **AND** runs AfterAll before the fixture is released

### Requirement: Every launcher stops cleanly
A run stopped by SIGINT, SIGTERM or a second SIGINT SHALL end with its exit code, or by the signal where go run dies of a SIGTERM, SHALL finish the fixture teardown unless a second signal abandons it and says so, and SHALL leave no suite process behind, whether gotest runs directly, through go run or through go tool. The cases are the table in `tests/conformance/lifecycle/launcher_unix_test.go`.

#### Scenario: Each launcher and each signal
- **WHEN** a run is stopped by a signal, however gotest was launched
- **THEN** ends with its exit code, or by the signal where go run dies of a SIGTERM, finishes the fixture teardown unless a second signal abandons it saying so, and leaves no suite process behind

### Requirement: No process outlives the run
A run SHALL leave no process a test started behind, even one the test never waits for.

#### Scenario: A process nobody waits for
- **WHEN** a test starts sleep 4242 and never waits for it
- **THEN** the test passes and no such process outlives the run

### Requirement: A stop by the stop file alone
On Windows, where the runner stops a suite by its stop file alone, an interrupted run SHALL skip the methods that have not started with the reason run stopping, SHALL carry every verdict to the stream and SHALL run AfterAll before the fixture is released.

#### Scenario: Interrupted on Windows
- **WHEN** a run is interrupted on Windows, where only the stop file reaches the suite
- **THEN** skips the methods that have not started with the reason run stopping
- **AND** every verdict reaches the stream: the holding method's, the suite's and the package's
- **AND** runs AfterAll before the fixture is released

### Requirement: A suite whose parent is gone
A suite process under the runner whose parent ends SHALL stop by itself: it SHALL end the holding method's context, run none of the methods that have not started, run AfterAll before the fixture is released, and exit.

#### Scenario: The parent is killed
- **WHEN** the parent of a suite process under the runner is killed while a method holds
- **THEN** ends the holding method's context
- **AND** the methods that have not started never run
- **AND** runs AfterAll before the fixture is released
- **AND** leaves no suite process behind

### Requirement: A suite whose stream nobody reads
A suite process under the runner whose stdout nobody reads any more, because the runner and its converter died while an -exec wrapper keeps the suite running, SHALL stop by itself the same way; outside the runner, a stdout nobody reads SHALL NOT stop a suite.

#### Scenario: The runner and its converter are killed
- **WHEN** the runner and its converter are killed under -json while an -exec wrapper waits on the suite
- **THEN** ends the holding method's context
- **AND** the methods that have not started never run
- **AND** runs AfterAll before the fixture is released
- **AND** leaves no suite process behind

#### Scenario: Outside the runner
- **WHEN** a suite run by its test binary outside the gotest runner loses the reader of its stdout
- **THEN** the holding method runs on: its context does not end

### Requirement: What an expired --timeout names
An expired --timeout SHALL name as running only the tests whose process it cut short, not a test whose suite process ended on its own before it.

#### Scenario: A crash before the timeout
- **WHEN** one suite's process crashed on its own and another suite hangs until --timeout
- **THEN** names only the hung test as running when the timeout expired

### Requirement: A stop signals only the suite's own processes
A cancelled run SHALL send no signal to a pid the suite published that is outside the suite's process group, and SHALL interrupt the suite's process group instead.

#### Scenario: A published pid outside the group
- **WHEN** the run is interrupted and the pid the suite published names a process outside its group
- **THEN** sends that process no signal
- **AND** interrupts the suite's process group instead
- **AND** runs AfterAll before the fixture is released
