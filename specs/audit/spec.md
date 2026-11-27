# Audit Specification

## Purpose
How `gotest audit` records the test estate a tree declares without running it, how it compares the record of a head tree with that of a base, which facts it lists as weakenings, what the runs and the coverage given prove of the change, how it writes the comparison as text, JSON and Markdown, when it exits non-zero, how `--base` obtains a base from git, and what it does with a tree that does not compile.

## Requirements

### Requirement: The tree record
`gotest audit --record` SHALL print, without running a test, one JSON document opening with `version` and `schema` that lists per package each suite, test, When, It and table row with its display path, module-relative file and lines, body hash, the gotest assertion calls its body writes, `AssertsBy`, `F_` and `X_`; each suite's budgets and guard; each suppression and snapshot file; byte for byte the same for an unchanged tree.

#### Scenario: Recording a tree
- **WHEN** audit --record runs over a module
- **THEN** prints one document opening with version and schema that lists each suite, test, When, It and row with its display path, module-relative file and lines and body hash, and exits 0
- **AND** counts the gotest assertions written in each body, and marks AssertsBy, F_ and X_
- **AND** names each suite's budgets and guard, each suppression with its rules and reason, and each snapshot file with its hash

#### Scenario: Recording twice
- **WHEN** the same tree is recorded twice
- **THEN** the two records are byte for byte equal and hold no absolute path

### Requirement: Behaviors compared
`gotest audit` SHALL compare a base record with a head record and list each behavior the head added, removed, reworded (its label changed, its body and its behaviors not), moved (its parent changed, its label and body not) or changed (its body differs), with its base and head file and line. A rename that more than one behavior could explain SHALL be listed as removed and added, never guessed.

#### Scenario: Comparing two records
- **WHEN** audit compares a base record with a head record that adds, removes, rewords, moves and changes behaviors
- **THEN** lists each as added, removed, reworded, moved or changed with its base and head file and line

#### Scenario: An ambiguous rename
- **WHEN** two removed behaviors have the body of one added behavior
- **THEN** lists both as removed and the new one as added

### Requirement: What got weaker
The audit SHALL list, as a finding of its class with its subject and its base and head value, each fact that weakens the estate: a removed behavior, an `X_` or `F_` the head adds, a guard added, a budget raised or `NoDeadline` set, a suppression added (matched by module, file, rules and reason, not line), a snapshot rewritten or deleted, fewer assertions or an `AssertsBy` added; a changed body, and a suppression that only changed its reason, SHALL be listed apart, as information.

#### Scenario: A head that weakens the estate
- **WHEN** the head removes a test, excludes one, focuses one, adds a guard, raises a budget, sets NoDeadline, adds a suppression, rewrites and deletes a snapshot, drops an assertion and adds AssertsBy
- **THEN** lists a finding of each class with its subject and its base and head values
- **AND** lists each changed body as an informational finding, apart from the weakenings

#### Scenario: A suppression that moved
- **WHEN** a suppression moves to another line with its rules and reason
- **THEN** lists no suppression finding for it

#### Scenario: A suppression with a new reason
- **WHEN** a suppression keeps its file and rules and gives a new reason
- **THEN** lists it as an informational suppression finding naming both comments

### Requirement: The audit document
With `--format=json` the audit SHALL print one document opening with `version` and `schema` that holds both records' provenance, a summary of counts, the findings, the behaviors, the streams read and the `evidence` and `unproven` lists, empty without a stream or a profile. The default text format SHALL print the weakenings before the behaviors and end with one `gotest audit:` closing line; `--format=md` SHALL write the same document as Markdown.

#### Scenario: The document as JSON
- **WHEN** audit compares two records with --format=json
- **THEN** prints one document opening with version and schema that holds base, head, summary, findings, behaviors and empty evidence and unproven lists

#### Scenario: The document as text
- **WHEN** audit compares two records in the text format
- **THEN** prints the weakenings before the behaviors and ends stdout with a closing line naming the verdict, the exit code and the counts, with nothing on stderr

#### Scenario: The document as Markdown
- **WHEN** audit compares two records with --format=md
- **THEN** writes a heading, a table of the weakenings and a table of the behaviors

