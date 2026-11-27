# Scenarios Specification

## Purpose
How `gotest scenarios` reads the scenarios of OpenSpec spec files, binds each to the `When` and `It` labels a module's suites declare by their text alone, with no marker and no committed file, reports what is unbound or ambiguous, judges each bound scenario by a saved stream or a run of its own, writes the result as text, JSON and Markdown, and when it exits non-zero.

## Requirements

### Requirement: Reading the spec files
`gotest scenarios` SHALL read every `spec.md` under the module's `specs/` and `openspec/specs/`, or under the files and directories `--specs` or the `scenarios.specs` key names, the capability being its directory. Under a requirement each level-4 heading SHALL be a scenario and each GIVEN, WHEN, THEN or AND bullet a clause, bold, bold with a colon, underlined or bare in capitals; nothing inside a code fence and no REMOVED requirement SHALL be read.

#### Scenario: A module's default spec directories
- **WHEN** scenarios runs in a module with spec files under specs and openspec/specs and no --specs
- **THEN** reads the scenarios of both, each under the capability its directory names

#### Scenario: The clause forms OpenSpec writes
- **WHEN** a spec writes its clauses bold, bold with a colon, underlined, bare in capitals and wrapped onto a second line
- **THEN** reads the same clauses as from the plain bold form
- **AND** reads no heading or clause inside a code fence

#### Scenario: A change's delta specs
- **WHEN** --specs names a change's specs directory
- **THEN** reads its ADDED and MODIFIED requirements and none of its REMOVED ones

#### Scenario: The spec directories from the config
- **WHEN** .gotest.yml names a directory under scenarios.specs and no --specs is given
- **THEN** reads the spec files of that directory and no other

#### Scenario: No spec file to read
- **WHEN** scenarios finds no spec file
- **THEN** exits 2 naming the places it looked

### Requirement: Binding by text
A scenario SHALL bind when its GIVEN and WHEN texts, with their ANDs, equal in order the labels of a `When` and the `When`s directly around it, the last innermost, and each THEN and AND text after them equals an `It` directly under it, read from source without running a test. Texts SHALL compare after whitespace is collapsed, backticks, `**` and `__` are dropped, trailing `.`, `,`, `;` or `:` is dropped and the first letter's case is folded.

#### Scenario: A scenario its suite states
- **WHEN** a scenario's texts equal a When and its Its, but for the first letter's case, backticks and a trailing period
- **THEN** binds the scenario, naming the file and line of each clause's It, and exits 0 without running a test

#### Scenario: A scenario with a GIVEN
- **WHEN** a scenario has a GIVEN
- **THEN** binds it only where its When sits directly in a When labelled with the GIVEN text

#### Scenario: A scenario no suite states
- **WHEN** a scenario's WHEN text is no When's label, or a THEN text is no It's under it
- **THEN** lists the scenario as unbound, naming the clause that has no When or no It

#### Scenario: Texts outside ASCII
- **WHEN** two scenarios differ only in letters outside ASCII
- **THEN** binds each to the When that spells it and not to the other

#### Scenario: An It no clause can be
- **WHEN** the only It with a clause's text is under an Each row and not a When, or in a suite X_ excludes
- **THEN** lists the scenario as unbound

### Requirement: Ambiguity
A clause whose text equals an `It` at more than one declaration, or an `It` the clauses of two scenarios bind, SHALL make each scenario involved ambiguous, naming every declaration, and none of them SHALL be judged; the rows of one table and the runs of one declaration SHALL be one `It`.

#### Scenario: One clause, two Its
- **WHEN** a clause's text equals Its declared at two places under Whens with the scenario's text
- **THEN** lists the scenario as ambiguous, naming the file and line of both Its

#### Scenario: One It, two scenarios
- **WHEN** two capabilities hold a scenario with the same WHEN and THEN texts
- **THEN** lists both scenarios as ambiguous

