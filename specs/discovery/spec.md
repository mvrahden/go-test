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
