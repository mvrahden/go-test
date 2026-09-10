---
title: "Specs Calcify. Tests Don't."
date: 2026-09-10
description: "Spec-driven development in Go without the waterfall: make the specification the test, so it grows with the code, a run verifies it, and it reads as prose."
tags: ["AI Engineering"]
keywords: ["spec-driven development go", "spec driven development", "executable specification go", "openspec go", "spec kit go", "tests as specification", "coding agents go tests"]
cta_text: "Add gotest to your module, then read what your suites promise."
cta_command: "go get -tool github.com/mvrahden/go-test/cmd/gotest@latest"
---

The loudest objection to spec-driven development is that it is waterfall with a chat window. "In that sense, SDD reminds me of the Waterfall model, which required massive documentation before coding so that developers could simply translate specifications into code," wrote François Zaninotto in [Spec-Driven Development: The Waterfall Strikes Back](https://marmelab.com/blog/2025/11/12/spec-driven-development-waterfall-strikes-back.html), and the [Hacker News thread](https://news.ycombinator.com/item?id=45935763) split down the middle over it. A second post put the same point more gently: "[You learn what you're building by trying to make a computer understand it](https://publish.obsidian.md/deontologician/Posts/Spec-driven+development+doesn%27t+work+if+you%27re+too+confused+to+write+the+spec)." A spec written before that learning is a guess, and a guess in a Markdown file has no way of finding out it is wrong.

We think the critics are right about Markdown and wrong about specs. The problem is not that the spec exists before the code. The problem is that the spec lives somewhere the code cannot reach it. Move the spec into the one artifact the compiler and the build already enforce, the test, and every objection above turns into a feature. This post is about what that looks like in Go, and what it does not solve.

## What the tools have in common

Four tools define the practice today: GitHub's [Spec Kit](https://github.com/github/spec-kit), AWS's Kiro, Tessl, and [OpenSpec](https://openspec.dev/), which by its own published numbers is the most widely used. They differ in ceremony. Birgitta Böckeler's [review of the first three on martinfowler.com](https://www.martinfowler.com/articles/exploring-gen-ai/sdd-3-tools.html) watched Kiro's requirements document turn a small bug "into 4 'user stories' with a total of 16 acceptance criteria", and OpenSpec positions itself as the lightweight alternative to exactly that. They agree on three things.

The spec is prose in a Markdown file. An agent turns the prose into code. And whether the code matches the prose is a judgment the agent makes by reading.

That third point is the one to look at closely. Böckeler noted that "often it's left vague or totally open what the spec maintenance strategy over time is meant to be", and that Spec Kit's branch-per-spec layout treats a spec as "a living artifact for the lifetime of a change request, not the lifetime of a feature". OpenSpec answered the maintenance half of that: it keeps a living `openspec/specs/` folder, records each change as ADDED, MODIFIED and REMOVED requirements, and merges the deltas back on archive. That is real progress, and it is why OpenSpec is winning.

The verification half is still open. OpenSpec's [`/opsx:verify`](https://github.com/Fission-AI/OpenSpec/blob/main/docs/commands.md) "searches codebase for implementation evidence" and lists "scenarios covered" as one of the things it checks. Its example output includes the line `⚠ Scenario "System preference detection" has no test coverage`. Nothing ran to produce that warning. An agent read the code, formed an opinion, and printed it. [Issue #880](https://github.com/Fission-AI/OpenSpec/issues/880) asks for a way to check code against the living specs at all, and it is labeled future roadmap.

None of this is a complaint about OpenSpec. Its own [writing guide](https://github.com/Fission-AI/OpenSpec/blob/main/docs/writing-specs.md) says a scenario is "a concrete GIVEN / WHEN / THEN that could become an automated test", and that a good requirement is "one behavior, stated so plainly you could hand it to someone else to test". The tools know where the scenarios want to go. They just stop one step short of sending them there.

An agent can claim a scenario is covered. Only a run can prove it. One commenter in the waterfall thread put it more strongly than we would: "Once specs are captured as tests, the LLM can no longer hallucinate." It can still hallucinate. It can no longer hallucinate quietly.

## A spec that grows with the code

Here is a scenario in the shape OpenSpec asks for:

```md {title="openspec/specs/cart/spec.md"}
### Requirement: Discounts
The cart SHALL apply percentage discounts and never charge a negative total.

#### Scenario: the discount exceeds 100 percent clamps to zero
- **WHEN** a discount over 100% is applied
- **THEN** the total is zero
```

And here is the same scenario as a gotest behavior:

```go {title="cart_test.go"}
type CartTestSuite struct {
    cart *Cart
}

func (s *CartTestSuite) BeforeEach(t *gotest.T) {
    s.cart = NewCart()
    s.cart.Add("widget", 1, 10.00)
}

func (s *CartTestSuite) TestApplyDiscount(t *gotest.T) {
    t.When("the discount exceeds 100 percent", func(t *gotest.T) {
        s.cart.ApplyDiscount(150)
        t.It("clamps to zero", func(t *gotest.T) {
            gotest.Equal(t, 0.0, s.cart.Total())
        })
    })
}
```

The suite is the subject, the method is the capability, `When` is the condition, `It` is the outcome. [BDD in Go]({{< ref "/blog/what-bdd-means-in-go" >}}) explains why those four map onto Go's type system rather than onto a DSL. What matters here is what you can do with the file before and after the implementation exists.

Before anything runs, `gotest spec --static` reads the behaviors straight from source:

{{< spec title="gotest spec --static ./cart" >}}
Cart
  ApplyDiscount
    when the discount exceeds 100 percent
      clamps to zero

1 suites, 1 behaviors
{{< /spec >}}

That is the spec as declared so far. No verdicts, no durations, because nothing has executed. It is the artifact you review at the stage where OpenSpec has you review `proposal.md`, and it is already a compiling Go file. Then the implementation lands, and the same command without `--static` runs the suite:

{{< spec title="gotest spec ./cart" >}}
Cart <span class="t-time">(<1ms)</span>
  ApplyDiscount <span class="t-time">(<1ms)</span>
    when the discount exceeds 100 percent <span class="t-time">(<1ms)</span>
      <span class="t-fail">✗</span> clamps to zero <span class="t-time">(<1ms)</span>
          cart_test.go:18: Equal failed:
                expected: 0
                actual:   -5

1 suites, 1 behaviors: 1 failed
{{< /spec >}}

The scenario now carries a verdict, and the verdict came from execution rather than from an agent's reading of the code. Fix the clamp, run again, and the `✗` becomes a `✓`. Render with `--format=md` and the same tree becomes the document you attach to the pull request. [Go Tests as Living Documentation]({{< ref "/blog/tests-as-documentation" >}}) covers the terminal, Markdown and JSON forms; the point of this post is only that all three are views of one artifact, and that artifact is the test.

In Böckeler's terms this is spec-anchored development, with the anchor being the compiler. The spec cannot drift from the code because the spec is compiled with the code. It cannot be stale because CI runs it. It cannot claim a scenario is covered, because a scenario with no `It` is not in the spec, and an `It` that fails is in the spec with a `✗` beside it.

## The three objections, taken seriously

**You cannot enumerate scenarios before you build.** Correct, and nothing here asks you to. The static spec renders what is declared so far, not what should exist. When implementation teaches you that the discount also needs to reject negative percentages, you add a `When` block, and the spec grows by one line. The most-agreed objection to the [Verified Spec-Driven Development proposal](https://news.ycombinator.com/item?id=47197595) was that implementing is how you discover the edge cases, so a spec sealed before implementation is a guess. Treat the declared spec as a hypothesis, then. A hypothesis that compiles and runs is still a hypothesis. It is just one that tells you when it is wrong. gotest even reports the cases it cannot enumerate. A `When` inside a loop or an `Each` over a computed table shows up on stderr as `incomplete:` with a file and line, because a partial spec that presents itself as whole is worse than none.

**Tests fix an API too early.** A commenter in the same thread warned that writing tests first "will cause the AI to hallucinate the API". This is true of tests whose names are function signatures. It is not true of behaviors, because the label is prose and the API call lives in the body. "when the discount exceeds 100 percent, clamps to zero" tells an agent what must hold, not which method to write. Two implementations with different signatures satisfy the same spec line, and the spec line is what the reviewer reads.

**A spec carries intent that a test cannot.** Also correct. Why the discount is clamped rather than rejected, which alternative was considered, what the migration plan is, none of that belongs in a `When` block, and OpenSpec's `proposal.md` and `design.md` are the right place for it. What we are claiming is narrower: the scenarios are the part of the spec that moves. They are what an agent implements, what verify checks and what archive merges. That is the part worth making executable. The rest can stay prose, because prose is what it is for.

## What it costs to read

Agents read specs, and reading costs tokens. We measured the artifacts a model might be handed for the 23 suites and 111 behaviors in gotest's own [examples](https://github.com/mvrahden/go-test/tree/main/examples), in bytes, because byte counts are reproducible and token counts depend on the tokenizer.

| Artifact | Bytes |
|---|---|
| `gotest spec --static --no-color` | 7,966 |
| `gotest spec --no-color` (with verdicts) | 10,058 |
| `gotest spec --format=md` | 14,882 |
| the `_test.go` sources | 41,200 |
| `go test -v` output | 50,441 |
| `gotest spec --format=json` | 107,019 |

The rendered spec is a third to a fifth the size of the sources it was read from, and it says what the sources promise rather than how they check it. The JSON is the largest artifact of all. It carries every node's status, timing, flags and captured output, which is what an editor or a script wants and what a model does not. Hand a model the text and hand a tool the JSON.

## Where this stops

Three limits, stated plainly.

It is Go only. The scenario-to-behavior mapping depends on suites being structs and behaviors being nested calls the generator can read from source. Other languages have their own ways to do this. This is ours.

The spec is only as good as its labels. A `When("case 3")` renders as `when case 3`, and no tool can make that mean anything. gotest pushes in the right direction, rendering the connective for you and flagging a `When("when …")` or an `It("it …")` with the `behavior-wording` lint rule, but the discipline of writing the condition and the outcome in words is yours. [The next post]({{< ref "/blog/openspec-scenarios-gotest" >}}) shows what happens when an agent does that transcription from OpenSpec scenarios, and how to check its work.

And behaviors cover the behavioral requirements. "The parser SHALL not panic on any input" and "checkout SHALL complete within 200ms" are requirements too, and a `When`/`It` cannot state them. gotest's fuzz targets and benchmark gates can, and a later post in this series takes them up.

If you want to see your own project through this lens, two commands get you there:

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest@latest
go tool gotest spec --static ./...
```

The second one needs a package that already has suites. If none of yours do, the [migration guide]({{< ref "/blog/testify-migration-guide" >}}) turns a `testify/suite` into one in a single command, and the [writing-gotest-tests skill](https://github.com/mvrahden/go-test#coding-agents) teaches a coding agent to write the next one in this shape from the start.
