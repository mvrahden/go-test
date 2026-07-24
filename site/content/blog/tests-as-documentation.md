---
title: "Go Tests as Living Documentation: The Spec View"
date: 2026-07-18
description: "Tests as documentation that can't drift: gotest's spec view turns Go tests into living specification documents in terminal, markdown, and JSON."
tags: ["Workflow"]
keywords: ["tests as documentation", "go living documentation", "go test markdown output", "gotest spec"]
cta_text: "Generate your first spec from your existing tests."
---

Documentation drifts. A README written six months ago describes code that no longer exists. An architecture doc from last quarter names services that have been renamed. Wiki pages go stale the moment they are published.

Tests don't drift. If the code changes and the test doesn't match, the build fails. This makes tests the most reliable form of documentation in any project — if you can read them. The problem is that most test output is designed for machines, not humans. `go test -v` produces a flat stream of PASS/FAIL lines with no hierarchy. You can see that everything passed, but you can't see what the system *does*.

gotest's spec view renders the same test results as a structured document. Other posts cover the [BDD syntax]({{< ref "/blog/readable-tests-with-bdd" >}}) for writing structured tests and the [`--spec` flag in the development loop]({{< ref "/blog/the-inner-loop" >}}). This post is about what comes after: using spec output as a *deliverable* — a documentation artifact that lives beyond the terminal.

## From test output to specification

The same tests, two views. First, standard `go test -v`:

{{< gotest-output title="go test -v" >}}
=== RUN   TestCartTestSuite
=== RUN   TestCartTestSuite/TestAddItem
=== RUN   TestCartTestSuite/TestAddItem/when_the_cart_is_empty/adds_the_item
--- PASS: TestCartTestSuite/TestAddItem/when_the_cart_is_empty/adds_the_item (0.00s)
=== RUN   TestCartTestSuite/TestAddItem/when_the_cart_is_empty/sets_quantity_to_1
--- PASS: TestCartTestSuite/TestAddItem/when_the_cart_is_empty/sets_quantity_to_1 (0.00s)
=== RUN   TestCartTestSuite/TestAddItem/when_the_item_already_exists/increments_the_quantity
--- PASS: TestCartTestSuite/TestAddItem/when_the_item_already_exists/increments_the_quantity (0.00s)
{{< /gotest-output >}}

Now the same results through `gotest spec`:

{{< spec title="gotest spec ./..." >}}
Cart
  AddItem
    when the cart is empty
      <span class="t-pass">✓</span> adds the item
      <span class="t-pass">✓</span> sets quantity to 1
    when the item already exists
      <span class="t-pass">✓</span> increments the quantity
{{< /spec >}}

The second version is something a product manager can read. It describes what the Cart does, under what conditions, with what outcomes. It is not a test report. It is a behavioral specification, generated from tests that the build enforces. To be fair, tools like gotestsum and Ginkgo's reporters already improve the formatting of `go test` output; the spec view differs in that it renders the behavioral hierarchy itself — suite, method, `When`, `It` — rather than a better-formatted stream of test names.

## Three output formats

`gotest spec` supports three output formats, each serving a different purpose.

### Terminal (default)

A color tree with pass/fail/skip icons. This is what you see in the dev loop:

```sh
gotest spec ./...
```

The terminal format is designed for quick visual scanning during development. It answers "what does this package do?" at a glance.

### Markdown

Headings and behavior tables. This is for documentation:

```sh
gotest spec ./... --format=md --output=docs/behavior-spec.md
```

The markdown output is a self-contained document with a summary header and per-suite behavior tables:

```md {title="docs/behavior-spec.md"}
# Behavior Specification

4 suites, 12 behaviors: 28 passed, 0 failed, 0 skipped.

## Cart

### AddItem

| Behavior | Status | Duration |
|----------|--------|----------|
| **when the cart is empty** | | |
| &nbsp;&nbsp;adds the item | PASS | <1ms |
| &nbsp;&nbsp;sets quantity to 1 | PASS | <1ms |
| **when the item already exists** | | |
| &nbsp;&nbsp;increments the quantity | PASS | <1ms |
```

This is a file you can commit to your repository, link from your README, or publish to a documentation site. It stays accurate because it is generated from tests that the CI pipeline enforces.

### JSON

Structured data for tooling:

```sh
gotest spec ./... --format=json
```

The JSON output includes full metadata: package names, node kinds (suite, method, block, test), status, duration, focused/excluded flags, and the complete child hierarchy. This is the machine-readable format for building custom reports, dashboards, or integrations.

