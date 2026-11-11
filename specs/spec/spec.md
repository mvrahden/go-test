# Living Spec Specification

## Purpose
How `gotest spec` writes the behavior specification of a run, and the one it reads from source with `--static`, as a Markdown outline whose labels stay text and whose counts add up; which verdict a branch and its package carry when a failure lies beneath them; how --declared lays a stream's verdicts over the specification the source declares and where the source declares each node; and which rows --hide leaves out of a rendering without changing what it counts.

## Requirements

### Requirement: Labels are text
Every label the Markdown spec writes, in a list item or a heading, SHALL be escaped so that no pipe, backslash, angle bracket, backtick or line break in it can break its line, open a code span or reach the reader as HTML, no bracket, asterisk, underscore or ampersand can open a link, emphasis or an entity, and none at the start of an item can open a list, a heading, a quote or a code block.

#### Scenario: A label that holds Markdown syntax
- **WHEN** a label holds a pipe, a backslash, an angle bracket, a backtick or a line break
- **THEN** keeps each behavior on one line of the outline
- **AND** renders the label as text, with no HTML tag and no code span

#### Scenario: A label that opens a block
- **WHEN** a label opens a list, a heading, a quote or a code block
- **THEN** the Markdown spec shows it as text

#### Scenario: A label that holds link or emphasis syntax
- **WHEN** a label holds brackets, asterisks, underscores or an ampersand
- **THEN** renders the label as written, with no link, emphasis or entity

### Requirement: A slash in a label
An It or a When whose label holds a slash, at its start, inside or at its end, SHALL be one node under its parent, in the spec of a run, in the spec read from source and in the spec replayed from a run's -json stream, and discover SHALL report it as one behavior, because go test reports one subtest for it; an It whose label begins with another It's label and a slash SHALL be that It's sibling, also when that It runs in parallel.

#### Scenario: An It with a slash
- **WHEN** an It's label holds a slash
- **THEN** a run's spec shows it as one It under its When
- **AND** the spec read from source shows it as one It under its When

#### Scenario: An It whose label extends another's
- **WHEN** an It's label is an earlier It's label, a slash and more
- **THEN** a run's spec shows both as Its under their When
- **AND** the spec read from source shows both as Its under their When

#### Scenario: An It that starts with a slash
- **WHEN** an It's label starts with a slash
- **THEN** a run's spec shows it as one It under its When
- **AND** the spec read from source shows it as one It under its When
- **AND** the spec replayed from the run's -json stream shows it as one It under its When
- **AND** discover reports it as one behavior

#### Scenario: An It that ends with a slash
- **WHEN** an It's label ends with a slash
- **THEN** a run's spec shows it as one It under its When
- **AND** the spec read from source shows it as one It under its When
- **AND** the spec replayed from the run's -json stream shows it as one It under its When
- **AND** discover reports it as one behavior

#### Scenario: A When that ends with a slash
- **WHEN** a When's label ends with a slash and holds Its
- **THEN** a run's spec shows its Its under it
- **AND** the spec read from source shows its Its under it

#### Scenario: An It that extends a parallel sibling's label
- **WHEN** an It's label extends a parallel sibling's label
- **THEN** a run's spec shows both as Its under their When

### Requirement: Suites of both test packages
A suite name declared in a package's test files and in its external test package SHALL read as two suites, the external one marked, each with its own behaviors, in a run's spec, in the spec replayed from its -json stream, in the declared spec and in discover, the internal one first, as go test runs it first.

#### Scenario: A suite declared twice
- **WHEN** a suite is declared in both test packages
- **THEN** a run's spec shows two suites, the external one marked
- **AND** each shows only its own methods
- **AND** the spec replayed from the run's -json stream shows the same two suites
- **AND** the declared spec under the run shows every behavior with its verdict
- **AND** discover marks the external suite

#### Scenario: The order go test runs them in
- **WHEN** go test runs a suite of both test packages
- **THEN** the internal suite runs first

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
`spec --input=<file> --declared [packages]` SHALL render the specification the source declares, as --static reads it: a node the stream reported carries its verdict, duration and output, one it did not report neither, or skipped under a node the run skipped; what only the stream holds stays under its declared parent. A behavior with no verdict, unreported or only started, SHALL count as not run, with no duration, sparing the exit code, so the counts add up to the behaviors.

#### Scenario: A run of some of a package's methods
- **WHEN** a stream of some of a package's methods is rendered with --declared
- **THEN** shows every method the source declares with its When and It rows, those the stream did not report with no verdict and no duration
- **AND** counts the behaviors the stream did not report as not run, and exits as without --declared
- **AND** reads a method the X_ prefix excludes, and its rows, as skipped rather than not run

