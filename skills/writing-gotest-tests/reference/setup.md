# Setup and upgrade

The repo has the *library*; the CLI runs via Go's tool directive (requires
Go ≥ 1.24; v1.28+ requires Go ≥ 1.25, v1.30+ Go ≥ 1.26). Run it at the
newest release the project can take.

## Check for a newer release

```sh
go list -m -u -f '{{.Version}} {{with .Update}}{{.Version}}{{end}} {{with .Replace}}(replaced){{end}}' github.com/mvrahden/go-test
go list -m -f '{{.GoVersion}}' github.com/mvrahden/go-test@latest
go list -m -f '{{.GoVersion}}'
```

The first line prints the pin, then a newer release if one exists; the
other two print that release's go line and this module's. Upgrade to the
newer release unless one of these holds:

- `(replaced)` — a source build; there is nothing to upgrade.
- A vendored module (`vendor/modules.txt`) — see below.
- The release's go line is newer than this module's: `go get` would
  silently raise the project's Go for everyone. Report it; the project
  decides.
- The user or the repo's instructions rule out dependency changes, or the
  task only reads code.
- The check itself fails (no network or module proxy).

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest@<newer release>
go tool gotest version
```

In a module that does not require go-test yet, the first command fails
("not a known dependency"): compare the second and third lines and add the
newest release only when its go line is not newer than the module's —
`go get -tool github.com/mvrahden/go-test/cmd/gotest@latest`. Otherwise
report it; the project decides.

To keep the pin (no newer release, or a condition above holds), name the
current version — a bare `go get -tool` upgrades without these checks:

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest@$(go list -m -f '{{.Version}}' github.com/mvrahden/go-test)
go tool gotest version
```

## After an upgrade, before writing anything

1. Read the `versions.md` entries newer than the old pin — each names the
   behavior the upgrade replaced.
2. Run `go tool gotest lint -fix ./...` and `go tool gotest lint ./...`,
   then both runners with `-race`.
3. On a failure, check whether it also fails at the old pin
   (`git checkout -- go.mod go.sum`, rerun) before blaming the upgrade.
   Fix one the upgrade caused in test code, per those entries, or revert
   the upgrade and report it — never fix it in production code.
4. Tell the user: `go-test <old> → <new>`.

## Vendored modules

In a vendored module (`vendor/modules.txt`) any go.mod edit breaks every
build until `go mod vendor` reruns, and gotest cannot run vendored at all
(the overlay's `pkg/gotestruntime` import is never vendored) — report, do
not work around.

## Running the CLI

Every command is `go tool gotest <args>`; if Go reports the short name
ambiguous, use `go tool github.com/mvrahden/go-test/cmd/gotest`. Never a
global `go install` binary: it drifts from the pin and from the module's Go.
From v1.29 the CLI refuses to run (`FAIL:`, exit 2; 1 from `lint`) when it
was built with an older Go than the module's go line, or when the pin
resolves below the oldest runtime it generates for; a binary drifted the
other way still runs. That refusal is never a fault in the tests: switch to
`go tool gotest`, which builds the CLI at the pinned version with a Go at
least as new as the module's go line. Ignore any version the message
suggests (below v1.33 it says `@latest`): upgrade only under the
conditions above, never to silence a refusal. (Bare
`go run github.com/mvrahden/go-test/cmd/gotest` fails on fresh consumers:
module pruning leaves the CLI's deps out of go.sum.)

## go.work workspaces

The tool declared in any `use` module works from the root and inside each
module, at the highest pin. `./...` is rejected at the root, so name each
module (`go tool gotest ./svc/... ./lib/...`) or run inside one. The
`go.work` go line must be at least the modules' go lines. Upgrade every
`use` module that requires go-test to the same release.
