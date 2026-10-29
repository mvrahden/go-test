# Lint Specification

## Purpose
What `gotest lint` reports, where it reports it, what its fixes change and what they leave alone, and how nolint comments, command-line skips and GitHub annotations behave.

## Requirements

### Requirement: Suite and fixture declarations
Lint SHALL report a suite declared inside a type block, a suite that embeds a fixture and a direct call of a fixture's lifecycle hook, except a shared fixture's own Hydrate or Dehydrate called from its BeforeAll or AfterAll, each at the position where it is written, and lint -fix SHALL move a grouped suite into its own declaration that still generates.

#### Scenario: A suite in a type block
- **WHEN** a suite is declared inside a type block
- **THEN** lint reports a suite declared in a type block at its declaration
- **AND** lint -fix moves it into its own declaration, doc comment kept, and the package then generates

#### Scenario: An embedded fixture
- **WHEN** a suite embeds a fixture
- **THEN** lint reports an embedded fixture at the field with the alternative

#### Scenario: A hook called by hand
- **WHEN** a suite method calls a fixture's lifecycle hook
- **THEN** lint reports a direct call of a fixture's lifecycle hook at the call

#### Scenario: A shared fixture connecting in its own setup
- **WHEN** a shared fixture's BeforeAll or AfterAll calls its own Hydrate or Dehydrate
- **THEN** lint leaves the call unreported, since the process that owns the fixture runs neither

### Requirement: Nolint comments
A nolint comment with no rule or with all SHALL suppress every rule on its line; one naming a rule lint does not have SHALL be reported, and SHALL not suppress that report; one naming only another linter's rule SHALL suppress none of lint's own.

#### Scenario: A bare nolint
- **WHEN** a nolint comment names no rule
- **THEN** a bare nolint comment suppresses every rule on its line

#### Scenario: nolint:all
- **WHEN** a nolint comment names all
- **THEN** nolint:all suppresses every rule on its line

#### Scenario: An unknown rule
- **WHEN** a nolint comment names a rule lint does not have
- **THEN** a nolint comment naming an unknown rule is a finding
- **AND** the comment naming the unknown rule cannot suppress that finding

#### Scenario: Another linter's rule
- **WHEN** a nolint comment names another linter's rule
- **THEN** lint leaves the rule to that linter and suppresses none of its own

### Requirement: Generated files
Lint SHALL report a generated file only when git tracks it or when it lies outside a git repository.

#### Scenario: A generated file on disk
- **WHEN** a generated file is on disk
- **THEN** an untracked generated file is not a finding
- **AND** a generated file git tracks is a finding
- **AND** outside a git repository a generated file is a finding

### Requirement: Integrity rules
Lint SHALL refuse a command-line skip of an integrity rule, say why it cannot be skipped and exit 2.

#### Scenario: Skipping an integrity rule
- **WHEN** the command line skips an integrity rule
- **THEN** skipping an integrity rule on the command line says why it cannot be skipped and exits 2

### Requirement: A fix that does not compile
Lint -fix SHALL drop and name a fix that does not compile and SHALL still apply the other fixes.

#### Scenario: One fix breaks the build
- **WHEN** a fix does not compile
- **THEN** a fix that does not compile is dropped and named
- **AND** the other fixes are applied

### Requirement: The focus fix
Lint -fix SHALL remove a focus prefix at every reference to the focused suite in its package, the receivers and uses in other files, generic receivers, aliases and the external test package included, so that the package still compiles.

#### Scenario: Methods and uses in several files
- **WHEN** lint -fix removes the focus of a suite whose methods and uses are spread over several files
- **THEN** lint -fix renames every receiver and use, and the package still compiles

#### Scenario: A generic suite with aliases
- **WHEN** lint -fix removes the focus of a generic suite that aliases instantiate
- **THEN** lint -fix renames every generic receiver and alias, and the package still compiles

#### Scenario: The external test package
- **WHEN** lint -fix removes the focus of a suite the external test package refers to
- **THEN** lint -fix renames the external test package's references too

### Requirement: A target that does not compile
Lint SHALL report each compile error of a target package once, with its path relative to the module.

#### Scenario: An uncompilable package
- **WHEN** a lint target does not compile
- **THEN** a package that does not compile is reported once, with its path relative to the module