#### Scenario: Rows only a run can name
- **WHEN** a stream holds the rows of an Each whose table is not a literal, rendered with --declared
- **THEN** shows the rows under their method, which is marked incomplete

#### Scenario: A suite its guard kept from running
- **WHEN** a stream holds the skip of a suite its guard kept from running, rendered with --declared
- **THEN** reads the suite's declared methods and rows as skipped, not as not run, and counts them so

#### Scenario: A behavior without a verdict
- **WHEN** a behavior started and reported no verdict
- **THEN** counts as not run
- **AND** shows no duration
- **AND** the counts add up to the behaviors

### Requirement: Where the source declares a node
With --static or --declared each JSON package SHALL carry its dir and each node the source declares its file and line.

#### Scenario: Where the source declares a node
- **WHEN** a package's specification is rendered as JSON with --static or with --declared
- **THEN** gives the package its dir and each node the source declares the file and line gotest discover gives it, and a node only the stream holds neither

### Requirement: Table rows named by constants
The specification read from source SHALL name an Each table row as the run does: by its Desc field, else its Name field, each when it is a string and not empty, else by its index, and a constant the type checker evaluates, such as one imported from another package, names a row by its value. A row whose name only a run can know SHALL not be listed, and its method SHALL be marked incomplete; under --declared the row the run reported SHALL show with its verdict.

#### Scenario: A row named by a constant
- **WHEN** a table row is named by a constant
- **THEN** the spec read from source names the row by the constant's value
- **AND** the declared spec under the run shows the row with its verdict

#### Scenario: A row named at run time
- **WHEN** a table row's name is known only at run time
- **THEN** the spec read from source lists no row for it and marks its method incomplete
- **AND** the declared spec under the run shows the row the run reported

### Requirement: A report a person reads
The Markdown spec SHALL be an outline: a header with the counts, the runs' time range, the checkout's branch and commit and its uncommitted changes, and what is hidden; then every failure with its path and message and every row not run with its path; then a heading per module, package and suite, each package and suite with its counts and an external suite marked EXTERNAL; then each behavior as one item that opens with its verdict. No duration under a second SHALL be shown.

#### Scenario: A run with failures and rows not run
- **WHEN** the Markdown spec of a run with failures and rows not run
- **THEN** the header says the time range of the runs and the commit
- **AND** lists every failure with its path and message before the packages
- **AND** lists every row not run with its path
- **AND** every package and suite heading carries its counts
- **AND** an external suite's heading is marked EXTERNAL
- **AND** no duration under a second is shown

#### Scenario: A report that hides rows
- **WHEN** the Markdown spec hides rows
- **THEN** the header names what is hidden

### Requirement: A terminal report a person reads
The terminal spec SHALL carry the Markdown spec's content laid out for a terminal: a heading per package and a line per suite, each with its counts, each behavior as one line led by its mark, then every failure and row not run with its path and message after the packages, then the provenance line. A label SHALL wrap at the terminal's width, COLUMNS or 100, and SHALL stay on one line when the spec is redirected.

#### Scenario: A run with failures
- **WHEN** the terminal spec of a run with failures
- **THEN** lists every failure with its path and message after the packages
- **AND** every package and suite line carries its counts

#### Scenario: A terminal
- **WHEN** the terminal spec is written to a terminal
- **THEN** long labels wrap at its width

#### Scenario: A redirected spec
- **WHEN** the terminal spec is redirected
- **THEN** every behavior stays on one line

### Requirement: Hiding rows by status
`--hide=<status>[,<status>]` SHALL leave out of every format each row whose own status it names (pass, fail, skip, none) and beneath which no row is shown, a package's own failure under fail; `--hide=` SHALL hide nothing. Counts and the exit code SHALL stay the whole stream's. With --hide or --declared every JSON package SHALL carry its stats whatever is hidden, else none SHALL. --hide under --static or with an unknown status, and --declared without --input or beside --static, SHALL exit 2.

#### Scenario: Hiding what passed
- **WHEN** a failing stream is rendered with --hide=pass
- **THEN** keeps the failing branch, its failing leaf and a suite that failed in its own hooks, and leaves out the rows that passed
- **AND** counts every behavior that passed in the trailer and in its package's stats, as without --hide

#### Scenario: Hiding a green run
- **WHEN** a stream in which everything passed is rendered with --hide=pass
- **THEN** renders the trailer and no row
- **AND** keeps every package in the JSON document with all of its stats and none of its rows

#### Scenario: Hiding a package's own failure
- **WHEN** a stream whose package failed to build is rendered with --hide=fail
- **THEN** leaves the package's failure and its output out of every format, its stats still counting it

#### Scenario: A view that cannot apply
- **WHEN** --hide is given with --static or with a status it does not know, or --declared without --input or with --static
- **THEN** refuses with exit 2, naming what it needs or the statuses it accepts
