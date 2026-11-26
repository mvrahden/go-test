# Lint Specification

## Purpose
What `gotest lint` reports, where it reports it, what its fixes change and what they leave alone, how nolint comments, command-line skips and GitHub annotations behave, and how it describes its rules for a reader and for a program.

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
A nolint comment with no rule or with all SHALL suppress every rule on its line; one naming a rule lint does not have SHALL be reported, and SHALL not suppress that report; one naming only another linter's rule SHALL suppress none of lint's own. A name -nolint-allow lists SHALL count as another linter's. lint.nolint-allow of .gotest.yml SHALL list such names for a project, the command line's flag taking precedence, and an entry that is no linter name SHALL be refused with exit 2.

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

#### Scenario: A linter lint was told about
- **WHEN** a nolint comment names a linter that -nolint-allow lists
- **THEN** lint leaves the comment unreported and suppresses none of its own rules

#### Scenario: A linter the project lists
- **WHEN** a nolint comment names a linter that lint.nolint-allow of .gotest.yml lists
- **THEN** lint leaves the comment unreported
- **AND** an entry that is no linter name is refused with exit 2, naming it

### Requirement: A nolint comment gives its reason
A nolint comment that can suppress one of lint's rules, one naming no rule, all or a rule of lint's, SHALL be reported under nolint-reason at the comment unless a reason follows its rules after ` //`, and SHALL not suppress that report. A nolint comment naming only other linters' rules SHALL be left to those linters.

#### Scenario: A nolint comment without a reason
- **WHEN** a nolint comment naming a rule of lint's gives no reason
- **THEN** lint reports the comment under nolint-reason at the comment and still honours its suppression
- **AND** the comment cannot suppress that finding, even naming nolint-reason

#### Scenario: A reason after the rules
- **WHEN** a nolint comment gives a reason after its rules
- **THEN** lint leaves the comment unreported, bare or naming rules

#### Scenario: Another linter's comment without a reason
- **WHEN** a nolint comment naming only another linter's rule gives no reason
- **THEN** lint leaves the comment without a reason to that linter

### Requirement: The suppression list
`gotest lint -suppressions` SHALL print instead of the findings one JSON document that opens with `version` and lists in `suppressions`, sorted by `module`, `file`, `line` and `col`, each nolint comment that can suppress one of lint's rules once: its `file` relative to the module root, its `rules` sorted (`all` for a bare comment), its `reason` and whether it suppressed a finding in the run (`suppressed`). It SHALL exit 0, and SHALL refuse -json, -fix, -diff and a driver-only flag with exit 2.

#### Scenario: Listing the suppressions
- **WHEN** lint -suppressions runs over a package with nolint comments
- **THEN** prints one document opening with version that lists each comment reaching a rule of lint's once, sorted by file and position, with its module, module-relative file, rules and reason, and exits 0
- **AND** marks as suppressed exactly the comments that suppressed a finding
- **AND** leaves out a comment naming only another linter's rule, and prints nothing on stderr

#### Scenario: Another output asked for beside it
- **WHEN** lint -suppressions is given with -json or -fix
- **THEN** lint refuses the pair with exit 2, naming the other flag

### Requirement: Generated files
Lint SHALL report a generated file only when git tracks it or when it lies outside a git repository, and SHALL say so where git cannot tell.

#### Scenario: A generated file on disk
- **WHEN** a generated file is on disk
- **THEN** an untracked generated file is not a finding
- **AND** a generated file git tracks is a finding
- **AND** outside a git repository a generated file is a finding
- **AND** where git cannot read its repository, the finding says git cannot tell whether it is tracked

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
Lint -fix SHALL remove a focus prefix, of a suite or of a Test, Benchmark or Fuzz method, at every reference to the focused suite or method in its package, the receivers and uses in other files, generic receivers, aliases, the fields that embed it, files a build constraint leaves out and the external test package included, so that the package still compiles. A fix SHALL be left unapplied and named, with the file's position, where such a file spells the name and no build type-checks it.

#### Scenario: Methods and uses in several files
- **WHEN** lint -fix removes the focus of a suite whose methods and uses are spread over several files
- **THEN** lint -fix renames every receiver and use, and the package still compiles

#### Scenario: A generic suite with aliases
- **WHEN** lint -fix removes the focus of a generic suite that aliases instantiate
- **THEN** lint -fix renames every generic receiver and alias, and the package still compiles

