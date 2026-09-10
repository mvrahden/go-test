---
title: "Verify OpenSpec Scenarios by Running Them, in Go"
date: 2026-09-10
description: "OpenSpec says every scenario could become an automated test. In Go it can: GIVEN/WHEN/THEN becomes When/It, a run verifies it, and verify reads verdicts."
tags: ["AI Engineering"]
keywords: ["openspec go", "openspec testing", "openspec verify", "spec-driven development go", "spec kit go tests", "scenario coverage go"]
cta_text: "Add the reconciliation command to your module."
cta_command: "go get -tool github.com/mvrahden/go-test/cmd/gotest-openspec@latest"
---

[OpenSpec](https://openspec.dev/) is the most used spec-driven development framework right now, and its [writing guide](https://github.com/Fission-AI/OpenSpec/blob/main/docs/writing-specs.md) contains the sentence this post is built on: a scenario is "a concrete GIVEN / WHEN / THEN that could become an automated test." We are going to take that literally. In a Go repository with gotest, a scenario becomes a behavior with one transcription, a run turns it into a verdict, and `/opsx:verify` gets to read those verdicts instead of searching the code for evidence.

[The previous post]({{< ref "/blog/spec-driven-development-go" >}}) made the argument. This one is the workflow, command by command, with a small program at the end that answers the question OpenSpec's verify step currently answers by opinion: which scenarios have a behavior, and did it pass?

## The mapping

OpenSpec specs are Markdown with two levels: a requirement stating what the system SHALL do, and scenarios giving concrete examples of it. gotest suites have four: a suite type as the subject, a method as the capability, `When` as the condition, and `It` as the outcome. They line up like this.

| OpenSpec | gotest | Where it goes |
|---|---|---|
| spec file (`specs/cart/spec.md`) | suite type (`CartTestSuite`) | one struct per capability area |
| `### Requirement:` | test method (`TestApplyDiscount`) | the SHALL sentence becomes the method's doc comment |
| `#### Scenario:` | `When` + `It` | the scenario title is the `It` label, or the `When` label followed by the `It` label |
| `- **GIVEN**` | `BeforeEach`, or an outer `When` | state the world is in before the action |
| `- **WHEN**` | `When("…")` | the action, written as a condition without the word "when" |
| `- **THEN**` | `It("…")` | the observable outcome |
| `- **AND**` | a second `It` under the same `When` | outcomes are separate leaves so each gets its own verdict |

Two conventions make the reconciliation at the end of the post work. First, the scenario title is written so that it reads as an outcome, or as a condition followed by an outcome. "the discount exceeds 100 percent clamps to zero" is a `When("the discount exceeds 100 percent")` around an `It("clamps to zero")`. Second, `When` labels are written as bare conditions. gotest renders the connective, so `When("the cart is empty")` shows as `when the cart is empty` everywhere, and its `behavior-wording` lint rule flags a label that spells the word itself. A GIVEN that opens with its own connective keeps it: `When("given an expired session")` renders as written.

## The loop

Here is an OpenSpec change for a cart, from proposal to archive, with gotest inserted at the two points where the tools currently ask an agent to form an opinion.

### 1. Propose

```sh
/opsx:propose add discount clamping
```

The agent drafts `proposal.md`, `design.md`, `tasks.md` and the delta spec. The delta is what we care about:

```md {title="openspec/changes/add-discount-clamping/specs/cart/spec.md"}
## ADDED Requirements

### Requirement: Discounts
The cart SHALL apply percentage discounts and never charge a negative total.

#### Scenario: the discount exceeds 100 percent clamps to zero
- **WHEN** a discount over 100% is applied
- **THEN** the total is zero

#### Scenario: applies a coupon code
- **WHEN** a valid coupon code is entered
- **THEN** the coupon's discount is applied
```

You review this as OpenSpec intends. The review is the plan review, and nothing about it changes.

### 2. Transcribe before implementing

Add one task at the top of `tasks.md`: write the scenarios as gotest behaviors, with assertions, before any implementation. If the agent has the [writing-gotest-tests skill](https://github.com/mvrahden/go-test#coding-agents) loaded, it knows the shape:

```go {title="cart_test.go"}
type CartTestSuite struct {
    cart *Cart
}

func (s *CartTestSuite) BeforeEach(t *gotest.T) {
    s.cart = NewCart()
    s.cart.Add("widget", 1, 10.00)
}

// The cart SHALL apply percentage discounts and never charge a negative total.
func (s *CartTestSuite) TestApplyDiscount(t *gotest.T) {
    t.When("the discount exceeds 100 percent", func(t *gotest.T) {
        s.cart.ApplyDiscount(150)
        t.It("clamps to zero", func(t *gotest.T) {
            gotest.Equal(t, 0.0, s.cart.Total())
        })
    })
}
```

Notice what is missing: the coupon scenario. The agent skipped it, or forgot it, or decided it was out of scope. In a Markdown-only workflow that omission surfaces when a reviewer notices, or when verify happens to grep for the right word. Here it surfaces in the next command.

### 3. Read the declared spec

```sh
go tool gotest spec --static ./cart
```

{{< spec title="gotest spec --static ./cart" >}}
Cart
  ApplyDiscount
    when the discount exceeds 100 percent
      clamps to zero

1 suites, 1 behaviors
{{< /spec >}}

Nothing has run. This is the list of promises the code currently makes, read from source. Put it next to the delta spec and the missing coupon scenario is visible without reading any Go. The reconciliation program below does that comparison for you; for now, note that this is the point at which the spec and the test are the same document, and the document compiles.

### 4. Apply

```sh
/opsx:apply
```

The agent implements the tasks. This is where OpenSpec's fluidity matters: the agent learns while implementing, and when it learns that a negative percentage also needs handling, the right move is to add a `When` block, not to edit a Markdown file it may or may not remember to keep in sync. The delta spec gets the same scenario added, and step 6 will confirm that the two agree.

### 5. Run

```sh
go tool gotest spec ./cart
```

{{< spec title="gotest spec ./cart" >}}
Cart <span class="t-time">(<1ms)</span>
  ApplyDiscount <span class="t-time">(<1ms)</span>
    when the discount exceeds 100 percent <span class="t-time">(<1ms)</span>
      <span class="t-pass">✓</span> clamps to zero <span class="t-time">(<1ms)</span>

1 suites, 1 behaviors: 1 passed
{{< /spec >}}

Same tree, now with verdicts. Every line that was a promise in step 3 is a result here, and the result came from executing the behavior, not from an agent's reading of the code.

### 6. Verify with verdicts, not opinions

This is the step the whole post exists for. OpenSpec's `/opsx:verify` "searches codebase for implementation evidence" and reports what it finds as CRITICAL, WARNING or SUGGESTION. Replace the search with a join. `gotest-openspec` ships beside the CLI from v1.29.1 on; it reads the spec JSON on stdin and the specs directory as its argument, and prints one line per scenario:

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest-openspec@latest
go tool gotest spec --format=json ./cart | go tool gotest-openspec openspec/specs
```

```text {title="output"}
✓ verified     increases the item count
✓ verified     merges the quantities
✓ verified     the discount exceeds 100 percent clamps to zero
? no behavior  applies a coupon code
✓ verified     the cart is empty returns an error

5 scenarios: 4 verified, 0 failing, 0 declared, 1 without a behavior
```

That output is from gotest's own [cart example](https://github.com/mvrahden/go-test/tree/main/examples/cart) reconciled against a five-scenario spec that ships [with the command's tests](https://github.com/mvrahden/go-test/blob/main/cmd/gotest-openspec/testdata/cart.md). The coupon scenario has no behavior, so the command exits 1. Pipe the static JSON instead and the verified lines read `· declared`, which is the same check before implementation. Point the agent at this output in `/opsx:verify` and its job shrinks to explaining the `?` lines, which is the part that needs judgment.

We call the last line **scenario coverage**: the share of scenarios in the spec that have a behavior in the code, and of those, the share that passed. It is a small number to compute and it is the one the WARNING line in OpenSpec's example output is trying to estimate.

### 7. Archive

```sh
/opsx:archive
```

The delta merges into `openspec/specs/`. The suite is already in `main`. From now on, the living spec and the living test say the same thing, and step 6 is the command that keeps it that way. Run it in CI after the test step and a scenario without a behavior fails the build the same way a failing behavior does.

## The command

It is one file with no dependencies beyond the standard library, small enough to read in a sitting. The full source is [in the repository](https://github.com/mvrahden/go-test/blob/main/cmd/gotest-openspec/main.go); the two pieces that matter are how it reads scenarios and how it matches them.

```go {title="cmd/gotest-openspec/main.go"}
var scenarioLine = regexp.MustCompile(`^#{3,4}\s+Scenario:\s*(.+?)\s*$`)

// behaviors indexes every It leaf by its label and by "<condition> <label>",
// so a scenario may spell its context too. Status is the leaf's verdict.
func behaviors(tree specTree) map[string]string {
    found := map[string]string{}
    var walk func(n node, ctx string)
    walk = func(n node, ctx string) {
        switch n.Vocab {
        case "it":
            found[key(n.Display)] = n.Status
            if ctx != "" {
                found[key(ctx+" "+n.Display)] = n.Status
            }
        case "when":
            ctx = strings.TrimPrefix(n.Display, "when ")
        }
        for _, c := range n.Children {
            walk(c, ctx)
        }
    }
    for _, p := range tree.Packages {
        for _, n := range p.Nodes {
            walk(n, "")
        }
    }
    return found
}
```

The `vocab` field is what makes this trivial. gotest's spec JSON records whether each node came from a `When` or an `It`, and the `display` field carries the label as the developer wrote it, with the connective rendered. Matching is case- and punctuation-insensitive, so "Clamps to zero." in the spec meets `It("clamps to zero")` in the code. Everything else is a walk over two trees.

## If you use Spec Kit instead

Spec Kit's [constitution](https://github.com/github/spec-kit/blob/main/spec-driven.md) is stricter than OpenSpec about this: "test scenarios aren't written after code, they're part of the specification", and Article III mandates that no implementation is written before tests exist and fail. That is steps 2 and 3 above, stated as policy. A gotest suite is the artifact Article III requires, and `gotest spec --static` is how you show a reviewer the red tests before anything is green. The reconciliation command reads any Markdown with `#### Scenario:` headings, so a Spec Kit `spec.md` written with that heading works unchanged.

## What the agent reads back

Two more commands close the loop from the agent's side. `gotest discover ./... --json` emits the same behavior tree with a `behaviorsComplete` flag per method, so an agent asked to "list what this package promises" can answer without running or reading the tests, and knows when the list is a floor rather than a total. And when a run fails, the [VS Code extension's]({{< ref "/blog/your-editor-knows-your-tests" >}}) copy-test-results command, filtered to failures, gives the agent the broken behaviors and their assertion messages in a few hundred bytes. The agent gets the spec as text, the tooling gets it as JSON, and neither has to guess what the other meant.

An agent can claim a scenario is covered. Only a run can prove it. The scenarios were always meant to become tests; in Go, with one small command, they are the tests.