### Requirement: The policy exit
The audit SHALL exit 0 whatever it finds unless `--fail-on` names finding classes, comma-separated, `all` for every weakening; it SHALL then exit 1 when the document holds a finding of a named class. A class it does not know SHALL be refused with exit 2.

#### Scenario: Weakenings without a policy
- **WHEN** audit finds weakenings and is given no --fail-on
- **THEN** exits 0

#### Scenario: A policy the document breaks
- **WHEN** --fail-on names a class the document lists
- **THEN** exits 1 and the closing line names FAIL and the classes

#### Scenario: An unknown class
- **WHEN** --fail-on names an unknown class
- **THEN** audit refuses it with exit 2, naming the classes

### Requirement: A base from git
`gotest audit --base=<ref>` SHALL record the tree of the commit ref names, exported by git outside the checkout, from the same directory and with the same packages, none in a directory the commit lacks, and compare it with the working tree, leaving the checkout's worktree, index and worktree list as they were. Where git or the commit is missing it SHALL exit 2, naming the flags that take records.

#### Scenario: A commit of the checkout
- **WHEN** audit --base names a commit of the checkout
- **THEN** compares that commit's tree with the working tree's, as the records of both would
- **AND** leaves the checkout's status, index and worktree list as they were

#### Scenario: A commit without a directory the packages name
- **WHEN** audit --base names a commit that lacks a directory one of the packages names
- **THEN** records no package of that directory for the base and compares the rest, exiting 0

#### Scenario: A ref the checkout lacks
- **WHEN** audit --base names no commit of the checkout
- **THEN** exits 2 naming --base-record and --head-record

### Requirement: Evidence from runs
`gotest audit --evidence=<stream>`, repeatable, SHALL state for each behavior the comparison lists as added, reworded, moved or changed and each finding's behavior its verdict, the assertions the run counted, whether it passed vacuous, its `AssertsBy` and the stream it was judged by, reading the stream's test names as the spec tree does. A behavior the stream does not report SHALL read as not run, skipped where X_ excludes it, and as no evidence where no stream holds its package.

#### Scenario: A stream of the head's run
- **WHEN** audit is given a stream of the head's run
- **THEN** states for each added, reworded, moved and changed behavior and each finding's behavior its verdict, its run-time assertions, whether it passed vacuous and its AssertsBy
- **AND** names a behavior the stream did not report not run, one X_ excludes skipped, and one of a package no stream holds no evidence
- **AND** lists the stream with its manifest's CLI version, commit, dirty flag, agent and exit code

### Requirement: Current and stale evidence
Evidence SHALL be current when its stream ran the head's commit with neither tree holding uncommitted changes, or when the audit ran it; else stale, naming why: another commit, uncommitted changes, or no manifest. Of the streams that judge one behavior the audit SHALL take a current one before a stale one, then the first given, and name the others' verdicts.

#### Scenario: Streams of other trees
- **WHEN** audit is given streams of another commit, of a tree with uncommitted changes and without a manifest
- **THEN** marks each stale, naming another commit, uncommitted changes and no manifest

#### Scenario: Two streams of one behavior
- **WHEN** a current and a stale stream judge one behavior
- **THEN** states the current stream's verdict and names the stale one's among the others

### Requirement: Assertions at run time
With `--base-evidence=<stream>`, a stream of the base's run, a finding of class `assertions` SHALL carry, beside the counts written in source, the count each side's run reported.

#### Scenario: A drop both runs saw
- **WHEN** audit is given a stream of each side's run and the head drops an assertion
- **THEN** the assertions finding carries the base's and the head's run-time counts

### Requirement: The audit's own run
`gotest audit --run` SHALL run, through gotest's runner, the packages holding a behavior the comparison names and, when the changed lines are known, the changed production packages, measuring their coverage, and take that run as current evidence. Where no package holds one it SHALL run nothing and say so.

#### Scenario: Running what changed
- **WHEN** audit --run compares the working tree with a base
- **THEN** runs the packages of the changed behaviors and states their verdicts as current evidence of that run
- **AND** lists the changed production lines that run did not execute

### Requirement: Unproven lines
Given a coverage profile, `--coverage` or the one `--run` measured, and the changed lines, from `--diff` or from git between `--base` and the files on disk with untracked files whole, the audit SHALL list per changed non-test file of the packages audited the changed lines a block spans and none executed, as ranges with counts. A line no block spans SHALL not count; a file of a package no block covers SHALL read as without coverage data.

