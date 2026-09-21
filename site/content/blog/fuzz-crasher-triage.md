---
title: "From Crasher to Regression Test: Triaging Go Fuzz Failures"
date: 2026-09-20
description: "A Go fuzz session found something. What the corpus file is, how to re-run it, why an unverified re-run proves nothing, and how to promote it to a seed."
tags: ["Workflow"]
keywords: ["go fuzz crasher", "reproduce go fuzz failure", "go fuzz testdata corpus", "go fuzz regression test"]
cta_text: "Turn your next crasher into a regression test."
---

A fuzz session ends with a line like this:

```text
new crasher: testdata/fuzz/FuzzMessageTestSuite_FuzzSummary/659470f35e31f313
```

Something failed, and the engine saved the input that did it. This is the good outcome — it is what you ran the fuzzer for. It is also the point where the workflow usually stops being supported by tooling and starts being a manual ritual: open the file, squint at it, copy the hash into a `-run` pattern, re-run, read a stack trace, fix the bug, and then decide what to do with the file.

That last decision is the one most repositories get wrong, and it is worth doing deliberately.

## What the file actually is

A corpus entry is the engine's encoding of one input, one value per line:

```text {title="testdata/fuzz/FuzzMessageTestSuite_FuzzSummary/659470f35e31f313"}
go test fuzz v1
string("0")
string("0")
[]byte("0000000\xf5")
[]byte("\x30")
```

For a target whose callback takes primitives, that is readable enough. For a struct-typed target it is not: those four lines are the fanned leaves of one `Message`, and reading them back into a value means knowing the field order and the encoding of each leaf. (Why a struct arrives as leaves at all is the subject of [Fuzzing Go Structs]({{< ref "/blog/fuzzing-go-structs" >}}).)

Two properties of this file matter later: it is positional, and it lives outside your source.

## Triage: what still fails

`gotest fuzz triage` scans each target's `testdata/fuzz/<Func>/` directory and re-runs every entry it finds, printing the decoded input and the cause:

{{< terminal title="gotest fuzz triage ./..." >}}
FuzzMessageTestSuite_FuzzNormalize: 1 crasher
  file:  testdata/fuzz/FuzzMessageTestSuite_FuzzNormalize/923d28c2ba1241b1
  input: string("!!")
  <span class="t-fail">cause: suite_test.go:12: Equal failed:</span>

FuzzMessageTestSuite_FuzzSummary: 1 crasher
  file:  testdata/fuzz/FuzzMessageTestSuite_FuzzSummary/659470f35e31f313
  input: Message{To: "0", Subject: "0", Retries: -779069752010067920, Priority: Priority(48)}
  <span class="t-fail">cause: panic: summary: negative retries [recovered, repanicked]</span>
{{< /terminal >}}

Note the second `input:` line. The entry on disk is four leaves; what triage prints is the `Message` the target reassembled from them — a Go literal, not an encoding. That line is the bug report.

Each entry re-runs on its own, so one undecodable file does not hide the rest. The command exits 1 while any crasher still fails, which makes it usable as a CI step: a repository whose corpus is all green has no known-failing inputs left.

## Three outcomes, not two

A re-run can end three ways, and the third one is the interesting one.

**Still failing.** The cause is printed. This is a bug you have not fixed yet.

**No longer failing.** Printed as `status: no longer failing`. Someone fixed it, or the behaviour changed. The entry is now a regression test with no home — promote it, as below.

**Unverified.** Printed with a reason:

```text
  status: unverified — the re-run was terminated by signal
```

This third outcome exists because the honest answer to "does this input still crash?" is sometimes "we do not know". A re-run that was killed from outside — a signal, a terminated process, a run that never started — produced no verdict of its own. Reading its exit status as a failure would print a cause nobody observed; reading it as a pass would quietly retire a crasher that may still be live. So triage says unverified, names why, and fails the run, because an unchecked crasher is not a clean one.

## Promote: the crasher becomes source

Once the bug is fixed, the input that found it is valuable and its file is fragile. `gotest fuzz promote` moves it into the test:

{{< terminal title="gotest fuzz promote ./..." >}}
promoted FuzzMessageTestSuite_FuzzNormalize/923d28c2ba1241b1 -> f.Add("!!") in suite_test.go:10
promoted FuzzMessageTestSuite_FuzzSummary/659470f35e31f313 -> f.Add(Message{To: "0", Subject: "0", Retries: -779069752010067920, Priority: Priority(48)}) in suite_test.go:20
{{< /terminal >}}

