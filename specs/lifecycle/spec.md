# Lifecycle Specification

## Purpose
How a suite's tests end, the rows of an Each table, the goroutines gotest.Go starts and a panic ahead of AfterEach included, and what the run reports about them when they do.

## Requirements

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
