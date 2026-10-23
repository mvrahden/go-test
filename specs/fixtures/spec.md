# Fixtures Specification

## Purpose
Which shared fixtures a run starts for the suites it selects, guarded suites included, and how a package fixture's per-method hooks run and where their failures are reported.

## Requirements

### Requirement: Selection decides which shared fixtures start
A shared fixture SHALL start only when a selected suite binds it, and a suite with a skipped method SHALL still get its fixture for the methods that run.

#### Scenario: Skipping the only suite that binds a shared fixture
- **WHEN** -skip names the only suite binding a shared fixture
- **THEN** the fixture is never started and the other suite passes

#### Scenario: Skipping one method of a suite that binds a shared fixture
- **WHEN** -skip names one method of the suite binding a shared fixture
- **THEN** the fixture starts and the suite's other method passes against it

### Requirement: Selection follows go test's matching
A suite SHALL be selected when go test would enter one of its top-level functions: -run enters a function the first level of an alternative matches, -skip keeps go test out of it only when the first alternative matching it has no further level, and a repeated -run or -skip counts by its last value. A bench run SHALL select a benchmark suite by its Benchmark function the same way.

#### Scenario: A repeated -skip
- **WHEN** -skip is given twice, naming the fixture-free suite last
- **THEN** the last one counts: the fixture starts, its suite passes and the other suite does not run

#### Scenario: A -skip whose first matching alternative names a method
- **WHEN** -skip's first alternative matching a suite names one of its methods and a later one names the suite
- **THEN** go test still enters the suite, so its fixture starts and the suite passes

#### Scenario: A -run whose alternatives name two suites
- **WHEN** -run's first alternative names a method of one suite and a later one names the other suite
- **THEN** both suites run, the one binding the fixture against it

#### Scenario: Skipping the only benchmark suite that binds a shared fixture
- **WHEN** -skip names the only benchmark suite binding a shared fixture
- **THEN** a bench run never starts the fixture and benchmarks the other suite

### Requirement: A guard does not hold back a shared fixture
A shared fixture bound by a guard-skipped suite SHALL still be started, and its setup failure SHALL fail the run and that suite without the guard being evaluated.

#### Scenario: A guarded suite whose shared fixture fails
- **WHEN** a guard-skipped suite binds a shared fixture whose setup fails
- **THEN** the fixture is started for it and its failure fails the run and the suite, whose guard never runs

### Requirement: Where a fixture hook failure is reported
An error a fixture BeforeEach or AfterEach returns SHALL be reported at the hook's declaration, not inside the runtime, wherever it is declared: in the suite's package, in another package, or promoted from another package's type. The file is named by its base name, or by its full path under -fullpath.

#### Scenario: A fixture BeforeEach error
- **WHEN** a fixture BeforeEach returns an error
- **THEN** is reported at the hook's declaration, not in the runtime's file

#### Scenario: A fixture AfterEach error
- **WHEN** a fixture AfterEach returns an error
- **THEN** is reported at the hook's declaration, not in the runtime's file

#### Scenario: A hook declared in another package
- **WHEN** a hook another package declares returns an error, a fixture's of that package or one promoted from its type
- **THEN** is reported at the hook's declaration, named by its file's base name

#### Scenario: A hook declared in another package, under -fullpath
- **WHEN** a hook another package declares returns an error under -fullpath
- **THEN** names the hook's file by its full path, as go test -fullpath does

### Requirement: Fixture per-method hooks in dependency order
A suite's package fixtures SHALL take their per-method turns in dependency order, a fixture's turn coming only once every BeforeEach before it returned without an error inside the Timeout. In its turn a fixture runs its BeforeEach, if it has one; its AfterEach SHALL run, in reverse, exactly when its turn came and that BeforeEach returned without an error.

#### Scenario: A BeforeEach overruns ahead of another fixture's
- **WHEN** a fixture's BeforeEach overruns the Timeout ahead of another fixture's
- **THEN** starts no later fixture's BeforeEach, fails the method once, at the hook that overran, and runs that fixture's AfterEach

#### Scenario: An AfterEach-only fixture before the one that fails
- **WHEN** a fixture with only an AfterEach comes before a fixture whose BeforeEach fails
- **THEN** runs its AfterEach, as every BeforeEach before it returned

#### Scenario: An AfterEach-only fixture after the one that fails
- **WHEN** a fixture with only an AfterEach comes after a fixture whose BeforeEach fails
- **THEN** does not run its AfterEach, as its turn never came
