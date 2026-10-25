# Lifecycle Specification

## Purpose
How a suite's tests end, the hooks and the body going wrong, FailFast, panics among parallel methods and in benchmarks, the rows of an Each table, the goroutines gotest.Go starts, a panic ahead of AfterEach included, and what the run reports about them when they do.

## Requirements

### Requirement: A hook or body going wrong
A suite hook, a fixture's per-method hook or the body that fails, panics or overruns the Timeout SHALL fail the run and its method or suite, located at that hook, and SHALL run the later hooks that release what ran, a fixture's AfterEach only after its BeforeEach returned: after an overrun only AfterEach hooks, after a panic in a method no further method. The cases are the tables in `tests/conformance/lifecycle/hooks_suite_test.go`.

#### Scenario: Each hook and each way it goes wrong
- **WHEN** a suite hook or the body fails an assertion, panics or overruns the Timeout
- **THEN** fails the run with exit 1 and the method or suite whose hook went wrong, located at that hook when it panics or overruns, running the later hooks the row lists and, after a panic in a method's hooks or body, no further method

#### Scenario: Each fixture per-method hook and each way it goes wrong
- **WHEN** a fixture's BeforeEach or AfterEach returns an error, panics or overruns the Timeout
- **THEN** fails the run with exit 1 and the method, naming the hook and located at it, running the later hooks the row lists and, after a panic, no further method

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

### Requirement: A context the default deadline ended
When the default deadline of a suite that declares no Timeout or SetupTimeout ends the context of a test method, BeforeAll or AfterAll still running, the harness SHALL write one line naming it and the default, unbuffered on stderr or, under -json, with its test, and SHALL leave its verdict as it is. A declared budget reports its own verdict, so it gets no such line, and neither does a context a stop or the end of its phase ended.

#### Scenario: A method waits past the default deadline
- **WHEN** a method of a suite that declares no Timeout waits on t.Context() past the default 30s deadline
- **THEN** prints one line naming the method and the default deadline
- **AND** leaves the method's verdict as it is

#### Scenario: A method waits past a declared Timeout
- **WHEN** a suite declares a Timeout and its method waits on t.Context() past it
- **THEN** fails the method by the Timeout and prints no line about the default deadline

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

### Requirement: The grace a stopped method gets
When a run is stopped, a method body or a suite's BeforeAll that has not returned 5s after its context ended, counted from its own start when it began after the stop, SHALL be cut short with a message saying so, its fixture released and its AfterAll not run; the 5s SHALL bound only the body and BeforeAll, and the stop SHALL NOT end AfterEach's context, so an AfterEach SHALL run to its end, followed by AfterAll and the fixture release.

#### Scenario: A body that ignores its context
- **WHEN** the run is stopped and a method body ignores its context
- **THEN** cuts the body short after 5s, saying so
- **AND** releases the fixture without running AfterAll

#### Scenario: A body that starts after the stop
- **WHEN** the run is stopped while a method's BeforeEach runs and its body then ignores its context
- **THEN** cuts the body short 5s after it began, saying so

#### Scenario: A BeforeAll that ignores its context
- **WHEN** the run is stopped while a suite's BeforeAll ignores its context
- **THEN** cuts BeforeAll short after 5s, saying so, and releases the fixture without running AfterAll

#### Scenario: A slow AfterEach
- **WHEN** the run is stopped and a method's AfterEach outlasts the 5s grace its body gets
- **THEN** runs AfterEach to its end, then AfterAll, then releases the fixture: the grace bounds only the body

#### Scenario: An AfterEach that waits on its context
- **WHEN** the run is stopped and a method's AfterEach waits on its context
- **THEN** leaves that context alone, so AfterEach runs to its end before AfterAll and the release

### Requirement: A forced stop
A stopped run still going when only its fixtures' teardown time is left of the release bound SHALL be forced, saying so, and the fixture teardown SHALL still get that time; a second stop signal reaching the suite SHALL force the stop at once, saying so, releasing the fixtures before the process ends by the signal.

#### Scenario: A run that outlasts its release bound
- **WHEN** the run is stopped and an AfterEach ignores its context until only the fixtures' teardown time is left of the release bound
- **THEN** forces the stop, saying so, and runs the fixture teardown to its end

