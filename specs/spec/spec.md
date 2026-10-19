# Living Spec Specification

## Purpose
How `gotest spec` writes the behavior specification of a run, and the one it reads from source with `--static`, as a Markdown document whose labels stay text.

## Requirements

### Requirement: Labels are text
Every label the Markdown spec writes, in a table cell or a heading, SHALL be escaped so that no pipe, backslash, angle bracket, backtick or line break in it can end a cell, break a row, open a code span or reach the reader as HTML.

#### Scenario: A label that holds Markdown syntax
- **WHEN** a label holds a pipe, a backslash, an angle bracket, a backtick or a line break
- **THEN** keeps each behavior on one row of its table
- **AND** renders the label as text, with no HTML tag and no code span

### Requirement: A slash in a label
An It whose label holds a slash SHALL be one It under its When, in the spec of a run and in the spec read from source, because go test reports one subtest for it.

#### Scenario: An It with a slash
- **WHEN** an It's label holds a slash
- **THEN** a run's spec shows it as one It under its When
- **AND** the spec read from source shows it as one It under its When

### Requirement: Excluded suites and methods
The Markdown spec read from source SHALL mark each suite and method the X_ prefix excludes, and every behavior under them, EXCLUDED.

#### Scenario: X_ suites and methods read from source
- **WHEN** the spec is read from source with X_ suites and methods
- **THEN** marks each excluded suite, method and behavior EXCLUDED
