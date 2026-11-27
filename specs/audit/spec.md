# Audit Specification

## Purpose
How `gotest audit` records the test estate a tree declares without running it, how it compares the record of a head tree with that of a base, which facts it lists as weakenings, what the runs and the coverage given prove of the change, whether a test notices a changed line made wrong, how it writes the comparison as text, JSON and Markdown, when it exits non-zero, how `--base` obtains a base from git, and what it does with a tree that does not compile.

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
Evidence SHALL be current when the audit ran it, or when its stream stamped the package's verdict with an InputHash the head's working tree still has; stale when the hash differs, naming the package's inputs changed. A stream without a hash for the package, or a head given as a record, SHALL be judged by commit: current when it ran the head's commit with neither tree dirty, else stale, naming another commit, uncommitted changes, or no manifest.

#### Scenario: Streams of other trees
- **WHEN** audit is given streams of another commit, of a tree with uncommitted changes and without a manifest
- **THEN** marks each stale, naming another commit, uncommitted changes and no manifest

#### Scenario: A stream with input hashes
- **WHEN** audit is given a stream with input hashes after an uncommitted change to one of its packages and to a file no package reads
- **THEN** marks the evidence of the unchanged package current, though the tree has uncommitted changes
- **AND** marks the evidence of the changed package stale, naming its inputs changed

### Requirement: A stream's own currency
Each stream SHALL be listed as current or stale as a whole. A stream of the head's run whose verdicts carry input hashes, one package's or more, SHALL be current when the head's working tree still has each hash, else stale, naming in order each package whose inputs changed; any other stream, a stream of the base's run among them, SHALL be judged by commit as its evidence is.

#### Scenario: Streams with input hashes
- **WHEN** audit is given streams with input hashes, one also as the base's run, after an uncommitted change to a package one of them ran
- **THEN** marks the stream that ran the changed package stale, naming each package whose inputs changed
- **AND** marks the stream of one unchanged package current, though the tree has uncommitted changes
- **AND** judges the base's stream by commit

### Requirement: The stream that judges a behavior
Of the streams that judge one behavior the audit SHALL take a current one before a stale one, then the first given, and name the others' verdicts.

#### Scenario: Two streams of one behavior
- **WHEN** a current and a stale stream judge one behavior
- **THEN** states the current stream's verdict and names the stale one's among the others

#### Scenario: Two current streams of one behavior
- **WHEN** two current streams judge one behavior
- **THEN** states the verdict of the first given and names the other's among the others

### Requirement: Assertions at run time
With `--base-evidence=<stream>`, a stream of the base's run, a finding of class `assertions` SHALL carry, beside the counts written in source, the count each side's run reported.

#### Scenario: A drop both runs saw
- **WHEN** audit is given a stream of each side's run and the head drops an assertion
- **THEN** the assertions finding carries the base's and the head's run-time counts

### Requirement: The audit's own run
`gotest audit --run` SHALL run, through gotest's runner, the packages holding a behavior the comparison names and, when the changed lines are known, the packages holding a changed test file and the changed production packages, measuring the coverage of the latter, and take that run as current evidence. Where no package holds one it SHALL run nothing and say so.

#### Scenario: Running what changed
- **WHEN** audit --run compares the working tree with a base
- **THEN** runs the packages of the changed behaviors and states their verdicts as current evidence of that run
- **AND** lists the changed production lines that run did not execute

#### Scenario: A changed test file without a changed behavior
- **WHEN** audit --run compares a head that changes only a suite's hook, which now fails
- **THEN** runs the suite's package though no behavior changed, and lists the failing run

#### Scenario: Nothing to run
- **WHEN** audit --run compares a head that changes no Go file
- **THEN** runs nothing and notes that no package holds a changed behavior, a changed test file or a changed production file

### Requirement: A run of the audit that fails
When the run `gotest audit --run` made exits non-zero and was not stopped, the audit SHALL list one finding of class `failing` about the run, its exit code as the head value, whatever the evidence rows read. It is no weakening: `--fail-on` naming `failing` or `proof` SHALL exit 1 on it, `all` SHALL not, and without `--fail-on` the audit SHALL exit 0. The text format SHALL name it under `run:` and in a `run:` segment of the closing line, Markdown under a `Run` heading.

