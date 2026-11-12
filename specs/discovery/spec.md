# Discovery Specification

## Purpose
Which declarations gotest reads as suites, fixtures and cases, which shapes it rejects and where, how focus is decided across a run, and how the generated harness stays in step with the suites it is generated from.

## Requirements

### Requirement: Suite and fixture declarations
A suite or fixture SHALL be read only from a type declaration of its own that composes through named fields; a declaration inside a type block, an embedded suite or fixture, an embedded type with a method gotest would read, and a field of suite type SHALL be rejected at the declaration or field, naming the alternative, while an embedded plain type SHALL be accepted.

#### Scenario: Each way a suite or fixture is declared
- **WHEN** a suite or fixture is declared
- **THEN** a suite declared alone, binding a fixture through a named pointer field, runs
- **AND** a suite embedding a plain type runs
- **AND** a suite in a type block is rejected at its declaration and told to stand alone
- **AND** a fixture in a type block is rejected at its declaration and told to stand alone
- **AND** a suite embedding a suite is rejected at the field and pointed to contract suites
- **AND** a suite embedding a fixture is rejected at the field and pointed to a named pointer field
- **AND** a suite embedding a type with a method gotest would read is rejected at the field
- **AND** a suite holding a suite in a field is rejected at the field: suites are never nested
- **AND** a fixture embedding a fixture is rejected at the field and pointed to a named pointer field

### Requirement: Aliases and signatures
An alias of a suite, at any depth and as a method receiver, SHALL stand for the type it names, whose own methods alone it runs; a defined type over a suite or over a type that declares a case, a Test method that returns a value and a shared fixture declared in a _test.go file SHALL be rejected where they are written.

#### Scenario: Aliases and unusual signatures
- **WHEN** a suite is declared through an alias or with an unusual signature
- **THEN** a method declared on a pointer to an alias of the suite runs
- **AND** an alias of an alias of a generic suite runs its methods
- **AND** a defined type over a suite is rejected at its declaration and pointed to an alias
- **AND** a defined type over a type that declares a case is rejected at its declaration and pointed to an alias
- **AND** an alias runs the methods of the type it names, not those of a type named like the alias
- **AND** a Test method that returns a value is rejected at the method
- **AND** a shared fixture declared in a _test.go file is rejected at its declaration

### Requirement: Fixture fields and case names
A suite SHALL bind each fixture once, through a pointer field or an alias of one, and a fixture-named type no suite reaches SHALL NOT fail the package, and a method SHALL be a case only by a name go test would run; one taking the T, B or F of its case under a name go test would not run SHALL be rejected at the method.

#### Scenario: Fields and method names
- **WHEN** a suite names its fixtures and methods
- **THEN** a fixture field by value is rejected at the field and told to take a pointer
- **AND** a fixture field typed by an alias of the fixture pointer binds the fixture
- **AND** two fields of the same fixture type are rejected at the second field
- **AND** a fixture-named type no suite reaches cannot fail the package
- **AND** a method whose name continues Test with a lowercase letter is not a case, as under go test
- **AND** a method taking a T whose name continues Test with a lowercase letter is rejected at the method, as go vet rejects such a test

### Requirement: Suites without a case
A type named like a suite SHALL declare at least one Test, Benchmark or Fuzz method, an excluded one included; one that declares none SHALL be refused at its declaration, naming the alternative, and discover SHALL report its package broken with the refusal, while a testify suite SHALL be left to testify.

#### Scenario: A type named like a suite with no case
- **WHEN** a type named like a suite declares no case
- **THEN** the suite is refused at its declaration, told to declare a case or name the helper otherwise
- **AND** discover reports the package broken with the refusal
- **AND** a suite whose only case is excluded runs as skipped
- **AND** a testify base suite that declares no case is left to testify

### Requirement: Excluded suites
A suite the X_ prefix excludes SHALL be reported skipped as excluded by the user, with none of its methods run, while the suites beside it run.

