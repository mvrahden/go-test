# Reporting a gotest bug

Report only what is gotest's own fault, only after the user says yes, and
never with the user's code in it.

## Is it gotest?

Worth a report:

- a panic whose top frames are in `github.com/mvrahden/go-test/…`, not in
  the test or the code under test;
- generated harness code that does not compile (errors in
  `gotest_psuite_test.go`/`gotest_pxsuite_test.go`) while running
  `go tool gotest` at the pin;
- `census: … never ran` on a run without `-run`/`-skip`;
- a verdict that contradicts the run: exit 0 beside a failed test, or a
  failure for a test that passed;
- `lint -fix` leaving code that no longer compiles, or a finding on code
  the skill or README calls correct;
- a command, flag or behavior this skill or the README documents that
  gotest lacks or does differently — the skill being wrong counts.

Not gotest's fault — fix these instead (most of them look like bugs):

- the CLI is not `go tool gotest` at the pin: a global binary, a `FAIL:`
  drift refusal (see SKILL.md Bootstrap);
- `versions.md` lists the behavior as changed in a later release: upgrade;
  if the upgrade is blocked, it is a known limitation;
- documented limits: a vendored module, stdlib tests under `gotest`,
  suites under `go test`;
- a failure of the test or the code under test, including a declared
  `Timeout` it outlives;
- a `replace`d go-test (a source build): tell the user, do not file.

## Before drafting

1. Reproduce at the newest release in a scratch module outside the
   project, holding only what shows the bug:

   ```sh
   cd "$(mktemp -d)" && go mod init example.com/repro
   go get -tool github.com/mvrahden/go-test/cmd/gotest@latest
   ```

   Use neutral names. Nothing from the project goes in: no code, module
   or file paths, environment values or secrets. If it does not reproduce
   there, it is fixed or not gotest's — upgrade, or tell the user; do not
   file.
2. Search for an existing report with the key error text:
   `gh issue list -R mvrahden/go-test --state all --search "<error text>"`,
   or `https://github.com/mvrahden/go-test/issues?q=<error text>` without
   `gh`. On a match, point the user to it instead of filing again.

## The draft

A one-line title saying what is wrong, and a body with the bug-report
form's sections, in this order:

```markdown
### Area

<CLI or test runs | Generated harness (suites, fixtures, lifecycle) | Assertions | Linter | VS Code extension | GitHub Action | Docs or agent skill>

### gotest version

<output of go tool gotest version, from the scratch module>

### Go version

<output of go version>

### OS and architecture

<output of go env GOOS GOARCH>

### What happened

<what you saw, and what you expected instead>

### Minimal reproduction

<the scratch module's go.mod, the test file, the command>

### Output

<the command's full output>

### How was this report written?

Drafted by a coding agent, reviewed by me
```

Show the user the whole title and body, say the issue will be public, and
ask whether to send it. Only an explicit yes sends it; anything else
keeps it a draft.

## Sending it

- **With access to the user's GitHub account** (`gh auth status`
  succeeds, or a GitHub tool that can create issues): file it yourself,
  then give the user its URL.

  ```sh
  gh issue create -R mvrahden/go-test --title "<title>" --body-file "<draft file outside the project>"
  ```

- **Without access:** hand it to the user — the draft, and a link that
  opens the bug-report form with the short fields filled in (values
  URL-encoded). The user picks Area and "How was this report written?" in
  the form and pastes the long sections from the draft:

  ```text
  https://github.com/mvrahden/go-test/issues/new?template=bug_report.yml&title=<title>&version=<gotest version>&go-version=<go version>&os=<GOOS GOARCH>
  ```

Then carry on with the user's task around the bug — never by weakening or
deleting a test to make it pass.
