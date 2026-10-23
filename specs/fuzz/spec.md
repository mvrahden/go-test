# Fuzz Specification

## Purpose
How each fuzz execution's BeforeEach and AfterEach pair when BeforeEach fails, skips or panics, which fuzz seed replays a run keeps when -run or -skip names some of a suite's Test and Fuzz functions, and how gotest fuzz and its triage give a target the shared fixtures its suite binds.

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
