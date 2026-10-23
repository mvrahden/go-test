# Lifecycle Specification

## Purpose
How a suite's tests end, the hooks and the body going wrong, FailFast, panics among parallel methods and in benchmarks, the rows of an Each table, the goroutines gotest.Go starts and a panic ahead of AfterEach included, and what the run reports about them when they do.

## Requirements

### Requirement: A hook or body going wrong
A suite hook, a fixture's per-method hook or the body that fails, panics or overruns the Timeout SHALL fail the run and its method or suite, locating an overrun or a panic at that hook, and SHALL still run the later hooks that release what ran; after an overrun only AfterEach hooks start, after a panic in a method no further method. An async method's budget ends when done() is called; its AfterEach is not budgeted. The cases are the tables in `tests/conformance/lifecycle/hooks_suite_test.go`.

#### Scenario: Each hook and each way it goes wrong
- **WHEN** a suite hook or the body fails an assertion, panics or overruns the Timeout
- **THEN** fails the run with exit 1 and the method or suite whose hook went wrong, located at that hook when it panics or overruns, running the later hooks the row lists and, after a panic in a method's hooks or body, no further method

#### Scenario: Each fixture per-method hook and each way it goes wrong
- **WHEN** a fixture's BeforeEach or AfterEach returns an error, panics or overruns the Timeout
- **THEN** fails the run with exit 1 and the method, naming the hook, located at it when it panics or overruns, running the later hooks the row lists and, after a panic, no further method

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
A FailFast suite SHALL stop at the first failing method, failing it and the run and skipping every method not yet started with that method as the reason, a failure recorded in the method's cleanups included; a failure BeforeAll records without stopping SHALL NOT stop it.

#### Scenario: The first method fails
- **WHEN** the first method of a FailFast suite fails
- **THEN** fails the method
- **AND** fails the run
- **AND** skips every method that has not started, with the failing method as the reason

#### Scenario: A parallel method fails in a cleanup
- **WHEN** a parallel method of a FailFast suite fails only in a cleanup, the methods running one at a time
- **THEN** skips the method that has not started, with the failing method as the reason

#### Scenario: BeforeAll records a failure and goes on
- **WHEN** BeforeAll of a FailFast suite records a failure without stopping and no method fails
- **THEN** runs every method

### Requirement: Each rows
Every row of an Each table SHALL be its own subtest with its own verdict; a row that fails or skips SHALL end the method there, a row that ends the method's own T SHALL end the method with that verdict alone, and a loop that breaks SHALL leave the method running.

#### Scenario: A row fails
- **WHEN** one row of an Each table fails
- **THEN** fails that row and the method, and the rows after it never run

#### Scenario: A row skips
- **WHEN** one row of an Each table skips
- **THEN** skips that row and the method from there, and the rows after it never run

#### Scenario: A row ends the method's own T
- **WHEN** a row of an Each table skips or fatally fails the method's own T
- **THEN** the method skips or fails there, with no failure of its own

#### Scenario: The loop breaks
- **WHEN** the loop over an Each table breaks
- **THEN** stops at the break and the method goes on

### Requirement: Goroutines started by gotest.Go
A fatal assertion on a goroutine started by gotest.Go SHALL fail the method at the wait, a skip there SHALL skip the method at the wait, and a goroutine that leaves through runtime.Goexit with neither SHALL fail the method with a message that says so.

#### Scenario: A goroutine fails a fatal assertion
- **WHEN** a goroutine started by gotest.Go fails a fatal assertion
- **THEN** the wait on it fails the method there, and nothing after the wait runs

#### Scenario: A goroutine skips the test
- **WHEN** a goroutine started by gotest.Go skips the test
- **THEN** the wait on it skips the method there, without failing it

#### Scenario: A goroutine leaves without a verdict
- **WHEN** a goroutine started by gotest.Go leaves through runtime.Goexit without a verdict
- **THEN** the wait on it fails the method, saying the goroutine exited via runtime.Goexit

### Requirement: Goroutines BeforeAll starts
A goroutine BeforeAll starts with gotest.Go, on the hook's T or any T derived from it, SHALL be waited for after AfterAll, as SHALL one it starts while that wait runs; the wait SHALL cover every goroutine before the fixture is released, even when the last one fails or panics, and SHALL be held to a declared SetupTimeout. The cases of the last goroutine are the table in `tests/conformance/lifecycle/goroutines_suite_test.go`.

#### Scenario: A goroutine only AfterAll stops
- **WHEN** BeforeAll starts a goroutine with gotest.Go that only AfterAll stops
- **THEN** the suite passes: the wait comes after AfterAll

#### Scenario: A goroutine on a derived T only AfterAll stops
- **WHEN** BeforeAll starts goroutines with gotest.Go on When, It, Each row and NewTWithDeadline Ts that only AfterAll stops
- **THEN** the suite passes: the waits come after AfterAll

#### Scenario: A hook taking a *testing.T
- **WHEN** a BeforeAll taking a *testing.T starts goroutines with gotest.Go on gotest.NewT(t) and NewTWithDeadline(t) that only AfterAll stops
- **THEN** the suite passes: the waits come after AfterAll

#### Scenario: The last of several goroutines goes wrong
- **WHEN** the last of several goroutines BeforeAll started with gotest.Go fails or panics
- **THEN** fails the suite and waits for every other one before the fixture is released

#### Scenario: A goroutine started while the wait runs
- **WHEN** a goroutine BeforeAll started starts another with gotest.Go once AfterAll stopped it
- **THEN** that one is waited for too, and its panic fails the suite

#### Scenario: The wait outlasts the SetupTimeout
- **WHEN** a goroutine BeforeAll started runs past the suite's declared SetupTimeout after AfterAll
- **THEN** fails the suite with a verdict naming the wait and its SetupTimeout

### Requirement: A panic ahead of a failing AfterEach
A panic in a test method or its BeforeEach SHALL be reported and fail the test even when AfterEach then fails a fatal assertion, and AfterEach SHALL still run.

#### Scenario: BeforeEach panics and AfterEach fails
- **WHEN** a test method's BeforeEach panics and its AfterEach fails a fatal assertion
- **THEN** the test fails with the panic reported, and its AfterEach still runs

### Requirement: What an expired --timeout names
An expired --timeout SHALL name as running only the tests whose process it cut short, not a test whose suite process ended on its own before it.

#### Scenario: A crash before the timeout
- **WHEN** one suite's process crashed on its own and another suite hangs until --timeout
- **THEN** names only the hung test as running when the timeout expired

### Requirement: A test waiting to run is not running
An expired --timeout SHALL NOT name a parallel test that was still waiting for a -parallel slot: only the tests holding a slot were running.

#### Scenario: A test waiting for a parallel slot
- **WHEN** parallel methods wait for the one slot -parallel=1 leaves and the method holding it hangs until --timeout
- **THEN** names only the method holding the slot, not those still waiting for it

### Requirement: A hung hook is named by its suite
An expired --timeout SHALL name the test method that was running, and the suite itself when none of its methods had started: a suite that hangs in its BeforeAll, or in a package fixture's BeforeAll, is named as the suite.

#### Scenario: A hook that hangs
- **WHEN** one suite hangs in its BeforeAll and another in a test method until --timeout
- **THEN** names the first suite itself and the second suite's method