#### Scenario: An excluded suite beside one that runs
- **WHEN** a package holds an X_ suite beside a suite that runs
- **THEN** reports the X_ suite skipped as excluded by the user, runs none of its methods and passes the suite beside it

### Requirement: Behaviors in discover
discover SHALL report, for each behavior a method declares, the call that declared it, a label that holds a slash as one behavior under the call around it, and each suite of the external test package marked external.

#### Scenario: A When with Its under it
- **WHEN** a method declares a When with Its under it
- **THEN** discover reports the When's kind as when and each It's as it

#### Scenario: Labels with slashes
- **WHEN** a method declares Its and a When whose labels start or end with a slash
- **THEN** discover reports each label as one behavior under the call around it

#### Scenario: Suites of the external test package
- **WHEN** a package's external test package declares suites
- **THEN** discover marks each of them external
- **AND** discover leaves the internal suite of the same name unmarked

### Requirement: Discover lists what the source declares
`gotest discover` SHALL list every Test, Benchmark and Fuzz method a suite declares, an excluded or unfocused one included and marked as such, whatever a run of the package would select.

#### Scenario: Excluded and unfocused methods
- **WHEN** a suite declares an excluded method and a focused one
- **THEN** discover lists the excluded method marked excluded and the unfocused methods beside the focused one

### Requirement: Declarations from other packages
A suite SHALL run an alias of a generic suite another package declares, an alias declared in the external test package SHALL be discovered and run whether the package under test, the external test package or a third package declares the generic suite, and a fixture a suite binds from another package SHALL be held to the signature rules a local fixture is held to.

#### Scenario: Across packages
- **WHEN** a suite reaches across packages
- **THEN** an alias of a generic suite from another package runs that suite's methods
- **AND** a package fixture from another package with a mis-signed hook is rejected with the signature it needs
- **AND** a package fixture from another package with a mis-signed FixtureConfig is rejected at the method
- **AND** a package fixture from another package with Hydrate is rejected at the method

#### Scenario: Generic aliases in the external test package
- **WHEN** the external test package declares an alias of a generic suite
- **THEN** an alias of a generic suite the package under test declares runs that suite's methods
- **AND** an alias of a generic suite the external test package declares runs that suite's methods
- **AND** an alias of a generic suite from a third package runs that suite's methods

### Requirement: Focus across a run
Focus SHALL be decided over the whole run: a focused suite or a focused method of any kind, Test, Benchmark or Fuzz, in any package or test package variant SHALL skip every unfocused suite, which then starts no fixture, and --ci SHALL still refuse the run, whatever kind of method is focused.

#### Scenario: A focused suite in the run
- **WHEN** a run holds a focused suite
- **THEN** a focused suite in one package skips every unfocused suite in the others
- **AND** an unfocused suite skipped by focus starts no fixture
- **AND** a focused suite in the internal test package skips the external one's suites
- **AND** --ci still refuses a run with a focused suite

#### Scenario: A focused method in the run
- **WHEN** a run holds a focused method
- **THEN** a focused method in one package skips every unfocused suite in the others
- **AND** a focused fuzz method in one package skips every unfocused suite in the others

### Requirement: Toggling focus
refactor toggle-focus SHALL add or remove the F_ prefix of a suite or method at every reference to it in its package, embedding fields, files a build constraint leaves out and the external test package included, so the package still compiles, and leave a same-named identifier for something else as it was. It SHALL refuse, at the position and writing nothing, a new name already declared, a method an interface in the package names, and a left-out file no build type-checks that spells the name.

#### Scenario: Methods in several files
- **WHEN** focus is toggled on a suite whose methods are spread over several files
- **THEN** refactor toggle-focus renames every receiver and the package still compiles

#### Scenario: A generic suite with aliases
- **WHEN** focus is toggled on a generic suite that aliases instantiate
- **THEN** refactor toggle-focus renames every generic receiver, alias and literal, and the package still compiles

#### Scenario: The external test package
- **WHEN** focus is toggled on a suite the external test package refers to
- **THEN** refactor toggle-focus renames the external test package's references too