### Requirement: Shapes the generator rejects
Lint SHALL report each shape the generator rejects at the position the generator rejects it, and discover SHALL mark such a package broken and name each rejection at its position.

#### Scenario: Rejected shapes
- **WHEN** a package holds shapes the generator rejects
- **THEN** lint reports each shape the generator rejects at the position it rejects it
- **AND** discover marks the package broken and names each rejection at its position

### Requirement: GitHub annotations
Lint --github SHALL escape each annotation's message and properties as a workflow command needs, and SHALL cut a long annotation on a rune boundary.

#### Scenario: Characters a workflow command escapes
- **WHEN** a finding's message and file carry characters a workflow command escapes
- **THEN** lint --github escapes the annotation's message and properties
- **AND** a long annotation is cut on a rune boundary

### Requirement: Assertion rewrites keep the verdict
An assertion rewrite SHALL leave the test's verdict as it was, and lint SHALL offer no fix where the operand's type would change the verdict; the operand types and assertions are the table in tests/conformance/lint/rewrites_suite_test.go.

#### Scenario: Nil checks
- **WHEN** an assertion checks an operand against nil
- **THEN** a nil comparison of a concrete pointer is not rewritten into an error assertion
- **AND** a Nil check of an interface is reported without a fix
- **AND** a type parameter is read as its core type and unsafe.Pointer as nilable

#### Scenario: Bool literals
- **WHEN** an assertion compares an operand with a bool literal
- **THEN** the comparison is rewritten into True or False only for an operand of type bool

#### Scenario: Error messages
- **WHEN** an assertion searches the result of an Error call
- **THEN** the search is rewritten into ErrorContains only on a receiver that implements error

#### Scenario: Orderings
- **WHEN** an assertion states an ordering comparison
- **THEN** a float comparison is not rewritten into an ordering assertion

#### Scenario: Lengths
- **WHEN** an assertion checks a length
- **THEN** a length check is rewritten only for an operand of a string, slice, map, channel or array type

#### Scenario: Equalities
- **WHEN** True or False asserts an == or != comparison
- **THEN** pointers are compared with Same, values that reach no pointer with Equal, and nothing else is rewritten

#### Scenario: Explicit type arguments
- **WHEN** Equal compares an operand with nil under an explicit type argument
- **THEN** Equal is not rewritten, since the type argument decides what it compares

#### Scenario: errors.Is
- **WHEN** True asserts an errors.Is call
- **THEN** ErrorIs is named without a fix unless the target is the nil literal

#### Scenario: Guards that fail every input
- **WHEN** ErrorIs is given a nil target or ErrorContains an empty substring
- **THEN** the assertion the guard names is offered without a fix

#### Scenario: Redundant assertions
- **WHEN** an assertion is followed by one that checks the same operand
- **THEN** the first is removed only where the second fails on every input the first fails

### Requirement: Rules skip what is not gotest's
Lint's gotest rules, generation-rejects among them, SHALL leave a testify suite to the testify rule, SHALL leave calls through a user type's own T method unreported, and SHALL leave a failure guard unreported where a local variable shadows the gotest import.

#### Scenario: A testify suite
- **WHEN** a package holds a testify suite
- **THEN** gotest's suite rules and generation-rejects leave a testify suite to the testify rule

#### Scenario: A T method of the user's own
- **WHEN** a type of its own has a T method
- **THEN** lint leaves calls through that T method unreported

#### Scenario: A local named gotest
- **WHEN** a local variable shadows the gotest import
- **THEN** fail-guard leaves a guard in the local's scope unreported and reports one where gotest names the package

### Requirement: Misuse reached through a helper
Lint SHALL report the test's outer t handed to a helper inside a polling callback.

#### Scenario: The outer t inside a poll
- **WHEN** a polling callback hands the test's t to a helper
- **THEN** lint reports the outer t passed to a helper inside a polling callback

### Requirement: Messages and fixes keep what the source said
A lint message SHALL name the check it reports, and lint -fix SHALL keep the messages and comments of the assertions it rewrites.

#### Scenario: A len check
- **WHEN** True asserts len(x) != 0
- **THEN** lint names the len != 0 check in its message

#### Scenario: Messages and comments in a fix
- **WHEN** lint -fix rewrites assertions that carry messages and comments
- **THEN** the stronger assertion takes a removed one's message and its comment stays
- **AND** a rewritten assertion keeps the comment between its arguments
