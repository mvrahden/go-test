# Migrate Specification

## Purpose
What `gotest migrate` makes of testify suites: the code it writes, what it leaves marked, and when it refuses to run.

## Requirements

### Requirement: Base suites
Migrate SHALL convert a testify suite embedding a base suite of the migrated test files, generic or not, by value or by a pointer each suite.Run sets to a fresh base: the child SHALL hold the base by value in a named field and declare the hooks and tests testify ran on it through the base, the base SHALL become a helper type gotest does not run, a child method calling a base method that gains t SHALL gain t, and no marker SHALL be left for the embedding.

#### Scenario: A child of a base suite
- **WHEN** a testify suite embeds a base suite of the migrated test files
- **THEN** the migrated child compiles and gotest runs the base's hooks and tests on it
- **AND** the base becomes a helper type gotest does not run as a suite
- **AND** migrate leaves no marker for the embedding
- **AND** gotest lint reports no testify finding on the migrated code

#### Scenario: A base embedded by pointer
- **WHEN** a testify suite embeds a base suite of the migrated test files by a pointer its suite.Run sets to a fresh base
- **THEN** the migrated child holds the base by value and gotest runs the base's hooks and tests on it

#### Scenario: A generic base
- **WHEN** a testify suite embeds an instance of a generic base suite
- **THEN** the base stays a generic helper type and gotest runs its hooks and tests on the child

#### Scenario: A child's method calling the base
- **WHEN** a method of the child without t calls a base method that gains t
- **THEN** the method gains t, its callers pass it, and the migrated child runs without a marker

### Requirement: Base suites left as they are
Migrate SHALL leave a suite's file unconverted under a marker giving the reason when its base cannot carry over: an embedding by any other pointer, which gotest's new leaves nil; a local or anonymous struct embedding a base, as gotest runs only package-level suites; a base in a non-test file of the suite's directory, as migrate changes only test files. A base of a package in another directory SHALL make migrate exit 2, naming its package, with nothing written.

#### Scenario: An embedding in a local or anonymous struct
- **WHEN** a struct declared inside a function, or an anonymous struct, embeds a base suite
- **THEN** migrate leaves its file unconverted under a marker saying gotest runs only suites declared at package level

#### Scenario: A base in a non-test file
- **WHEN** a testify suite embeds a base suite declared in a non-test file of its directory
- **THEN** migrate exits 1 and leaves the child's file unconverted under a marker naming that file

#### Scenario: A base outside the migrated directory
- **WHEN** a testify suite embeds a base suite of a package in another directory
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

#### Scenario: NotNil on an interface
- **WHEN** NotNil checks a value of an interface type
- **THEN** migrate leaves it unconverted under a marker naming the typed nil and the non-nilable value inside

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
Migrate SHALL remove the testify imports nothing uses any more, with the comments that stand with them, and leave no blank line where they stood.

#### Scenario: testify imports between imports that stay
- **WHEN** testify imports sit between imports that stay
- **THEN** the migrated import block holds no blank line where they stood

#### Scenario: A comment on a removed import
- **WHEN** an import migrate removes carries a comment on its line or above it
- **THEN** the comment goes with the import and the file stays as gofmt formats it