#### Scenario: A reused name
- **WHEN** focus is toggled on a suite whose name another scope reuses
- **THEN** refactor toggle-focus leaves the identifiers of the other scope as they were

#### Scenario: An embedded suite
- **WHEN** focus is toggled on a suite another type embeds
- **THEN** refactor toggle-focus renames the selectors and literal keys that name the embedded field, and the package still compiles

#### Scenario: Files a build constraint leaves out
- **WHEN** focus is toggled on a suite that files behind a build constraint refer to
- **THEN** refactor toggle-focus renames their references too, and the package still compiles with the tag
- **AND** a reference in such a file that cannot be resolved is refused at its position and nothing is written

#### Scenario: A method an interface names
- **WHEN** focus is toggled on a method an interface in the package names
- **THEN** refactor toggle-focus refuses at the interface's position and writes nothing

#### Scenario: A name already declared
- **WHEN** focus is toggled on a suite or method whose new name is already declared
- **THEN** refactor toggle-focus refuses at the declaration that holds the name and writes nothing

### Requirement: The generated harness
The generated harness SHALL compile beside whatever names the package declares, except a name Go predeclares that the harness uses with Go's meaning, which SHALL be rejected at its declaration, and gotest generate SHALL keep it in step with the suites: it SHALL drop references to renamed suites and files of variants without suites, and SHALL NOT rewrite a file whose content is unchanged.

#### Scenario: Names the harness imports
- **WHEN** the package declares names the harness imports
- **THEN** a package declaring context, testing, gotest and the other imported names runs its suite
- **AND** a suite binding fixtures from two packages both named pg runs
- **AND** a package declaring a name Go predeclares that the harness uses is rejected at the declaration

#### Scenario: Generating again
- **WHEN** gotest generate runs again after the suites changed
- **THEN** a renamed suite leaves no reference to its old name, a variant without suites loses its file, and the run passes
- **AND** an unchanged generated file is not written again

### Requirement: Stale generated files and build constraints
A generated file left on disk by an earlier generate SHALL neither break nor be compiled into a run and SHALL be read past by discover and spec; --tags SHALL select the suite files built with that tag.

#### Scenario: A stale generated file
- **WHEN** a package holds a generated file from an earlier generate
- **THEN** a stale generated file on disk neither breaks the run nor is compiled
- **AND** discover and spec read past a stale generated file

#### Scenario: A build-constrained suite file
- **WHEN** a suite file carries a build constraint
- **THEN** --tags= selects the files built with that tag

### Requirement: Cases no run reaches
A method named as a case taking *gotest.T, *gotest.B or *gotest.F, declared in a _test.go file on a type that is no suite, SHALL be refused with exit 2 unless a test file of the package calls it or converts a value of the type to an interface holding it: one error per type, at its first such method, naming the suite suffix. A generic suite in a _test.go file SHALL be refused at its declaration unless a suite alias instantiates it. A type in a non-test file SHALL be left alone.

#### Scenario: Case-named methods on a type that is no suite
- **WHEN** a test file declares methods named as cases, taking a gotest T, B or F, on a type that is no suite
- **THEN** a type nothing calls them on is refused at its first such method, told which suffix makes it a suite
- **AND** a helper whose method a suite calls, directly or through an interface, runs
- **AND** a helper only the external test package calls runs
- **AND** a type a non-test file declares is left alone

#### Scenario: A helper a harness package calls
- **WHEN** a test file hands a helper to an interface a non-test package declares and calls
- **THEN** a helper converted to an interface that holds its method runs, by argument, assignment, return, composite literal or conversion
- **AND** a helper converted only to any or to an interface without the method is still refused

#### Scenario: A generic suite nothing instantiates
- **WHEN** a test file declares a generic suite
- **THEN** one no alias instantiates is refused at its declaration, told to declare an alias
- **AND** one an alias instantiates runs