#### Scenario: A run that fails outside its rows
- **WHEN** audit --run runs an added test whose suite's BeforeAll fails
- **THEN** lists a failing finding about the run naming exit 1, beside the test read as not run, and exits 0 without --fail-on
- **AND** --fail-on=failing exits 1 and the closing line names run: exited 1
- **AND** names the run under run: in the text format and under a Run heading in Markdown

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

### Requirement: A change the run tested nothing of
Where the run `--run` made executed no test and so wrote no profile, a changed non-test file with a changed line inside a function's body SHALL read as without coverage data, and one with none SHALL not be listed.

#### Scenario: A changed package the run executed no test for
- **WHEN** audit --run compares a head that adds a package with a function and no test
- **THEN** lists the package's file as without coverage data and --fail-on=unproven exits 1

#### Scenario: A changed file without a function
- **WHEN** audit --run compares a head that adds a package declaring only a type
- **THEN** lists no unproven file and --fail-on=unproven exits 0

### Requirement: Coverage of the binaries a test starts
When `gotest audit --run` measures coverage it SHALL give the run a `GOCOVERDIR` under its own directory and count, beside the test binaries' profiles, what each binary built with `go build -cover` that a test starts wrote there. A file only binaries built without it execute SHALL read as without coverage data, and where no binary wrote any a note SHALL say how one counts. Nothing SHALL be written into the module, and the directory SHALL go with the run's.

#### Scenario: A test that starts a binary built with -cover
- **WHEN** audit --run runs a test that starts a binary built with go build -cover
- **THEN** lists, of the changed lines that binary's packages hold, only those it did not execute
- **AND** leaves the module as it was and no directory of the run behind

#### Scenario: Binaries built without -cover
- **WHEN** audit --run runs only tests that start binaries built without -cover
- **THEN** lists the changed files only those binaries execute as without coverage data
- **AND** notes that a binary a test starts counts when built with go build -cover

### Requirement: Proof in the policy
`--fail-on` SHALL take the proof classes `failing`, `not-run`, `vacuous`, `stale`, `unproven` and `survived`, and `proof` for the six; `all` SHALL stay every weakening, without them, for they judge the evidence given, not the change. A proof class with nothing to judge SHALL be refused with exit 2. The closing line SHALL count the evidence and the unproven lines.

#### Scenario: A policy on the proof
- **WHEN** --fail-on names failing and a stream fails a behavior the comparison names
- **THEN** exits 1 and the closing line counts the evidence and names the class
- **AND** --fail-on=all names every weakening class and no proof class

#### Scenario: A proof class without evidence
- **WHEN** --fail-on names a proof class and no stream or profile is given
- **THEN** audit refuses it with exit 2, naming the flags that give it

### Requirement: A skip the head adds
The audit SHALL list, as a finding of class `skip`, each behavior whose own body writes more calls that skip the test than the base's did, and each added behavior whose body writes one, naming both counts: `Skip`, `Skipf` and `SkipNow` of `testing`'s T, B, F and TB and `Skipf` of gotest's T, B and F, counted from source into the record, a helper's calls not followed. A body that writes no more of them than before SHALL be no such finding.

#### Scenario: Skip calls added
- **WHEN** the head adds calls that skip the test to two bodies and adds a test whose body writes one
- **THEN** lists each as a finding of class skip naming the calls its body wrote before and writes now
- **AND** --fail-on=all exits 1 and the closing line counts the three skip findings

#### Scenario: A body that keeps its skip call
- **WHEN** the head changes a body that wrote a skip call and still writes one
- **THEN** lists no skip finding for it

### Requirement: A record without skip counts
Where a record was written without the counts of the calls that skip the test, the audit SHALL list no `skip` finding and say so in a note.

#### Scenario: A record without skip counts
- **WHEN** the base record was written without skip counts
- **THEN** lists no skip finding and notes that a skip the head adds is not listed

### Requirement: A behavior the run skipped
An evidence row whose behavior the run skipped, by a call in the test, by its suite's guard or by a focus elsewhere, SHALL read `skip`, give the reason, and count as not run: under `not-run`, in the closing line and as a row that needs a decision. A row `X_` excludes SHALL not count: its `excluded` finding names it. A skip SHALL be no weakening by itself, so a changed test under a guard the base already has leaves `all` unbroken.

