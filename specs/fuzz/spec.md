# Fuzz Specification

## Purpose
How each fuzz execution's BeforeEach and AfterEach pair when BeforeEach fails, skips or panics, and when -skip drops a suite's fuzz seed replays.

## Requirements

### Requirement: Execution hooks pair
A fuzz execution whose BeforeEach fails or panics SHALL fail, one whose BeforeEach skips SHALL skip without running its body, and in each case its AfterEach SHALL still run.

#### Scenario: BeforeEach fails
- **WHEN** a fuzz execution's BeforeEach fails
- **THEN** the execution fails and its AfterEach still runs

#### Scenario: BeforeEach panics
- **WHEN** a fuzz execution's BeforeEach panics
- **THEN** the execution fails and its AfterEach still runs

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
