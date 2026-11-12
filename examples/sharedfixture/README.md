# sharedfixture — One Resource for Two Packages

Demonstrates a shared fixture: one ledger server, started once for the whole run, read by suites in two packages, each running in a process of its own.

## Structure

- **ledger/ledger.go** — an in-memory ledger behind HTTP on a loopback port, and its client
- **ledger/fixture.go** — `LedgerSharedFixture`: `BeforeAll`/`AfterAll` in the fixture process, `Hydrate`/`Dehydrate` in each suite process
- **accounts/accounts.go** — opens an account with a deposit
- **accounts/suite_test.go** — `AccountsTestSuite` bound to the shared fixture
- **transfers/transfers.go** — moves money between two accounts
- **transfers/suite_test.go** — `TransfersTestSuite` bound to the same fixture
- **README.md** — this file

## Run

```bash
go run ./cmd/gotest spec ./examples/sharedfixture/...
```

```
=== examples/sharedfixture/accounts — 2 passed ===

Accounts — 2 passed
  Open
    when an account is opened with a deposit
      ✓ holds the deposit
  TheLedgerIsShared
    ✓ runs in the fixture process, not in this suite's

=== examples/sharedfixture/transfers — 3 passed ===

Transfers — 3 passed
  Move
    when the source holds enough
      ✓ takes the amount from the source
      ✓ gives it to the target
    when the source holds too little
      ✓ returns ErrInsufficient

Runs from 2026-11-12 14:31 to 14:31 UTC · main @ 26af182b

2 suites, 5 behaviors: 5 passed
```

## What to notice

- The server starts once and both packages read it: `BeforeAll` runs in the fixture process, `Hydrate` in each suite process.
- `URL` and `PID` cross the process boundary as JSON. `Client` is assigned in `Hydrate`, so it is rebuilt in each suite process, and `srv`, unexported, never leaves the fixture process.
- Accounts are named after the test that uses them, because the ledger is shared by two packages and by parallel methods.
- Under plain `go test` these suites do not run: `go test ./examples/sharedfixture/...` prints `[no tests to run]`. The shared fixture needs the gotest runner.

## From here to a database

The same shape holds a database container: `BeforeAll` starts the container and sets a `ConnStr` field, `Hydrate` opens a pool from it in each suite process, `Dehydrate` closes the pool, and `SharedFixtureConfig` returns `gotest.ContainerFixtureConfig()` for the time a container needs to come up.
