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

### Requirement: Kept verdicts
Migrate SHALL translate a testify assertion, hook or call only where gotest gives the verdict and the guarantees testify gave. Where a mechanical translation would change what a test passes or fails on, or what runs when, migrate SHALL leave the code unconverted or marked under a `TODO(gotest-migrate)` marker that gives the reason and names the gotest way.

#### Scenario: An assertion on a goroutine
- **WHEN** an assertion runs on a goroutine a go statement starts
- **THEN** migrate leaves it unconverted under a marker naming gotest.Go

#### Scenario: Elements compared deeply
- **WHEN** ElementsMatch or Subset compares pointer or interface elements
- **THEN** migrate leaves it unconverted under a marker saying gotest compares them with ==

#### Scenario: Zero or Empty on an interface
- **WHEN** Zero or Empty checks a value of an interface type
- **THEN** migrate leaves it unconverted under a marker saying testify judged the value inside

#### Scenario: An untyped constant of another type
- **WHEN** Equal compares an untyped constant with a value of another type
- **THEN** migrate leaves it unconverted under a marker naming both types

#### Scenario: TearDownSuite after a SetupSuite that can stop early
- **WHEN** a suite declares TearDownSuite and a SetupSuite that can stop early
- **THEN** migrate marks AfterAll, which gotest runs even after BeforeAll stopped

#### Scenario: Parallel and Cleanup on the suite's T
- **WHEN** a test calls s.T().Parallel() or s.T().Cleanup
- **THEN** migrate marks each call, naming SuiteConfig.Parallel, AfterEach and AfterAll

#### Scenario: Tests in another order
- **WHEN** gotest would run the tests of a suite with fields in another order than testify
- **THEN** migrate notes on the suite that the order changed

#### Scenario: A base's tests among the child's
- **WHEN** a base declares a test whose name sorts between the child's own tests
- **THEN** the migrated child declares its forwarding method between them

### Requirement: Module files
Migrate SHALL load the packages it converts with `-mod=readonly`, or `-mod=vendor` for a vendored module outside a workspace, so it never writes go.mod or go.sum, whatever GOFLAGS asks for.

#### Scenario: GOFLAGS asking for -mod=mod
- **WHEN** GOFLAGS asks for -mod=mod and go.sum lacks an entry the load needs
- **THEN** migrate writes neither go.mod nor go.sum

### Requirement: Imports
Migrate SHALL remove the testify imports nothing uses any more and leave no blank line where they stood.

#### Scenario: testify imports between imports that stay
- **WHEN** testify imports sit between imports that stay
- **THEN** the migrated import block holds no blank line where they stood
