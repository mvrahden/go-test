---
title: "How Go Names Its Subtests"
date: 2026-09-25
draft: true
description: "How many subtest names in the 1,000 most-starred Go repositories are written for a reader, how many are identifiers, and how many come from a table?"
tags: ["Research", "AI Engineering"]
keywords: ["go subtest naming", "t.Run naming convention", "go testing statistics", "readable go test names", "go tests for coding agents"]
cta_text: "Measure your own repository with the census scanner."
cta_command: "go run github.com/mvrahden/go-test/research/test-census@research/subtest-naming -out census.csv ."
---

<!--
DRAFT. Every number marked [[…]] below is a placeholder awaiting the corpus rerun.
The scanner change that produces them lives on branch research/subtest-naming
(three new columns: subtest_literal_names, subtest_prose_names, subtest_dynamic_names).
Rerun: ./run.sh corpus/census-full.csv results.csv 50 — pinned SHAs, so the
existing numbers reproduce and the new columns are attributable to the same commits.
Do not publish with a placeholder left in.
-->

A test function name is an identifier. It cannot contain a space, so `TestCreateUser_WhenEmailInvalid_ReturnsError` is the best a Go developer can do to state a scenario there, and [36.7% of test functions in the top 1,000 repositories]({{< ref "/blog/how-go-actually-tests" >}}) do exactly that. A subtest name is different. `t.Run` takes a string, and a string can say "when the email is invalid, returns an error" in words. It is the one place the standard library lets a test state a behavior the way a person would say it.

So we asked the corpus a question the [July census]({{< ref "/blog/how-go-actually-tests" >}}) did not: when Go developers get a string, what do they put in it?

## What we measured

Same corpus, same commits, one new classification. For every `t.Run` call site in the 917 repositories with tests, the scanner looks at the first argument and sorts it into one of three bins.

- **Prose.** A string literal containing a space: `t.Run("returns an error when the email is invalid", …)`. Written for a reader.
- **Identifier.** A string literal without a space: `t.Run("InvalidEmail", …)`, `t.Run("case_3", …)`. Written for `-run`.
- **Computed.** Anything that is not a literal: `t.Run(tc.name, …)`, `t.Run(fmt.Sprintf("%d", i), …)`. The name lives in a table row or is built at runtime, and the scanner cannot see it without executing the test.

The classification is syntactic and errs toward the identifier bin: a literal with a hyphen or a slash and no space counts as an identifier even when a human would read it fine. Computed names are neither good nor bad in this framing. They are the table-driven pattern, and the July census found it in 21.8% of test functions. What the bin tells us is that the behavior's name is data, and data is only readable once the test has run.

## What the corpus does

Of [[N]] subtest call sites, [[P]]% are prose, [[I]]% are identifiers, and [[C]]% are computed.

<!-- chart: three-way split, full corpus vs top 100 vs long tail; charts.py needs a new function for it -->

[[Interpretation paragraph. Candidate readings, to be chosen once the numbers exist:
- If prose is the minority: the one place Go lets a test speak, most tests still write identifiers. Structure demand without a home, again.
- If prose is the majority among literals but computed dominates overall: developers do write behaviors in words when they write them by hand; the table-driven pattern moves the words into data where no tool can read them before a run.
- Top-100 vs long-tail split: does readability track with the maturity gradient, or invert like t.Helper did?]]

## Why this matters more than it did in July

Two months ago this would have been a curiosity about style. It is not anymore, for a reason the [first post in this series]({{< ref "/blog/spec-driven-development-go" >}}) laid out: coding agents now read tests as the specification of what a system does, and the tools that organize that work, OpenSpec and Spec Kit among them, ask for scenarios stated "so plainly you could hand it to someone else to test".

A subtest named `"InvalidEmail"` is a test. It is not a scenario. An agent handed the `-v` output of a package full of identifiers learns which functions exist, not what the system promises. A subtest named `"returns an error when the email is invalid"` is both, and it costs nothing extra to write. The [[P]]% figure is, in that light, a measure of how much of the ecosystem's test suite is already a specification an agent can read, and the [[I]]% figure is how much of it could be with a rename.

The computed bin is the interesting one. [[C]]% of subtest names come from a table row, which means the behaviors exist, are usually well named, and are invisible to anything that reads source without running it. `gotest spec --static` reports exactly this case as `incomplete:` for an `Each` over a computed table, because a static spec that silently omits them would be lying. That design choice was made before this measurement, and the measurement is the first time we can say how often it applies.

## Reproducing it

The scanner change is small: a new function that classifies the first argument of each `t.Run` and three new CSV columns. It is on the [research/subtest-naming branch](https://github.com/mvrahden/go-test/tree/research/subtest-naming/research/test-census), and the full rerun uses the pinned SHAs from the published dataset so every earlier number in the July post reproduces alongside the new ones.

```sh
go build -o test-census . && export PATH=$PWD:$PATH
./run.sh corpus/census-full.csv results.csv 50
```

To measure one repository, point the binary at it:

```sh
test-census -out census.csv path/to/repo
```

The summary line reads `subtest names: 76.3% literal (100.0% of those prose, with a space) | 23.7% computed at runtime`, which is what gotest's own stdlib tests report, and which is the kind of number we would like to see the ecosystem move toward.

[[Closing paragraph once numbers exist: what the July "structure demand" reading looks like with this column added; link to readable-tests-with-bdd as the practical follow-up.]]