#### Scenario: A profile and a diff
- **WHEN** audit is given a coverage profile and a diff that changes production lines
- **THEN** lists per file the changed executable lines no test executed as ranges, and counts them in the summary
- **AND** counts no changed line that no block of the profile spans
- **AND** lists a changed file of a package the profile does not cover as without coverage data

#### Scenario: Lines from git
- **WHEN** audit --base with --coverage runs in a checkout with uncommitted and untracked production files
- **THEN** takes the changed lines from the commit to the files on disk, every line of an untracked file included

### Requirement: Proof in the policy
`--fail-on` SHALL take the proof classes `failing`, `not-run`, `vacuous`, `stale` and `unproven`, and `proof` for the five; `all` SHALL stay every weakening, without them, for they judge the evidence given, not the change. A proof class with nothing to judge SHALL be refused with exit 2. The closing line SHALL count the evidence and the unproven lines.

#### Scenario: A policy on the proof
- **WHEN** --fail-on names failing and a stream fails a behavior the comparison names
- **THEN** exits 1 and the closing line counts the evidence and names the class
- **AND** --fail-on=all names every weakening class and no proof class

#### Scenario: A proof class without evidence
- **WHEN** --fail-on names a proof class and no stream or profile is given
- **THEN** audit refuses it with exit 2, naming the flags that give it

### Requirement: Evidence in the renderings
Text and Markdown SHALL mark each behavior and weakening with its verdict, a passing leaf's run-time assertions and `*` for a stale stream, name the streams read, list removed, weakened, failing, vacuous, not run and stale behaviors first, and list the unproven lines. Text SHALL fold five or more rows alike of one package and suite into one line, a failing row never, unless `-v` is given; JSON SHALL keep every row.

#### Scenario: Evidence in text
- **WHEN** audit renders evidence in the text format
- **THEN** marks each row with its verdict and names each stream read under the first line
- **AND** lists the removed, weakened and failing behaviors before the others

#### Scenario: Rows alike
- **WHEN** the text format lists five or more rows alike of one suite
- **THEN** folds them into one line that counts them, and -v lists each

#### Scenario: Evidence in Markdown
- **WHEN** audit renders evidence with --format=md
- **THEN** writes the evidence as a column of the behaviors table and the unproven lines as a table

### Requirement: Where a base is exported
`--base` SHALL export the commit into a directory under gotest's cache fixed per checkout, never inside the checkout, under a lock one audit of the checkout holds at a time, exporting again only when the directory holds another commit; `gotest clean --cache` SHALL remove the directories no audit holds.

#### Scenario: Two audits of one commit
- **WHEN** audit --base records one commit twice
- **THEN** exports it once into one directory under gotest's cache, outside the checkout
- **AND** gotest clean --cache removes that directory

### Requirement: What a tool reads from the document
In the JSON document each finding and evidence row SHALL carry an `id` made from what it is about, never its order or lines; a finding SHALL carry each side's location where it has one, and its module; a finding, behavior or evidence row about a test SHALL name the go test name and `-run` pattern that select it; the document SHALL give each module's directory and each side's branch; an evidence row SHALL say whether it needs a decision, and its stream and commit.

#### Scenario: A tool reading the document
- **WHEN** a tool reads the audit document as JSON
- **THEN** finds on each finding and evidence row an id that stays when the lines move
- **AND** finds on a finding each side's location, and the module of one without a package
- **AND** finds on a finding, behavior and evidence row about a test the go test name and -run pattern that select it
- **AND** finds each module's directory relative to the audited one and each side's branch
- **AND** finds on each evidence row whether it needs a decision, the stream it was judged by and that stream's commit

### Requirement: A tree that does not compile
A record SHALL list a package that does not compile with its errors and no suites, and exit 2; the audit SHALL compare none of that package's behaviors, name it as not compared and exit 2 unless the policy failed.

#### Scenario: A head package that does not compile
- **WHEN** the head tree holds a package that does not compile
- **THEN** audit --record lists the package with its errors and no suites and exits 2
- **AND** audit compares none of its behaviors, names it as not compared and exits 2