The result is a diff your reviewer can read:

```diff
 	f.Add("hello")
+	f.Add("!!")
 	f.Add(Message{To: "a@b.c", Subject: "welcome", Retries: 1, Priority: PriorityHigh})
+	f.Add(Message{To: "0", Subject: "0", Retries: -779069752010067920, Priority: Priority(48)})
```

The seed is spliced in after the method's last existing `f.Add`, and the corpus file is deleted. If the method cannot be located with confidence, the file stays where it is and the run says so — promote never partially edits source.

Three things change once the input lives in the test file.

**It replays for free.** Seeds run as ordinary subtests of the target on every plain `gotest ./...`. You get the regression test without budgeting any fuzzing time in CI.

**It is reviewable.** `f.Add(Message{Retries: -779069752010067920})` in a pull request next to the fix says what was wrong. A new binary file under `testdata/` says nothing, and reviewers approve it unread.

**It survives refactoring.** Which brings us to the reason this matters more than tidiness.

## The refactor trap

A corpus entry is positional. Its values line up with the leaves of the target's parameter list, in order.

Swap two same-typed fields in the struct you fuzz — `To` and `Subject`, both strings — and the stored entry still loads, still runs, and now means something else. The same file that used to produce `Message{To: "alice", Subject: "hello"}` produces `Message{Subject: "alice", To: "hello"}`. Nothing warns you. The entry you saved because it crashed is now testing a different input, probably an uninteresting one.

Add or remove a field and the arity changes, which gotest does catch, before any fuzzing budget is spent:

```text
fuzz: FuzzMessageTestSuite_FuzzSummary: testdata/fuzz/.../deadbeefcafe0001 has 4 values of
[string, string, []byte, []byte], but the target now takes 3 [string, string, []byte] — it
predates a change to the fuzzed type's fields; run gotest fuzz promote to turn it into a typed
f.Add seed, or delete it
```

The silent case is the dangerous one, so there is a lint rule for it. `fuzz-struct-corpus` flags a struct-typed target that is still keeping corpus files:

```text
suite_test.go:17:1: fuzz target MessageTestSuite.FuzzSummary keeps 1 corpus entry under
testdata/fuzz/FuzzMessageTestSuite_FuzzSummary/ bound to the declaration order of
fuzzdemo.Message's fields — a same-kind reorder silently reinterprets them and an added or
removed field rejects them; run gotest fuzz promote to turn them into typed f.Add seeds
```

A typed seed has none of this exposure. `f.Add(Message{To: "0", Subject: "0", ...})` names its fields; reorder the struct and the seed still means what it said. Rename a field and the compiler tells you.

## The loop in CI

Fuzzing in CI is a search, not a test. It runs for a budget, and most runs find nothing. Set it up so the runs that *do* find something hand you a crasher rather than a red build you have to archaeology.

```yaml {title=".github/workflows/fuzz.yml"}
- uses: mvrahden/go-test@v1
  with:
    fuzz: true
    fuzz-for: 10m
```

`fuzz-for` is the budget for the whole session, split across targets — each gets `--for × min(--jobs, targets) / targets`, never less than ten seconds, and the schedule prints before the search starts. The action caches the corpus between runs (`fuzz-cache`, on by default), restoring the most recent one for the branch and falling back to the base, so coverage accumulated on main gives every pull request a head start. It sets a `fuzz-crashers` output listing any new files, which is what you hang a comment or an issue on.

Then the local half of the loop:

1. `gotest fuzz triage ./...` — see what is real, with the input as a Go literal.
2. Fix the bug.
3. `gotest fuzz promote ./...` — the input becomes a seed, the file disappears.
4. Commit the fix and the seed together.

Step four is the point of the whole exercise. The pull request that fixes the bug carries the input that found it, in a form the next reader can understand and the next refactor cannot silently invalidate.

## Further reading

For why a struct-typed target's corpus arrives as separate leaves — and why seeds do not — see [Fuzzing Go Structs]({{< ref "/blog/fuzzing-go-structs" >}}). For the CI wiring around this, including summaries and annotations, see [Go Tests in GitHub Actions]({{< ref "/blog/gotest-in-ci" >}}). The commands, flags and exit codes are in the [Fuzzing reference](/reference/#fuzzing).
