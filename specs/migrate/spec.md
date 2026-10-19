# Migrate Specification

## Purpose
What `gotest migrate` makes of testify suites: the code it writes, what it leaves marked, and when it refuses to run.

## Requirements

### Requirement: Base suites
Migrate SHALL convert a testify suite that embeds a base suite declared in the test files being migrated: the child SHALL hold the base in a named field and declare the hooks and tests testify ran on it through the base, the base SHALL become a helper type gotest does not run, and no marker SHALL be left for the embedding. A base declared outside those test files SHALL make migrate exit 2, naming the base's package, with nothing written.

#### Scenario: A child of a base suite
- **WHEN** a testify suite embeds a base suite of the migrated test files
- **THEN** the migrated child compiles and gotest runs the base's hooks and tests on it
- **AND** the base becomes a helper type gotest does not run as a suite
- **AND** migrate leaves no marker for the embedding
- **AND** gotest lint reports no testify finding on the migrated code

#### Scenario: A base outside the migrated test files
- **WHEN** a testify suite embeds a base suite declared outside the migrated test files
- **THEN** migrate exits 2 naming the base's package and writes nothing