#### Scenario: The external test package
- **WHEN** lint -fix removes the focus of a suite the external test package refers to
- **THEN** lint -fix renames the external test package's references too

#### Scenario: An embedded suite
- **WHEN** lint -fix removes the focus of a suite another type embeds
- **THEN** lint -fix renames the selectors and literal keys that name the embedded field, and the package still compiles

#### Scenario: Benchmark and fuzz methods
- **WHEN** lint -fix removes the focus of a benchmark or fuzz method
- **THEN** lint -fix renames the focused benchmark and fuzz methods, and the package still compiles

#### Scenario: Files a build constraint leaves out
- **WHEN** lint -fix removes the focus of a suite that files behind a build constraint refer to
- **THEN** lint -fix renames their references too, and the package still compiles with the tag

#### Scenario: A constrained file no build type-checks
- **WHEN** a file behind a build constraint spells the focused name and no build type-checks it
- **THEN** lint -fix leaves the focus and names the file's position

### Requirement: A target that does not compile
Lint SHALL report each compile error of a target package once, with its path relative to the module.

#### Scenario: An uncompilable package
- **WHEN** a lint target does not compile
- **THEN** a package that does not compile is reported once, with its path relative to the module

### Requirement: Shapes the generator rejects
Lint SHALL report each shape the generator rejects at the position the generator rejects it, and discover SHALL mark such a package broken and name each rejection at its position. Lint SHALL leave a rejection to another rule only on a line where an integrity rule's finding is shown.

#### Scenario: Rejected shapes
- **WHEN** a package holds shapes the generator rejects
- **THEN** lint reports each shape the generator rejects at the position it rejects it
- **AND** discover marks the package broken and names each rejection at its position

#### Scenario: Another finding on the rejected line
- **WHEN** a skippable rule reports on a line the generator rejects, shown or suppressed by a nolint comment
- **THEN** lint still reports the generator's rejection on that line
- **AND** lint leaves the rejection to an integrity rule whose finding is shown on that line

### Requirement: GitHub annotations
Lint --github SHALL escape each annotation's message and properties as a workflow command needs, SHALL cut a long annotation on a rune boundary, and SHALL name each file relative to the workspace. --github=false SHALL print the plain report, inside GitHub Actions too, and a value that is no boolean SHALL be refused with exit 2.

#### Scenario: Characters a workflow command escapes
- **WHEN** a finding's message and file carry characters a workflow command escapes
- **THEN** lint --github escapes the annotation's message and properties
- **AND** a long annotation is cut on a rune boundary

#### Scenario: A step in a subdirectory
- **WHEN** lint --github runs in a subdirectory of the workspace
- **THEN** annotation paths are relative to the workspace

#### Scenario: Annotations switched off
- **WHEN** lint runs with --github=false inside GitHub Actions
- **THEN** lint prints the plain report and no annotation

#### Scenario: A value that is no boolean
- **WHEN** --github is given a value that is no boolean
- **THEN** lint refuses it with exit 2, naming the value

### Requirement: Assertion rewrites keep the verdict
An assertion rewrite SHALL leave the test's verdict as it was, and lint SHALL offer no fix where the operand's type would change the verdict; the operand types and assertions are the table in tests/conformance/lint/rewrites_suite_test.go. Lint SHALL report an assertion as redundant only where both check the same operand and every argument of both is a name, field, element or literal, which evaluating cannot change.

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
- **AND** operands that call a function or receive from a channel are left unreported, since each assertion evaluates its own

### Requirement: Failure guards on an Each row
fail-guard SHALL leave unreported a guard whose failing call is given an Each row's T, the first variable of a `range gotest.Each` loop or a plain alias of it, where the guard ends the method with FailNow or a return or holds more than one condition, as an or-chain or an else-if chain. A guard of one condition whose Fail or Errorf goes on SHALL be reported with its fix and no claim that an assertion halts.

#### Scenario: A guard that ends the method
- **WHEN** a guard on an Each row's T ends the method
- **THEN** fail-guard leaves it unreported, through an alias and inside a closure too

#### Scenario: A guard of several conditions
- **WHEN** an or-chained or else-if guard fails an Each row's T
- **THEN** fail-guard leaves it unreported