#### Scenario: A clause under a table's rows
- **WHEN** a bound When sits under the rows of an Each table
- **THEN** binds each clause to one It, every row's run counting toward its verdict

### Requirement: Behaviors no clause states
With `--unstated`, the command SHALL list each `It` directly under a bound `When` whose label no clause states, with its file and line; without it, it SHALL list none.

#### Scenario: Asking for the unstated behaviors
- **WHEN** a bound When holds an It no clause states and --unstated is given
- **THEN** lists that It as unstated with its file and line

#### Scenario: Not asking for them
- **WHEN** a bound When holds an It no clause states and --unstated is not given
- **THEN** lists no unstated behavior

### Requirement: Evidence
With `--input=<events file>`, or `--verify`, which runs the packages that hold a bound `It` and no other, a bound scenario SHALL be verified when each clause's `It` passed with an assertion or an `AssertsBy`, failing when one failed, not-run when one was skipped, guarded or not reported, and vacuous when one passed with no assertion, each row and run of an `It` counted; the stream's manifest and packages SHALL name the run judged.

#### Scenario: Judging by a saved stream
- **WHEN** scenarios is given a saved stream with --input
- **THEN** lists each bound scenario as verified, failing, not-run or vacuous as its Its ran
- **AND** names the stream's run, its version and arguments and the packages it ran, as the run judged

#### Scenario: A skipped It
- **WHEN** a clause's It was skipped or its suite's guard kept it from running
- **THEN** lists the scenario as not-run, not verified

#### Scenario: Running the bound packages
- **WHEN** scenarios is given --verify
- **THEN** runs the packages that hold a bound It and no other, and judges the scenarios by that run

### Requirement: The scenarios document
With `--format=json` the command SHALL print one document opening with `version` and `schema` that holds the evidence, the run, the spec files, a summary and each scenario with its clauses and their `It`s, in file and line order, with module-relative paths. The text format SHALL list the scenarios not bound or verified first and end stdout with one `gotest scenarios:` closing line; `--format=md` SHALL write a table per capability.

#### Scenario: The document as JSON
- **WHEN** scenarios writes --format=json
- **THEN** prints one document opening with version and schema that holds evidence, run, specs, summary and scenarios, each with its clauses and their Its, with no absolute path

#### Scenario: The document as text
- **WHEN** scenarios writes the text format
- **THEN** lists the scenarios that are not bound or verified, each with the clause at fault, and ends stdout with a closing line naming the verdict, the exit code and the counts, with nothing on stderr

#### Scenario: The document as Markdown
- **WHEN** scenarios writes --format=md
- **THEN** writes a heading and a table of requirements and scenarios per capability

### Requirement: The policy exit
Without evidence the command SHALL exit 0 unless `--fail-on` names statuses, comma-separated, `all` for every one; with evidence `--fail-on` SHALL default to `failing,ambiguous`. It SHALL exit 1 when a scenario has a named status, and 2 for an unknown status, an evidence status without evidence, or a package that does not compile, unless the policy failed.

#### Scenario: Unbound scenarios without a policy
- **WHEN** scenarios finds unbound scenarios without evidence and without --fail-on
- **THEN** exits 0

#### Scenario: A failing scenario under the default policy
- **WHEN** the evidence holds a failing scenario and no --fail-on is given
- **THEN** exits 1 and the closing line names FAIL and the statuses failing and ambiguous

#### Scenario: A policy on the binding
- **WHEN** --fail-on=unbound is given and a scenario is unbound
- **THEN** exits 1

#### Scenario: A status it does not know
- **WHEN** --fail-on names an unknown status
- **THEN** refuses it with exit 2, naming the statuses

#### Scenario: An evidence status without evidence
- **WHEN** --fail-on names failing without --input or --verify
- **THEN** refuses it with exit 2, naming --input and --verify

#### Scenario: A package that does not compile
- **WHEN** a package of the module does not compile
- **THEN** names it and exits 2