#### Scenario: A changed test under a guard that holds
- **WHEN** audit --run runs a changed test whose suite's guard, as the base has it, keeps it from running
- **THEN** reads the test as skip with the guard's reason, counts it as not run and lists no weakening
- **AND** --fail-on=all exits 0 and --fail-on=not-run exits 1

#### Scenario: A test the head makes skip
- **WHEN** audit --run runs tests the head added skip calls to
- **THEN** reads each as skip and counts it as not run, beside its skip finding

#### Scenario: A behavior X_ excludes
- **WHEN** a stream's run left out a behavior X_ excludes
- **THEN** reads it as skip and does not count it as not run

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

### Requirement: Rows a tool links
In the JSON document a finding and a behavior SHALL name, as `evidence`, the id of the evidence row about its behavior where there is one; a behavior and an evidence row SHALL carry its `module`; an evidence row SHALL carry, as `location`, where the head declares its behavior, and, as `mark`, the mark the text format writes on its rows; a behavior SHALL carry, as `order`, its place in the text format's list, the findings being in the text format's order.

#### Scenario: A tool linking the rows
- **WHEN** a tool reads the audit document with evidence as JSON
- **THEN** finds on each finding and behavior about an evidence row's behavior that row's id, and none on the others
- **AND** finds on each behavior and evidence row its module, and on each evidence row the head's file and lines of its behavior
- **AND** finds on each evidence row the mark the text format writes on its rows
- **AND** finds on each behavior its place in the text format's list of behaviors

### Requirement: A stopped audit run
`gotest audit --run --control=stdin` SHALL hand its standard input to the run it makes, which stops on a stop line or the input's end as a run does, and an interrupt the audit gets SHALL stop that run as one; the audit SHALL then write its document with what that run reported, name the run as stopped among what was not compared, and exit 130 whatever the policy. --control SHALL be refused with exit 2 without --run, and beside a stream or diff read from standard input.

#### Scenario: A run stopped by the input's end
- **WHEN** audit --run --control=stdin has its standard input at its end
- **THEN** writes the document naming the stopped run among what was not compared and exits 130
- **AND** the text format's closing line names exit 130

#### Scenario: An interrupted audit run
- **WHEN** audit --run is interrupted while its run is under way
- **THEN** writes the document naming the stopped run among what was not compared and exits 130
- **AND** leaves no directory of the run behind

#### Scenario: Control without the run
- **WHEN** --control=stdin is given to audit without --run, or beside --evidence=-
- **THEN** audit refuses it with exit 2, naming --run or standard input

### Requirement: A tree that does not compile
A record SHALL list a package that does not compile with its errors and no suites, and exit 2; the audit SHALL compare none of that package's behaviors, name it as not compared and exit 2 unless the policy failed.

#### Scenario: A head package that does not compile
- **WHEN** the head tree holds a package that does not compile
- **THEN** audit --record lists the package with its errors and no suites and exits 2
- **AND** audit compares none of its behaviors, names it as not compared and exits 2

### Requirement: Mutants of the changed lines
`gotest audit --mutate` SHALL write, from the syntax and types of the packages, mutants of each changed executable line of their non-test files: a comparison negated or its boundary moved, `&&` and `||` swapped, an `if` or `for` condition negated, a statement removed, a return of zero values first, `true`, `false`, `0` and `1` flipped, a string emptied only under `--mutate-strings`. It SHALL compile them into one build, each switched on alone, and leave the module as it was.

#### Scenario: Mutating a change
- **WHEN** audit --mutate is given a diff that changes the lines of a package
- **THEN** lists each changed line holding a mutant once, with each mutant's operator, original text and mutated text
- **AND** leaves the module as it was

### Requirement: The suites that judge a mutant
Before any mutant, `--mutate` SHALL run, as `--run` does, every package the patterns match that holds tests, measuring each suite's coverage of the mutated packages, a binary a test starts built with `go build -cover` included, whether or not the test imports them. A mutant SHALL be judged by the suites that executed its line; one of a line no suite executed SHALL read as not covered and not be run.

#### Scenario: Suites by coverage
- **WHEN** audit --mutate judges lines only another package's tests or a binary a test starts execute
- **THEN** judges each line's mutants by the suites that executed it, that package's suite and the one starting the binary among them
- **AND** reads the mutants of a line no test executed as not covered

