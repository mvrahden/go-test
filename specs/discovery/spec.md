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

### Requirement: Case-less suites
A type named like a suite that declares no case SHALL be treated as a helper: nothing SHALL be generated, run, reported or discovered for it.

#### Scenario: A suite with no case
- **WHEN** a suite declares no case
- **THEN** the run passes with the real suite's methods
- **AND** nothing is generated or reported for the case-less suite
- **AND** discover does not list the case-less suite

### Requirement: Declarations from other packages
A suite SHALL run an alias of a generic suite another package declares, an alias declared in the external test package SHALL be discovered and run whether the package under test, the external test package or a third package declares the generic suite, and a fixture a suite binds from another package SHALL be held to the signature rules a local fixture is held to.

#### Scenario: Across packages
- **WHEN** a suite reaches across packages
- **THEN** an alias of a generic suite from another package runs that suite's methods
- **AND** a package fixture from another package with a mis-signed hook is rejected with the signature it needs

#### Scenario: Generic aliases in the external test package
- **WHEN** the external test package declares an alias of a generic suite
- **THEN** an alias of a generic suite the package under test declares runs that suite's methods
- **AND** an alias of a generic suite the external test package declares runs that suite's methods
- **AND** an alias of a generic suite from a third package runs that suite's methods

### Requirement: Focus across a run
Focus SHALL be decided over the whole run: a focused suite or a focused method of any kind, Test, Benchmark or Fuzz, in any package or test package variant SHALL skip every unfocused suite, which then starts no fixture, and --ci SHALL still refuse a run with a focused suite or Test method.

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

### Requirement: The generated harness
The generated harness SHALL compile beside whatever names the package declares, and gotest generate SHALL keep it in step with the suites: it SHALL drop references to renamed suites, files of variants without suites and the files an older gotest generated under its legacy names, and SHALL NOT rewrite a file whose content is unchanged.

#### Scenario: Names the harness imports
- **WHEN** the package declares names the harness imports
- **THEN** a package declaring context, testing, gotest and the other imported names runs its suite
- **AND** a suite binding fixtures from two packages both named pg runs

#### Scenario: Generating again
- **WHEN** gotest generate runs again after the suites changed
- **THEN** a renamed suite leaves no reference to its old name, a variant without suites loses its file, and the run passes
- **AND** a generated file an older gotest left under its legacy name is removed
- **AND** an unchanged generated file is not written again

### Requirement: Stale generated files and build constraints
A generated file left on disk by an earlier generate, under its current or legacy name, SHALL neither break nor be compiled into a run and SHALL be read past by discover and spec; --tags SHALL select the suite files built with that tag.

#### Scenario: A stale generated file
- **WHEN** a package holds a generated file from an earlier generate
- **THEN** a stale generated file on disk neither breaks the run nor is compiled
- **AND** discover and spec read past a stale generated file
- **AND** a legacy generated file name is treated the same

#### Scenario: A build-constrained suite file
- **WHEN** a suite file carries a build constraint
- **THEN** --tags= selects the files built with that tag