### Requirement: Callers the build leaves out
A test file of the package that the build constraints leave out of the run SHALL count as a caller for the refusal of cases no run reaches: a case-named method whose name such a file selects, and a generic suite such a file names, SHALL NOT be refused, with the tag off or on. A helper that no file names, constrained or not, SHALL still be refused.

#### Scenario: A caller behind a build constraint
- **WHEN** the only caller of a helper, or the only alias of a generic suite, is in a test file a build constraint leaves out
- **THEN** a run with the build tag off runs the package
- **AND** a run on a platform the file is not for runs the package
- **AND** a run with the build tag on runs the constrained suites too
- **AND** a helper no file names is still refused

### Requirement: A case prefix alone
A suite method named exactly Test, Benchmark or Fuzz, an X_ or F_ marker aside, that takes the T, B or F of that case SHALL be refused at the method, told to add a name; one that takes no such parameter SHALL stay a helper.

#### Scenario: A method named by its prefix alone
- **WHEN** a suite declares a method named exactly Test, Benchmark or Fuzz
- **THEN** one taking the T, B or F of that case is refused at the method, told to add a name
- **AND** one taking no such parameter is a helper and the suite runs

### Requirement: Fixture per-method hooks under a parallel suite
gotest SHALL refuse with exit 2 a suite with Parallel: true and two or more test methods that binds, directly or through another fixture, a package fixture whose BeforeEach or AfterEach uses the fixture: parallel methods share the one fixture, so the hook would run for all of them at once. The error is located at the suite and names the hook, the fixture and the way out. A hook whose body never names its receiver, and a suite with one test method, SHALL run.

#### Scenario: A parallel suite binds a fixture with per-method hooks
- **WHEN** a suite with Parallel: true binds a package fixture that declares BeforeEach or AfterEach
- **THEN** a suite with two or more test methods whose fixture hook uses the fixture is refused at the suite, told to move the work into its returning BeforeEach or drop Parallel
- **AND** a suite whose fixture hooks never use the fixture runs
- **AND** a suite with one test method runs

### Requirement: Setenv under a parallel suite
gotest SHALL refuse with exit 2 a suite with Parallel: true whose test method, BeforeEach or AfterEach calls Setenv on a *gotest.T or a *testing.T, closures included: testing panics when a parallel test sets the environment. One error per suite, at the first call. It reads the method's own body, not the helpers it calls. A call in BeforeAll SHALL run.

#### Scenario: A parallel suite sets the environment
- **WHEN** a suite with Parallel: true calls Setenv on a T
- **THEN** a call in a test method, in BeforeEach, in a nested It and through t.T() is refused at the call, told to set it in BeforeAll or drop Parallel
- **AND** a call in BeforeAll runs and its value reaches every method

### Requirement: Shared fixture fields the state cannot carry
gotest SHALL refuse with exit 2, at the field and naming the way out, a shared fixture that embeds a type with exported fields unless Hydrate assigns it or every exported field it promotes (only named fields are transferred, so the suites would read zero values), and a transfer field whose type holds an interface with methods (JSON cannot rebuild its value). An embedded type without exported fields, a field Hydrate assigns, and a field of a type that refers to itself SHALL be accepted.

#### Scenario: A shared fixture embeds a type
- **WHEN** a shared fixture embeds a type
- **THEN** one with exported fields is refused at the field, told to hold it in a named field or assign it in Hydrate
- **AND** one without exported fields and one Hydrate assigns are left out, and the suite runs with its state

#### Scenario: Hydrate assigns an embedded struct's fields
- **WHEN** Hydrate assigns the fields a shared fixture's embedded struct promotes
- **THEN** a fixture whose Hydrate assigns every one of them runs with its state
- **AND** one whose Hydrate assigns only some is refused at the field, naming the fields it does not

#### Scenario: A field JSON cannot rebuild
- **WHEN** a shared fixture transfers a field whose type holds an interface with methods
- **THEN** the field is refused at the field, told to assign it in Hydrate or unexport it
- **AND** an interface field Hydrate assigns and a field of a type that refers to itself reach the suite
