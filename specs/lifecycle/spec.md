# Lifecycle Specification

## Purpose
How a suite's tests end, and what the run reports about them when they do.

## Requirements

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
