# Fuzz Specification

## Purpose
How each fuzz execution's BeforeEach and AfterEach pair when BeforeEach fails, skips or panics, which fuzz seed replays a run keeps when -run or -skip names some of a suite's Test and Fuzz functions, how gotest fuzz and its triage give a target the shared fixtures its suite binds, where a seed of the wrong type is reported, what a fuzz session warns about before it starts, and which crashers `gotest fuzz promote` turns into seeds.

## Requirements

### Requirement: Execution hooks pair
A fuzz execution whose BeforeEach fails or panics SHALL fail, one whose BeforeEach skips SHALL skip without running its body, and in each case its AfterEach SHALL still run; a panic SHALL be reported even when AfterEach then fails a fatal assertion.

#### Scenario: BeforeEach fails
- **WHEN** a fuzz execution's BeforeEach fails
- **THEN** the execution fails and its AfterEach still runs

#### Scenario: BeforeEach panics
- **WHEN** a fuzz execution's BeforeEach panics
- **THEN** the execution fails and its AfterEach still runs

#### Scenario: BeforeEach panics and AfterEach fails
- **WHEN** a fuzz execution's BeforeEach panics and its AfterEach fails a fatal assertion
- **THEN** the execution fails with the panic reported, and its AfterEach still runs

#### Scenario: BeforeEach skips
- **WHEN** a fuzz execution's BeforeEach skips
- **THEN** the execution skips, its body never runs and its AfterEach still runs

### Requirement: Skipping a suite's functions
A -skip filter SHALL drop a suite's fuzz seed replays only when it names every Test and Fuzz function of the suite, as go test does; a filter naming only the suite's Test function SHALL still replay its seeds.

#### Scenario: Skipping the Test function of a suite with fuzz methods
- **WHEN** -skip names a suite's Test function and the suite has fuzz methods
- **THEN** its fuzz seeds still replay and pass
- **AND** its test method does not run

#### Scenario: Skipping every function of a suite
- **WHEN** -skip names every Test and Fuzz function of a suite
- **THEN** the suite does not run: its shared fixture never starts and the run passes

### Requirement: A suite without test methods
A suite with fuzz methods and no test method SHALL have no Test function to select or skip: -skip SHALL drop it when it names every fuzz wrapper, and a -run naming only its Test function SHALL replay none of its seeds.

#### Scenario: Skipping every fuzz wrapper of a suite without test methods
- **WHEN** -skip names every fuzz wrapper of a suite with no test method
- **THEN** the suite does not run: its shared fixture never starts and the run passes

#### Scenario: Running the missing Test function of a suite without test methods
- **WHEN** -run names only the Test function of a suite with no test method
- **THEN** no seed replays and its shared fixture never starts, as the suite has no such function

### Requirement: Fuzzing with shared fixtures
`gotest fuzz` and `gotest fuzz triage` SHALL start the shared fixtures a target's suite binds once, before its processes run, hand their state to every one of them, fuzz workers included, and tear the fixtures down after the last; a target whose fixture does not come up SHALL fail without running.

#### Scenario: Fuzzing a target whose suite binds a shared fixture
- **WHEN** gotest fuzz runs a target whose suite binds a shared fixture
- **THEN** starts the fixture once, its fuzz workers see its state, and it is torn down after

#### Scenario: Fuzzing against a shared fixture that does not come up
- **WHEN** gotest fuzz runs a target whose suite binds a shared fixture that does not come up
- **THEN** fails without running the target, naming the fixture

#### Scenario: Triaging a crasher of a target whose suite binds a shared fixture
- **WHEN** gotest fuzz triage re-runs a crasher of a target whose suite binds a shared fixture
- **THEN** reports the crasher's own failure, seen with the fixture's state, and tears the fixture down

### Requirement: Seed-type errors
An f.Add seed whose type does not match the fuzz target's parameters SHALL fail the target at the f.Add call's file and line, never at gotest's or the generated code's.

#### Scenario: A seed of the wrong type
- **WHEN** an f.Add seed's type does not match the fuzz target's parameters
- **THEN** the failure is located at the f.Add call

### Requirement: Stale corpus entries
A fuzz session SHALL warn about each recorded corpus entry whose values no longer match its target before it spends the budget, naming the entry; the warning SHALL NOT suggest promote, which takes only the latest session's crashers.

#### Scenario: An entry of an older shape
- **WHEN** a fuzz session meets a corpus entry of an older shape
- **THEN** warns, naming the entry, without suggesting promote

### Requirement: Promote takes corpus literals only
`gotest fuzz promote` SHALL read a crasher file only in Go's fuzz corpus format, the `go test fuzz v1` header and one literal per line in the grammar Go's encoder writes. Any other expression or token SHALL be refused, naming the file and line, and then no crasher of the run SHALL be promoted.

#### Scenario: A crasher that is not a corpus literal
- **WHEN** a session's crasher file holds anything but Go's corpus literals
- **THEN** promote refuses it with exit 1, naming the file and line
- **AND** no crasher of the run is promoted and every crasher file stays

### Requirement: Promote takes the latest session's crashers only
`gotest fuzz promote` SHALL take only the crasher files written by the latest `gotest fuzz` session that fuzzed their target, unchanged since, and SHALL leave every other corpus file and its target's source as they are. A session SHALL NOT drop the crashers recorded for targets it did not fuzz, though it shares the gotest cache with other modules and checkouts.

#### Scenario: A session's crasher
- **WHEN** a fuzz session finds a crasher
- **THEN** promote adds it as a seed and removes its file

#### Scenario: A file the session did not write
- **WHEN** a corpus file was not written by the latest fuzz session
- **THEN** promote leaves the file and its target's source untouched

#### Scenario: A session of another checkout
- **WHEN** a later session sharing the cache fuzzes another checkout's target
- **THEN** promote still takes the crashers of the targets it did not fuzz

### Requirement: Promoted seeds hold the crasher's values
`gotest fuzz promote` SHALL write each seed so its target receives exactly the values the crasher holds, a negative zero, the bits of a NaN and the integer limits included, and a decoding target's crasher as one seed of all its arguments.

#### Scenario: Edge values of a decoding target
- **WHEN** a crasher of a decoding target holds a value at the edge of its Go spelling
- **THEN** the promoted seed fails the target as the crasher did

#### Scenario: A decoding target of several arguments
- **WHEN** a decoding target takes several arguments
- **THEN** promote adds its crasher as one seed of all of them
- **AND** the promoted seed fails the target as the crasher did

### Requirement: Promoting a decoded target's crasher
For a fuzz target that decodes its input from the engine's leaves, `gotest fuzz promote` SHALL build the seed from the value the target's codec decodes, and SHALL refuse a crasher it cannot decode that way instead of splicing raw leaf values.

#### Scenario: No decoded value
- **WHEN** a decoding target's crasher re-run echoes no decoded value
- **THEN** promote refuses it with exit 1 and splices no raw leaf