#### Scenario: A guard that goes on
- **WHEN** a guard of one condition calls Fail or Errorf on an Each row's T
- **THEN** fail-guard reports it with a fix and no claim that an assertion halts

### Requirement: Escapes and redundant pairs on an Each row
t-escape SHALL leave unreported a gotest assertion given `row.T()`, where row is an Each row's T or a plain alias of it: the assertion halts the method there and goes on given the row. assertion-redundant SHALL leave unreported a pair whose assertions are given different T's of which exactly one is an Each row's T.

#### Scenario: An assertion given a row's T()
- **WHEN** an assertion is given an Each row's T()
- **THEN** t-escape leaves it unreported and still fixes one given the outer t.T()

#### Scenario: A redundant pair across a row's T
- **WHEN** a redundant pair splits between an Each row's T and another T
- **THEN** assertion-redundant leaves it unreported and still reports a pair on one T

### Requirement: Rules skip what is not gotest's
Lint's gotest rules, generation-rejects among them, SHALL leave a testify suite to the testify rule, SHALL leave calls through a user type's own T method unreported, SHALL leave a failure guard unreported where a local variable shadows the gotest import, and SHALL leave calls through a user type's own B or F method unreported.

#### Scenario: A testify suite
- **WHEN** a package holds a testify suite
- **THEN** gotest's suite rules and generation-rejects leave a testify suite to the testify rule

#### Scenario: A T method of the user's own
- **WHEN** a type of its own has a T method
- **THEN** lint leaves calls through that T method unreported

#### Scenario: A local named gotest
- **WHEN** a local variable shadows the gotest import
- **THEN** fail-guard leaves a guard in the local's scope unreported and reports one where gotest names the package

#### Scenario: A B method of the user's own
- **WHEN** a type of its own has a B method
- **THEN** lint leaves calls through that B method unreported

### Requirement: Escapes from every wrapper
The t-escape and suite-lifecycle rules SHALL report a method reached through `b.B()` on a gotest.B or `f.F()` on a gotest.F like one through `t.T()`: a method the wrapper has with a fix to call it there; Helper, Log, Fatal, Error, Fail, Skip and SkipNow without one; Cleanup as suite-lifecycle. They SHALL leave what gotest.B lacks by design, such as `N`, the timers or `Run`, unreported, and the bench and fuzz rules SHALL read a loop or seed written through the escape as one written directly.

#### Scenario: A test escaping to testing.T
- **WHEN** a test calls Name, Setenv and TempDir through t.T()
- **THEN** lint reports each as a t-escape with a fix

#### Scenario: A benchmark escaping to testing.B
- **WHEN** a benchmark calls Loop, Helper, Cleanup, Setenv and TempDir through b.B() and reads b.B().N
- **THEN** lint reports Loop as a t-escape with a fix, Helper as a t-escape without one and Cleanup as suite-lifecycle, and leaves N unreported
- **AND** lint reports Setenv and TempDir as t-escapes with a fix

#### Scenario: A fuzz target escaping to testing.F
- **WHEN** a fuzz target seeds through f.F().Add and calls TempDir through f.F()
- **THEN** lint reports the seed as a t-escape with a fix and fuzz-seed stays silent
- **AND** lint reports TempDir as a t-escape with a fix

#### Scenario: A loop written through the escape
- **WHEN** a benchmark loops with b.B().Loop() and sleeps inside it
- **THEN** bench-loop stays silent and bench-wait reports the sleep

#### Scenario: The fix calls the wrapper
- **WHEN** lint -fix rewrites a benchmark that loops and asserts through b.B()
- **THEN** the loop and the assertion call the gotest.B directly
- **AND** Setenv and TempDir through b.B() call the gotest.B directly

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

### Requirement: The rules
`gotest lint -rules` SHALL print the rule list `gotest help lint` prints, and with -json one JSON document that opens with `version` and lists in `rules` every rule once with its `id`, its `tier` (integrity, expressiveness or migration), its `scope`, whether -skip-<rule> accepts it (`skippable`), whether it can attach a suggested fix (`fixable`) and a one-line `description`. Neither SHALL need a package, and both SHALL exit 0.

#### Scenario: Asking for the rules
- **WHEN** the rules are asked for with lint -rules
- **THEN** prints the rule list gotest help lint prints and exits 0, with no package to lint
- **AND** with -json prints one document opening with version that lists every rule once with its tier, scope, skippable, fixable and description

