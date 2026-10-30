# Living Spec Specification

## Purpose
How `gotest spec` writes the behavior specification of a run, and the one it reads from source with `--static`, as a Markdown document whose labels stay text; which verdict a branch and its package carry when a failure lies beneath them; how --declared lays a stream's verdicts over the specification the source declares and where the source declares each node; and which rows --hide leaves out of a rendering without changing what it counts.

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

### Requirement: One label on every surface
A behavior SHALL read the same in discover, in a run's spec, in the spec read from source and in the spec replayed from a run's -json stream: its label as the developer wrote it, underscores included, a When's behind the word when, which SHALL be written once, also for a label that opens with a connective of its own.

#### Scenario: Labels with underscores
- **WHEN** a When and its Its hold underscores in their labels
- **THEN** discover reports each label as written, the When's behind the word when
- **AND** a run's spec renders each behavior under the label discover reports
- **AND** the spec read from source renders each behavior under that label
- **AND** the spec replayed from the run's -json stream renders each behavior under that label

#### Scenario: A When that opens with a connective
- **WHEN** a When's label opens with a connective of its own
- **THEN** discover and every spec write it with no when in front

### Requirement: A failure below fails the branch
A node SHALL be failed when a node beneath it failed, and a package SHALL be failed when one of its nodes failed, whatever the stream's own line for the node or the package says, so a stream composed from several runs renders, counts and exits as one go test failed.

#### Scenario: A pass line above a failed method
- **WHEN** a stream holds a suite's pass line above a fail line of one of its methods
- **THEN** renders the suite and its package as failed in the terminal, Markdown and JSON formats, as when their own lines say fail
- **AND** counts one failure and exits 1, or 0 with --render-only

### Requirement: The declared specification under a run's verdicts
`spec --input=<file> --declared [packages]` SHALL render the specification the source declares, as --static reads it, each node the stream reported carrying its verdict, duration and output and each other declared node neither, and SHALL keep what only the stream holds under its declared parent. Behaviors the stream did not report SHALL count as not run and SHALL NOT change the exit code. With --static or --declared each JSON package SHALL carry its dir and each declared node its file and line.

#### Scenario: A run of some of a package's methods
- **WHEN** a stream of some of a package's methods is rendered with --declared
- **THEN** shows every method the source declares with its When and It rows, those the stream did not report with no verdict and no duration
- **AND** counts the behaviors the stream did not report as not run, and exits as without --declared
- **AND** reads a method the X_ prefix excludes, and its rows, as skipped rather than not run

#### Scenario: Rows only a run can name
- **WHEN** a stream holds the rows of an Each whose table is not a literal, rendered with --declared
- **THEN** shows the rows under their method, which is marked incomplete

#### Scenario: Where the source declares a node
- **WHEN** a package's specification is rendered as JSON with --static or with --declared
- **THEN** gives the package its dir and each node the source declares the file and line gotest discover gives it, and a node only the stream holds neither

### Requirement: Hiding rows by status
`--hide=<status>[,<status>]` SHALL leave out of every format each row whose own status it names (pass, fail, skip, none) and beneath which no row is shown, and SHALL leave every count, every package's stats and the exit code those of the whole stream. With --hide or --declared each JSON package SHALL carry its own stats. --hide under --static or with an unknown status, and --declared without --input or beside --static, SHALL exit 2.

#### Scenario: Hiding what passed
- **WHEN** a failing stream is rendered with --hide=pass
- **THEN** keeps the failing branch, its failing leaf and a suite that failed in its own hooks, and leaves out the rows that passed
- **AND** counts every behavior that passed in the trailer and in its package's stats, as without --hide

#### Scenario: Hiding a green run
- **WHEN** a stream in which everything passed is rendered with --hide=pass
- **THEN** renders the trailer and no row

#### Scenario: A view that cannot apply
- **WHEN** --hide is given with --static or with a status it does not know, or --declared without --input or with --static
- **THEN** refuses with exit 2, naming what it needs or the statuses it accepts
