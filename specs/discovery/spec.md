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
An alias of a suite, at any depth and as a method receiver, SHALL stand for the suite it names; a defined type over a suite, a Test method that returns a value and a shared fixture declared in a _test.go file SHALL be rejected where they are written.

#### Scenario: Aliases and unusual signatures
- **WHEN** a suite is declared through an alias or with an unusual signature
- **THEN** a method declared on a pointer to an alias of the suite runs
- **AND** an alias of an alias of a generic suite runs its methods
- **AND** a defined type over a suite is rejected at its declaration and pointed to an alias
- **AND** a Test method that returns a value is rejected at the method
- **AND** a shared fixture declared in a _test.go file is rejected at its declaration

### Requirement: Fixture fields and case names
A suite SHALL bind each fixture once, through a pointer field or an alias of one, and a fixture-named type no suite reaches SHALL NOT fail the package, and a method SHALL be a case only by a name go test would run.

#### Scenario: Fields and method names
- **WHEN** a suite names its fixtures and methods
- **THEN** a fixture field by value is rejected at the field and told to take a pointer
- **AND** a fixture field typed by an alias of the fixture pointer binds the fixture
- **AND** two fields of the same fixture type are rejected at the second field
- **AND** a fixture-named type no suite reaches cannot fail the package
- **AND** a method whose name continues Test with a lowercase letter is not a case, as under go test

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
