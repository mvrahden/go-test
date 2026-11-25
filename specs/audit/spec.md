# Audit Specification

## Purpose
How `gotest audit` records the test estate a tree declares without running it, how it compares the record of a head tree with that of a base, which facts it lists as weakenings, how it writes the comparison as text, JSON and Markdown, when it exits non-zero, how `--base` obtains a base from git, and what it does with a tree that does not compile.

## Requirements

### Requirement: The tree record
`gotest audit --record` SHALL print, without running a test, one JSON document opening with `version` and `schema` that lists per package each suite, test, When, It and table row with its display path, module-relative file and lines, body hash, the gotest assertion calls its body writes, `AssertsBy`, `F_` and `X_`; each suite's budgets and guard; each suppression and snapshot file; byte for byte the same for an unchanged tree.

#### Scenario: Recording a tree
- **WHEN** audit --record runs over a module
- **THEN** prints one document opening with version and schema that lists each suite, test, When, It and row with its display path, module-relative file and lines and body hash, and exits 0
- **AND** counts the gotest assertions written in each body, those on t.Soft() included, and marks AssertsBy, F_ and X_
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
The audit SHALL list, as a finding of its class with its subject and its base and head value, each fact that weakens the estate: a removed behavior, an `X_` or `F_` the head adds, a guard added, a budget raised or `NoDeadline` set, a suppression added (matched by module, file, rules and reason, not line), a snapshot file rewritten or deleted, fewer assertions or an `AssertsBy` added; a changed body SHALL be listed apart, as information.

#### Scenario: A head that weakens the estate
- **WHEN** the head removes a test, excludes one, focuses one, adds a guard, raises a budget, sets NoDeadline, adds a suppression, rewrites and deletes a snapshot, drops an assertion and adds AssertsBy
- **THEN** lists a finding of each class with its subject and its base and head values
- **AND** lists each changed body as an informational finding, apart from the weakenings

#### Scenario: A suppression that moved
- **WHEN** a suppression moves to another line with its rules and reason
- **THEN** lists no suppression finding for it

### Requirement: The audit document
With `--format=json` the audit SHALL print one document opening with `version` and `schema` that holds both records' provenance, a summary of counts, the findings, the behaviors and the `evidence` and `unproven` lists, empty in this version. The default text format SHALL print the weakenings before the behaviors and end with one `gotest audit:` closing line; `--format=md` SHALL write the same document as Markdown.

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
`gotest audit --base=<ref>` SHALL record the tree of the commit ref names, exported by git outside the checkout, from the same directory and with the same packages, and compare it with the working tree, leaving the checkout's worktree, index and worktree list as they were. Where git or the commit is missing it SHALL exit 2, naming the flags that take records.

#### Scenario: A commit of the checkout
- **WHEN** audit --base names a commit of the checkout
- **THEN** compares that commit's tree with the working tree's, as the records of both would
- **AND** leaves the checkout's status, index and worktree list as they were

#### Scenario: A ref the checkout lacks
- **WHEN** audit --base names no commit of the checkout
- **THEN** exits 2 naming --base-record and --head-record

### Requirement: A tree that does not compile
A record SHALL list a package that does not compile with its errors and no suites, and exit 2; the audit SHALL compare none of that package's behaviors, name it as not compared and exit 2 unless the policy failed.

#### Scenario: A head package that does not compile
- **WHEN** the head tree holds a package that does not compile
- **THEN** audit --record lists the package with its errors and no suites and exits 2
- **AND** audit compares none of its behaviors, names it as not compared and exits 2