## Post-processing with `--input`

You don't have to run tests to generate specs. The `--input` flag reads saved `go test -json` output:

```sh
# Save test output in CI
go test -json ./... > test-output.json

# Generate spec from saved output (on any machine, later)
gotest spec --input=test-output.json --format=md --output=spec.md
```

Or pipe directly:

```sh
go test -json ./... | gotest spec --input=- --format=md
```

This separation is powerful: run tests on one machine, generate documentation on another. Save test output as a CI artifact, then render specs from it in a post-processing step. The spec generation is fast because it doesn't compile or execute any code — it just transforms JSON into a structured view.

## Embedding specs in project documentation

Once you can generate a markdown spec, you can embed it anywhere.

### In your README

```makefile {title="Makefile"}
spec:
	gotest spec ./... --format=md --output=docs/BEHAVIOR.md
```

Add this to a Makefile or pre-commit hook. The spec file stays in sync with the tests automatically.

### In CI as a build artifact

```yaml {title=".github/workflows/ci.yml"}
- name: Generate behavior spec
  run: gotest spec ./... --format=md --output=behavior-spec.md
- uses: actions/upload-artifact@v4
  with:
    name: behavior-spec
    path: behavior-spec.md
```

Every CI run produces a behavior spec as a downloadable artifact. When a build breaks, the spec shows which behaviors failed — not just which test functions.

### In pull request descriptions

```sh
# Diff the spec to show what behaviors changed
gotest spec ./... --format=md --output=new-spec.md
diff old-spec.md new-spec.md
```

When a PR adds or changes tests, the spec diff shows exactly which behaviors were added, modified, or removed — in plain English. "Added: UserService / Create / when email is duplicate / returns ErrDuplicate" is a more useful PR description than a list of changed files.

## The specification as a contract

When tests follow the BDD structure — suite as subject, method as capability, `When` as context, `It` as behavior — the spec output is more than a report. It is a behavioral contract. ([BDD in Go]({{< ref "/blog/what-bdd-means-in-go" >}}) explores why this structure maps so naturally onto Go's type system.)

- **For developers:** the spec lists every behavior the system promises. Adding a feature means adding a behavior. Changing a behavior is visible in the spec diff.
- **For reviewers:** the spec in a PR shows what changed at the behavior level, not just the code level. "Added: UserService / Create / when email is duplicate / returns ErrDuplicate" is easier to review than reading test code.
- **For product:** the markdown spec is a living requirements document. If a requirement isn't in the spec, it isn't tested. If it's tested, it's in the spec.

This works because the spec is not written separately from the tests. It *is* the tests, rendered in a format that non-developers can read. There is no synchronization problem because there is only one source of truth.

## Structured output for language models

The JSON format from `gotest spec --format=json` contains the full behavioral tree of your project. Feeding this to an LLM gives it a structured understanding of what the system does — not how it's implemented, but what it promises. "UserService / Create / when email is valid / creates the user" communicates the system's contract without requiring the model to parse implementation details. This is more compact and more reliable than feeding raw source code.

## Tests that don't document themselves

Not all tests are good documentation. A test named `TestFoo` with no `When`/`It` blocks produces a spec line that says nothing:

{{< spec title="gotest spec" >}}
Foo
  <span class="t-pass">✓</span> TestFoo
{{< /spec >}}

The spec view rewards structure. When you invest in clear naming — descriptive suite names, verb-based method names, specific `When`/`It` descriptions — the spec output becomes genuinely useful documentation. When you don't, it reflects that too. The tool is honest.

This creates a feedback loop. Once you see the spec output for the first time, you start writing tests differently. Not because a linter tells you to, but because you can see the result. A vague test name produces a vague spec line. A specific test name produces a specification that someone else can read and understand.

## Documentation that can't drift

Documentation that drifts is worse than no documentation — it misleads. Tests can't drift because the build enforces them. The spec view makes that enforcement visible as a document anyone can read.

The workflow is straightforward: write tests with [BDD structure]({{< ref "/blog/readable-tests-with-bdd" >}}), run `gotest spec --format=md`, and commit the output. The spec stays accurate because the tests stay accurate. The tests stay accurate because the build breaks if they don't.

For the development workflow that keeps the spec view in front of you while you code, see [The Inner Loop]({{< ref "/blog/the-inner-loop" >}}). For the full CLI reference, see the [reference docs]({{< ref "/reference" >}}).