### Requirement: A mutant's verdict
A mutant SHALL be run through gotest's runner with only the suites that judge it: killed at their first failing test, named, survived when they pass, a timeout when a run outruns three times the run with no mutant on, start-up included, and a second, alone after the others, twice that. Before any mutant, those suites SHALL pass on the build with none switched on; else no mutant SHALL be judged, the failures named among what was not compared, and the audit exits 2 unless the policy failed.

#### Scenario: A weakly tested change
- **WHEN** audit --mutate runs the mutants of a change one of whose lines no test checks at its boundary
- **THEN** marks each mutant a test fails killed, naming that test
- **AND** marks the boundary mutant survived, naming the suites that ran its line
- **AND** marks a mutant that keeps its suite from ending a timeout, apart from the killed ones

#### Scenario: A run a busy machine slowed
- **WHEN** a mutant's first run outruns its limit and its second run ends in time
- **THEN** judges the mutant by its second run, run alone after the other mutants

#### Scenario: Suites that fail with no mutant
- **WHEN** a suite that judges a mutant fails before any mutant is switched on
- **THEN** judges no mutant, names the failing test among what was not compared and exits 2

### Requirement: The mutation budget
`--mutate-budget`, three minutes unless given, SHALL bound the time the mutants run: once it is spent, no mutant SHALL start and each left, and each a run the budget's end stopped, SHALL read as not judged, never killed or survived. The first mutant of every line SHALL run before the second of any, the lines of packages holding a changed behavior first; `--parallel` SHALL bound the tests the runs make at once.

#### Scenario: A budget spent
- **WHEN** audit --mutate is given a budget spent before its first mutant
- **THEN** reads each mutant it would run as not judged and none as killed or survived
- **AND** names the budget and the time spent

### Requirement: Mutants in the document
The JSON document SHALL hold, only under `--mutate`, a `mutation` section and its counts under `summary`: one entry per mutated line with an `id` made from its module, file, function and text, never its number, its module, file, package, line, function, `group`, `order`, worst verdict, counts per verdict, the suites that ran it and its mutants. Text SHALL write per file the lines a reader acts on, survivors first, count the rest and add a closing-line segment; Markdown a table.

#### Scenario: Mutants as JSON
- **WHEN** a tool reads the document of audit --mutate as JSON
- **THEN** finds a mutation section whose lines carry an id, module, file, line, verdict, order, counts, the suites that ran them and their mutants
- **AND** finds the mutants counted by verdict in the summary
- **AND** finds neither in a document made without --mutate

#### Scenario: Mutants as text
- **WHEN** audit --mutate renders text
- **THEN** writes per file the survived and timed-out lines first, each with its mutants, function and suites, and counts the other lines
- **AND** the closing line counts the mutants survived, killed, timed out and not judged

#### Scenario: Mutants as Markdown
- **WHEN** audit --mutate renders --format=md
- **THEN** writes the survived and timed-out lines as a table

### Requirement: Survivors in the policy
`survived`, a line holding a mutant that survived, SHALL be a proof class: `proof` names it, `all` does not. `--fail-on` naming it without `--mutate` SHALL be refused with exit 2.

#### Scenario: A policy on survivors
- **WHEN** --fail-on names survived and a mutant survives
- **THEN** exits 1 and the closing line names the class

#### Scenario: Survivors without mutants
- **WHEN** --fail-on names survived without --mutate
- **THEN** audit refuses it with exit 2, naming --mutate

### Requirement: Keeping a line from mutation
A `//gotest:mutate-ignore <reason>` comment SHALL keep from mutation the line it ends or, on a line of its own, the declaration or statement below it, a whole function when it is one; one without a reason SHALL keep none, and `--mutate` says so. A record SHALL list each such comment of the packages' non-test files with its scope and reason, and the audit SHALL list one the head adds as a suppression finding.

#### Scenario: Ignored lines
- **WHEN** audit --mutate meets a comment with a reason at the end of a changed line, and one without a reason above a function
- **THEN** reads the mutants of the first line as ignored, giving the reason
- **AND** judges the function's mutants and notes that the comment without a reason keeps no line

#### Scenario: A comment added
- **WHEN** the head adds mutate-ignore comments
- **THEN** audit --record lists each with its file, line, scope and reason
- **AND** audit lists each as a suppression finding naming the comment

