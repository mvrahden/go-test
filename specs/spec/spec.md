# Living Spec Specification

## Purpose
How `gotest spec` writes the behavior specification of a run, and the one it reads from source with `--static`, as a Markdown document whose labels stay text.

## Requirements

### Requirement: Labels are text
Every label the Markdown spec writes, in a table cell or a heading, SHALL be escaped so that no pipe, backslash, angle bracket, backtick or line break in it can end a cell, break a row, open a code span or reach the reader as HTML, and no bracket, asterisk, underscore or ampersand can open a link, emphasis or an entity.

#### Scenario: A label that holds Markdown syntax
- **WHEN** a label holds a pipe, a backslash, an angle bracket, a backtick or a line break
- **THEN** keeps each behavior on one row of its table
- **AND** renders the label as text, with no HTML tag and no code span

#### Scenario: A label that holds link or emphasis syntax
- **WHEN** a label holds brackets, asterisks, underscores or an ampersand
- **THEN** renders the label as written, with no link, emphasis or entity

### Requirement: A slash in a label
An It whose label holds a slash SHALL be one It under its When, in the spec of a run and in the spec read from source, because go test reports one subtest for it; an It whose label begins with another It's label and a slash SHALL be that It's sibling.

#### Scenario: An It with a slash
- **WHEN** an It's label holds a slash
- **THEN** a run's spec shows it as one It under its When
- **AND** the spec read from source shows it as one It under its When

#### Scenario: An It whose label extends another's
- **WHEN** an It's label is an earlier It's label, a slash and more
- **THEN** a run's spec shows both as Its under their When
- **AND** the spec read from source shows both as Its under their When

### Requirement: A group's own failure
A When or an It that fails while every behavior under it passes SHALL show, on its own row of a run's spec, that it failed and what it wrote.

#### Scenario: A group that fails on its own
- **WHEN** a group fails with every behavior under it passing
- **THEN** shows the failure and what it wrote on the group's row

### Requirement: Excluded suites and methods
The Markdown spec read from source SHALL mark each suite and method the X_ prefix excludes, and every behavior under them, EXCLUDED.

#### Scenario: X_ suites and methods read from source
- **WHEN** the spec is read from source with X_ suites and methods
- **THEN** marks each excluded suite, method and behavior EXCLUDED