#### Scenario: A second stop signal
- **WHEN** a second stop signal reaches the suite while a stopped method body ignores its context
- **THEN** releases the fixture at once, saying so, and the process ends by the signal

### Requirement: Stopping benchmarks and fuzz seeds
A run stopped while a benchmark or a fuzz seed runs SHALL skip the benchmarks or seeds not started with the reason run stopping and run AfterAll before the fixture is released; a benchmark that ignores the stop SHALL be cut short after 5s, saying so, and its fixture released.

#### Scenario: Stopped during a benchmark
- **WHEN** the run is stopped while a benchmark runs
- **THEN** skips the benchmarks not started with the reason run stopping, then runs AfterAll before the fixture is released
- **AND** cuts a benchmark that ignores the stop short after 5s, saying so, and releases the fixture

#### Scenario: Stopped during a fuzz seed
- **WHEN** the run is stopped while a fuzz seed replays
- **THEN** skips the seeds not started with the reason run stopping, then runs AfterAll before the fixture is released

### Requirement: Stopping a run
A run interrupted while a method holds SHALL exit 130 when gotest runs directly, end the holding method's context, skip the methods that have not started with the reason run stopping, and run AfterAll before the fixture is released. Under the gotest runner the suite's process then ends with its tests' own status, and the -json stream ends with a failed run naming the suites it stopped; under plain go test it ends by the signal.

#### Scenario: Interrupted while a method holds
- **WHEN** the run is interrupted while a method holds
- **THEN** exits 130 when gotest runs directly
- **AND** ends the holding method's context
- **AND** skips the methods that have not started with the reason run stopping
- **AND** runs AfterAll before the fixture is released
- **AND** ends the stream with a failed run naming the suite it stopped, the tests' own verdicts kept

### Requirement: Every launcher stops cleanly
A run stopped by SIGINT, SIGTERM or a second SIGINT SHALL end with its exit code, or by the signal where go run dies of a SIGTERM, SHALL finish the fixture teardown unless a second signal abandons it and says so, and SHALL leave no suite process behind, whether gotest runs directly, under an -exec wrapper, through go run or through go tool. The cases are the table in `tests/conformance/lifecycle/launcher_unix_test.go`.

#### Scenario: Each launcher and each signal
- **WHEN** a run is stopped by a signal, however gotest was launched
- **THEN** ends with its exit code, or by the signal where go run dies of a SIGTERM, finishes the fixture teardown unless a second signal abandons it saying so, and leaves no suite process behind

### Requirement: No process outlives the run
A run SHALL leave no process a test started behind, even one the test never waits for.

#### Scenario: A process nobody waits for
- **WHEN** a test starts sleep 4242 and never waits for it
- **THEN** the test passes and no such process outlives the run

### Requirement: A second Ctrl-C to gotest
A second SIGINT reaching gotest while its suites stop SHALL, on Unix, send each suite process a second stop signal, which releases the fixtures at once, and kill what still runs two seconds later or at a further SIGINT; on Windows, where no second stop reaches a suite, it SHALL kill the suites at once. It SHALL ask the shared fixture process to tear down within those two seconds, and SHALL exit 130 leaving no suite process behind.

#### Scenario: A method that ignores the stop
- **WHEN** a second SIGINT reaches gotest while a stopped method body ignores its context
- **THEN** releases the fixture at once, without AfterAll
- **AND** exits 130 and leaves no suite process behind

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

### Requirement: A stop signals only the suite's own processes
A cancelled run SHALL send no signal to a pid the suite published that is outside the suite's process group, and SHALL interrupt the suite's process group instead.

#### Scenario: A published pid outside the group
- **WHEN** the run is interrupted and the pid the suite published names a process outside its group
- **THEN** sends that process no signal
- **AND** interrupts the suite's process group instead
- **AND** runs AfterAll before the fixture is released

### Requirement: A stop ends what the tests started
A run stopped while a test has a process of its own running SHALL end that process before gotest exits: once the suite's process has torn down, what is left of its process tree is, on Unix, interrupted and, past the teardown budget, killed, and on Windows killed at once.

#### Scenario: A process a test started
- **WHEN** the run is interrupted while a holding method has started a process of its own
- **THEN** ends that process before gotest exits
- **AND** interrupts it before killing it
- **AND** runs AfterAll before the fixture is released