### Requirement: Findings cover the code they are about
In the findings `gotest lint -json` prints, each finding's `end` SHALL lie past its `posn`, at the end of the call, expression or declaration header the finding is about.

#### Scenario: A finding's extent
- **WHEN** lint -json reports a finding on a call and one on a function
- **THEN** ends each past its posn, at the end of the call and of the function's signature

### Requirement: A finding names its rule
Lint's plain report SHALL end each finding's line with its rule in parentheses, the id a nolint comment takes. Lint -json SHALL carry the rule as the finding's `category` and leave its `message` without it.

#### Scenario: The rule a finding is reported under
- **WHEN** lint reports a finding
- **THEN** the plain line ends with the rule in parentheses
- **AND** the -json finding carries the rule as its category and a message without it

### Requirement: Assertions of another library inside a poll
Inside a polling callback lint SHALL report a gotest assertion whose first argument is not the poll parameter, and a call that only bears an assertion's name where its first argument is a test handle: a testing.T, B, F or TB, or a gotest.T, B, F or R other than the poll parameter. A call that shares the name and takes anything else SHALL be left unreported.

#### Scenario: A call named like an assertion
- **WHEN** a polling callback calls functions that only share an assertion's name
- **THEN** lint reports the one handed the test's t and leaves those on plain values unreported

### Requirement: A benchmark's loop in a helper
bench-loop SHALL read a loop as the benchmark's own when the benchmark hands its b, the escape of it or an alias of that escape to a function or method of its package that names Loop or N on that parameter, itself or through at most five further such calls. It SHALL stay silent when the benchmark hands b to a function whose body it cannot read, gotest's own apart, and SHALL report a benchmark whose helpers never loop.

#### Scenario: A loop held by a helper
- **WHEN** a benchmark hands its b to a helper of its package that holds the loop
- **THEN** bench-loop stays silent on it, through a second helper too
- **AND** bench-loop still reports a benchmark whose helper only asserts on b

### Requirement: Cases that never run
Lint SHALL report, under dropped-case, a Test, Benchmark or Fuzz method taking a gotest.T, B or F no suite runs: on a test file's type without the TestSuite suffix, named just Test, Benchmark or Fuzz on a suite, or on a test file's generic suite no alias instantiates. It SHALL leave unreported a method either test package of the directory refers to, a type either aliases as a suite, and a non-test file's declarations; a driver seeing one test package SHALL report only where there is no other.

#### Scenario: A case on a type that is no suite
- **WHEN** a test file declares a Test method taking a gotest.T on a type named without the TestSuite suffix
- **THEN** lint reports that the method never runs and names the suffix the type lacks
- **AND** lint leaves such a method unreported where the package calls it

#### Scenario: A method named exactly Test
- **WHEN** a suite declares methods named exactly Test, Benchmark and Fuzz
- **THEN** lint reports each as a case that never runs

#### Scenario: A generic suite without an alias
- **WHEN** a test file declares a generic suite with a case
- **THEN** lint reports the suite no alias instantiates at its declaration
- **AND** lint leaves the suite an alias instantiates unreported

#### Scenario: A case only the external test package reaches
- **WHEN** the external test package takes a case of a type that is no suite as a value, a method expression or an interface method, calls it, or aliases the type as a suite
- **THEN** lint leaves each unreported, as the generator runs it
- **AND** lint reports a case the external package reaches from nowhere, a comment naming it included

#### Scenario: A case reached where the build leaves it out, or through an interface
- **WHEN** a helper's only caller or a generic suite's only alias sits in a test file the build constraints leave out, or a helper is handed to an interface that holds its case
- **THEN** lint leaves each unreported, as the generator runs it
- **AND** lint reports beside them a case reached from nowhere

#### Scenario: A driver that sees one test package
- **WHEN** go vet runs the exported analyzer on directories with cases no suite runs
- **THEN** dropped-case reports nothing in a directory with an external test package, leaving it to the generator
- **AND** dropped-case reports the cases of a directory with no external test package

### Requirement: Other drivers
The analyzer pkg/lint exports SHALL report under go vet -vettool what gotest lint reports.

#### Scenario: go vet with a vettool
- **WHEN** go vet runs a unitchecker main around the exported analyzer as its vettool
- **THEN** go vet prints lint's findings for the package
