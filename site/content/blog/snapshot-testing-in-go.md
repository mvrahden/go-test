---
title: "Snapshot Testing in Go"
date: 2026-07-17
description: "Snapshot testing in Go without boilerplate: gotest.MatchSnapshot stores golden files, diffs on mismatch, works in parallel tests, and is read-only in CI."
tags: ["Patterns"]
keywords: ["snapshot testing go", "golden files go", "go test golden file", "gotest matchsnapshot"]
cta_text: "Try snapshot testing in your Go project."
---

Some test assertions are about structure, not specific values. JSON API responses, rendered HTML, error messages, log output — things where the exact expected value is long, tedious to maintain by hand, and changes often enough to be annoying. Snapshot testing solves this by storing the expected output in a file and comparing against it on subsequent runs.

If you've used Jest, you know the pattern. In Go, it's less common — but [gotest](https://github.com/mvrahden/go-test) has a built-in implementation that's thread-safe, works inside parallel tests, and integrates with CI safety guards.

## The problem: testing large structured output

Consider a test that asserts on a JSON response:

```go
func TestRenderResponse(t *testing.T) {
    resp := renderUserProfile(testUser)
    expected := `{
  "id": "usr-123",
  "name": "Alice Smith",
  "email": "alice@example.com",
  "roles": ["admin", "editor"],
  "preferences": {
    "theme": "dark",
    "notifications": true
  }
}`
    if resp != expected {
        t.Errorf("got %s, want %s", resp, expected)
    }
}
```

This works, but it has problems:

- The expected string is 10 lines of noise in the test file.
- Adding a field means updating the string literal manually.
- Multi-line string formatting is fragile — whitespace and escaping are easy to get wrong.
- Reviewer attention goes to the string literal, not the test logic.

The test is correct, but it's doing the wrong kind of work. The assertion logic is simple (does the output match?), but the expected value dominates the function. The important thing — what `renderUserProfile` is being called with and what the test is checking — is buried.

## Golden files: the stdlib approach

The Go community's conventional answer is golden files: store expected output in `testdata/` and compare at runtime.

```go
func TestRenderResponse(t *testing.T) {
    resp := renderUserProfile(testUser)

    golden := filepath.Join("testdata", t.Name()+".golden")

    if *update {
        os.WriteFile(golden, []byte(resp), 0644)
    }

    expected, err := os.ReadFile(golden)
    if err != nil {
        t.Fatal(err)
    }
    if resp != string(expected) {
        t.Errorf("output mismatch; run with -update to refresh")
    }
}
```

Better — the expected output is in a separate file. But you're still managing the plumbing yourself:

- No standard naming convention — every project invents its own.
- No diff output on mismatch — you just know it's wrong.
- Manual file management — you write the read/write/compare logic in every test.
- The `-update` flag needs to be wired up with `flag.Bool` in each package.
- No thread safety — parallel tests writing to the same file will race.

## `gotest.MatchSnapshot`

`gotest.MatchSnapshot` wraps this entire pattern into a single call:

```go
func (s *UserAPITestSuite) TestRenderProfile(t *gotest.T) {
    resp := renderUserProfile(testUser)
    gotest.MatchSnapshot(t, resp)
}
```

That's it. On first run, gotest creates `testdata/__snapshots__/TestUserAPITestSuite.snap` with the output. On subsequent runs, it compares the current output against the stored snapshot and shows a diff on mismatch.

No boilerplate. No manual file paths. No flag wiring. The test reads exactly like what it does: render a profile, check that it matches the snapshot.

## Where snapshots live

Snapshot files are stored in `testdata/__snapshots__/`, next to the test file. One `.snap` file is created per top-level test suite. Each subtest gets its own named section within the file:

```text {title="testdata/__snapshots__/TestUserAPITestSuite.snap"}
=== SNAP TestRenderProfile/when_the_user_has_roles/renders_the_profile ===
{
  "id": "usr-123",
  "name": "Alice Smith",
  "email": "alice@example.com",
  "roles": ["admin", "editor"],
  "preferences": {
    "theme": "dark",
    "notifications": true
  }
}
=== SNAP TestRenderProfile/when_the_user_is_new/renders_the_profile ===
{
  "id": "usr-456",
  "name": "Bob Jones",
  "email": "bob@example.com",
  "roles": [],
  "preferences": {
    "theme": "light",
    "notifications": false
  }
}
```

Sections are sorted alphabetically within the file, so the order is deterministic regardless of test execution order. The files are plain text — they diff cleanly in pull requests, and reviewers see exactly what changed in the output.

## What gets snapshotted

`MatchSnapshot` serializes the value based on what interface it implements, checked in this order:

1. **`string`** — used directly as the snapshot content.
1. **`[]byte`** — converted to string.
1. **`encoding.TextMarshaler`** — calls `MarshalText()`.
1. **`fmt.Stringer`** — calls `String()`.
1. **`json.Marshaler`** — marshals and pretty-prints the JSON.
1. **`error`** — calls `Error()`.
1. **`io.Reader`** — reads the content (restores position for seekable readers).

This means types that already know how to serialize themselves work without any extra code. For example, a `json.RawMessage` value gets pretty-printed automatically:

```go
gotest.MatchSnapshot(t, json.RawMessage(`{"user":"alice","role":"admin"}`))
```

The snapshot file contains the pretty-printed JSON, not the compact input:

```json
{
  "user": "alice",
  "role": "admin"
}
```

## Custom snapshot names

When a single test produces multiple outputs to snapshot, pass an optional name as the third argument:

```go
gotest.MatchSnapshot(t, renderProfile(admin), "admin-profile")
gotest.MatchSnapshot(t, renderProfile(guest), "guest-profile")
```

The name becomes part of the section key in the snapshot file. Without it, multiple `MatchSnapshot` calls in the same test would overwrite each other. With it, each snapshot gets its own section and can be compared independently.

## Updating snapshots

When the output changes intentionally — you added a field, changed a format, updated a template — the snapshot needs to be updated. Pass the `--update-snapshots` flag:

```sh
gotest --update-snapshots ./...
```

Or set the environment variable:

```sh
GOTEST_UPDATE_SNAPSHOTS=1 gotest ./...
```

This overwrites all snapshot files with the current output. Review the diff with `git diff` before committing — the snapshot files are version-controlled, so you can see exactly what changed and verify it's what you intended.

## Reviewing snapshot changes in PRs

Because snapshot files live in `testdata/__snapshots__/`, they show up in pull request diffs like any other source file. Alphabetical section ordering means that adding a new test doesn't shift existing sections around — unrelated snapshots stay stable, and the diff shows only the sections that actually changed.

Reviewers see the actual output, not an assertion about it. If a change to `renderUserProfile` adds an `"avatar"` field, the PR diff shows that field appearing in the snapshot. The reviewer doesn't need to run the test to understand the impact.

## CI safety: read-only snapshots

In CI mode (`--ci` flag, or auto-detected from the `CI` environment variable), snapshots are read-only:

- Existing snapshots are compared normally — mismatches still fail the test.
- New baseline snapshots cannot be created. If a test calls `MatchSnapshot` and no snapshot file exists, the test fails with: *"no baseline snapshot — run tests locally to generate."*

This prevents a specific failure mode: a developer adds a new `MatchSnapshot` call, runs tests locally (which creates the baseline), but forgets to commit the snapshot file. Without CI protection, the test would silently pass in CI by creating a fresh snapshot, and the next run would compare against that. With CI protection, the missing file is caught immediately.

## Thread safety

`MatchSnapshot` is safe to call from parallel tests. Each snapshot file has its own mutex. Concurrent writes to the same file are serialized, and section ordering is deterministic regardless of execution order.

```go
for i := range 10 {
    t.It(fmt.Sprintf("goroutine %d", i), func(it *gotest.T) {
        it.T().Parallel()
        gotest.MatchSnapshot(it, fmt.Sprintf("value-%d", i))
    })
}
```

All 10 goroutines can call `MatchSnapshot` concurrently. The snapshot file ends up with 10 sections, sorted alphabetically, and the result is identical whether the goroutines ran sequentially or in parallel.

## When to use snapshot testing

### Good fits

- **JSON API responses.** The structure matters, but writing out the entire expected JSON in the test is noise.
- **Rendered templates.** HTML output, email bodies, any template-driven text where the output is long and changes with the template.
- **Error messages and log output.** You want to know when the wording changes, but maintaining string literals is tedious.
- **CLI output.** Command-line tools produce structured text that's hard to assert on inline.
- **Any structured text** where the exact value matters but is tedious to maintain by hand.

### Poor fits

- **Values that change every run.** Timestamps, UUIDs, random tokens — these produce a new snapshot on every run. You can snapshot them if you normalize first (replace timestamps with a fixed value), but if the variable part is all you're testing, a snapshot adds no value.
- **Simple scalar values.** If the expected value is a single integer or a short string, `gotest.Equal` is clearer. Snapshots are for cases where writing out the expected value inline hurts readability.
- **Binary data.** Snapshots are text-based. Binary content will produce unreadable diffs.

## A complete example

Snapshot testing isn't a new idea. Jest popularized it in JavaScript, and the pattern exists in most testing ecosystems. In Go, cupaloy and go-snaps are established snapshot libraries that offer this workflow as standalone packages; gotest's version differs mainly in being integrated with its assertion set, safe to call from parallel tests, and read-only in CI mode.

`gotest.MatchSnapshot` handles the plumbing: file naming, section management, diff output, update workflow, CI guards, and thread safety. The test code stays focused on what it's actually testing.

Snapshots cover the "is this output right?" half of the job. For the other recurring sources of test friction, [Testing Async Code in Go]({{< ref "/blog/testing-async-go" >}}) shows how to wait for background work without `time.Sleep`, and [Go Tests as Living Documentation]({{< ref "/blog/tests-as-documentation" >}}) shows how the same suites double as a browsable spec of your system's behavior.
