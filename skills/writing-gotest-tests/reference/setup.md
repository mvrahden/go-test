# Setup and upgrade

The repo has the *library*; the CLI runs via Go's tool directive (requires
Go ≥ 1.24; v1.28+ requires Go ≥ 1.25, v1.30+ Go ≥ 1.26). Run it at the
newest release the project can take.

## Check and upgrade

SKILL.md's three check commands print the pin (and whether it is
`(replaced)`), the module's go line, and every release with its go line.
Pick the newest release whose go line is not newer than the module's (a
module line `1.N` takes a release needing `1.N.0` — the same language
version; `go get` only rewrites the line to `1.N.0`); when
it is newer than the pin, upgrade to it unless one of these holds:

- `(replaced)` — a source build; there is nothing to upgrade.
- A vendored module (`vendor/modules.txt`) — see below.
- The user or the repo's instructions rule out dependency changes, or the
  task only reads code.
- The check itself fails (no network or module proxy).

```sh
go get -tool github.com/mvrahden/go-test/cmd/gotest@<release>
go tool gotest version
```

Never `@latest` without that check: when the newest release needs a newer
Go than the module declares, `go get` silently raises the go line for
everyone. If no release newer than the pin fits, keep the pin and tell the
user which release needs which Go; the project decides.

In a module that does not require go-test yet, the first command fails
("not a known dependency"); the other two still work. Add the newest
release whose go line fits the same way, or report that none does.

To keep the pin, name the current version — a bare `go get -tool`
upgrades without these checks:

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
   (`go get -tool github.com/mvrahden/go-test/cmd/gotest@<old>`, rerun,
   then back to `@<new>`) before blaming the upgrade.
   Fix one the upgrade caused in test code, per those entries, or revert
   the upgrade and report it — never fix it in production code.
4. If the project holds this skill under `.agents/skills` or
   `.claude/skills`, rerun `go tool gotest skill install` (with the same
   `--dir`) so it matches the new pin (v1.34+).
5. Tell the user: `go-test <old> → <new>`.

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
least as new as the module's go line. From v1.33 a refusal about the
binary's Go suggests the version the module already selects (running it
only declares the tool), and the runtime-floor refusal names the floor
release — upgrade to that only under the conditions above. Below v1.33 both say `@latest`:
ignore it. Never upgrade just to silence a refusal. (Bare
`go run github.com/mvrahden/go-test/cmd/gotest` fails on fresh consumers:
module pruning leaves the CLI's deps out of go.sum.)

## go.work workspaces

The tool declared in any `use` module works from the root and inside each
module, at the highest pin. `./...` is rejected at the root, so name each
module (`go tool gotest ./svc/... ./lib/...`) or run inside one. The
`go.work` go line must be at least the modules' go lines. Upgrade every
`use` module that requires go-test to the same release: the newest one
whose go line all of them take.
