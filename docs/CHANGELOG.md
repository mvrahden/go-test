# Changelog

What each release changed in the runner, the CLI and the tooling. What a release changed in how tests are written is in the agent skill's [version notes](../skills/writing-gotest-tests/reference/versions.md), which name the behavior each change replaced.

## v1.38

1. **Descriptor 3 is taken at start and released at init** — under gotest
   a suite process starts with its control channel on descriptor 3 (on
   Windows two inherited pipe handles), named by `GOTEST_CONTROL`; the runtime's
   init moves it to descriptor 1000 or above and unsets the variable, so
   the tests and the processes they start see neither
   ([channel](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-stop-reaches-the-suite-on-its-channel)).
   Code that runs before that init, an `-exec` wrapper included, finds
   descriptor 3 taken. Under a wrapper that does not pass it on (`sudo`,
   `ssh`, `docker run`) a stop reaches the suite as one SIGINT to its
   process group, and the stream still ends whole
   ([wrapper](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-suite-under-a-wrapper-that-drops-the-channel)).
   Before v1.38 gotest passed a suite nothing on descriptor 3.

2. **A crash fails the package by its exit status** — the suite runs as
   the runner's own child, with `test2json` beside it as a filter, so a
   suite process that ends by a signal, a panic or an `os.Exit` fails its
   package by its real exit status, and the closing event no longer names
   the test that was running
   ([crash](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-crash-is-the-packages-failure)).
   A converter that dies early costs no verdict: the runner converts the
   output it kept again and says so on stderr
   ([converter](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-converter-that-ends-early)).
   Before v1.38 `test2json` ran the suite on the `-json`, `spec` and
   `summary` pipelines, and its closing event named the test that was
   running.

3. **A second Ctrl-C releases the fixtures on Windows too** — gotest
   sends each suite `force` on its channel, on every OS, which releases
   its fixtures at once, and the shared fixture process `force` on its
   standard input, which ends the contexts its releases run under; what
   still runs two seconds later is killed
   ([second Ctrl-C](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-second-ctrl-c-to-gotest),
   [shared fixtures](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-shared-fixture-process-asked-to-stop)).
   Before v1.38 a second Ctrl-C on Windows killed the suites at once,
   without teardown, and the shared fixture process ignored a second
   stop.

4. **The stop, pid and budget files are gone** — the runner sets neither
   `GOTEST_STOP_FILE` nor `GOTEST_TEARDOWN_BUDGET_FILE`, and the work
   directory holds no stop, pid or budget file beside a suite: the stop,
   the force and the teardown budget travel as lines on its channel
   ([channel](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-stop-reaches-the-suite-on-its-channel)).
   A stop raised while the process runs its `TestMain` is read when the
   first suite opens, which skips every suite with the reason
   `run stopping` and sets up no fixture
   ([before the first suite](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-stop-before-the-first-suite-opens)).
   A dead runner is the channel's end, on every OS: the suite stops by
   itself and, on Unix, after its release ends what its tests started,
   SIGTERM to its own process group and SIGKILL 2s later
   ([runner gone](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-suite-whose-runner-is-gone)).
   Before v1.38 the suite polled the stop file and watched its parent
   and, on Linux and macOS, the reader of its stdout, and on Unix what its
   tests started outlived a dead runner. Nothing user-facing read the
   files; tooling that listed the work directory sees them gone.

5. **The runtime floor and the extension floor are v1.38.0** — a v1.38
   CLI refuses a runtime below v1.38.0 before it generates anything, with
   exit 2 and the `go get -tool` command that fixes it. A v1.38 runtime
   under an older CLI fails at the first suite open, with exit 1 and one
   line naming both versions and `go tool gotest`. Plain `go test` runs
   either as before. Through `go tool gotest` the CLI and the runtime come
   from one module version, so only a global binary or the extension's
   `cliPath` meets these. The extension from 1.38.0 needs a v1.38.0 CLI.
   Before v1.38 the runtime floor was v1.35.0 and the extension's v1.37.0.

## v1.37

1. **The harness imports under its own names** and every load reads past a
   generated file left on disk, so a package declaring `context` or
   `testing`, or a suite renamed since the last `gotest generate`, still
   runs
   ([generated harness](https://github.com/mvrahden/go-test/blob/main/specs/discovery/spec.md#requirement-the-generated-harness)).

2. **`lint -fix` unfocuses files behind a build constraint** — the `focus`
   fix renames a suite in the files a build constraint leaves out too, and
   names, unapplied, a fix such a file would break
   ([the focus fix](https://github.com/mvrahden/go-test/blob/main/specs/lint/spec.md#requirement-the-focus-fix)).

3. **A stop reaches gotest from CI and from a closed terminal** — on Linux
   and macOS runners the action passes a cancelled or timed-out job's
   signal on to gotest, and SIGHUP (a closed terminal, a dropped ssh
   session; not under `nohup`) and, on Windows, the console closing stop a
   run as Ctrl-C does: the suites tear down and the summary and events
   file are written
   ([every launcher](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-every-launcher-stops-cleanly)).
   Before v1.37 the action's shell kept the signal and the run was killed
   without teardown, and a hangup or a closed console ended gotest at once,
   without a summary. Outside the action, `exec` gotest so no shell holds
   the signal back (`ci.md`).

4. **On Windows a gotest that dies leaves its suites to finish** — they run
   to their end and tear down. Before v1.37 they were killed with it,
   mid-method, without teardown.

5. **A full or `noexec` temp dir no longer breaks or stalls a run** — the
   work directory goes under `GOTMPDIR` when it is set, as Go's own does, so a
   run works where `/tmp` is mounted `noexec` (`docker run --tmpfs /tmp`);
   a stop that cannot write a suite's stop file kills that suite at once
   and says so on stderr; and the files kept beside each suite are named
   by its index in the run, so two suites whose names differ only in case
   no longer share them on macOS or Windows. Before v1.37 the work
   directory ignored `GOTMPDIR`, and such a stop waited out the suite's
   shutdown budget, minutes, before the run ended.

6. **`--control=stdin` stops gotest through its standard input** — every
   command that stops on Ctrl-C takes it and then reads `stop` (a first
   Ctrl-C) and `force` (a second) lines, counted with the signals; the
   input's end is a stop
   ([standard input](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-stopping-gotest-through-its-standard-input)).
   The VS Code extension cancels runs, watches and debug sessions this
   way, which gives it a graceful cancel on Windows. Below v1.37 the flag
   is unknown (exit 2), and on Windows the extension's cancel killed the run
   without teardown.

## v1.36.1

1. **A shared fixture's teardown is judged on its protocol** — a fixture
   that logs `X.AfterAll failed:` no longer fails a passing run.

2. **`bench --json` carries `failures`** — what a failed benchmark printed,
   a panic's message and stack included.

## v1.36

1. **Shared fixtures are released concurrently** — each after the fixtures
   holding a pointer field to it, as package fixtures already were. Two
   shared fixtures that share something without such a field (a Docker
   network, a port) used to be released one after another by accident; give
   the one that must go last a pointer field to the other.

2. **A shared fixture is released at its last use** — once no remaining
   planned suite declares it, not at the end of the run, and one only an
   exclusive suite needs starts just before that suite. A suite that reads
   a shared fixture it does not declare can find it gone; declare a pointer
   field for every fixture a suite touches.

## v1.35

1. **The runtime floor is v1.35.0 from v1.35.1** — a v1.35.1 CLI refuses
   a pin below v1.35.0, skipping the retracted v1.34.0; v1.35.0 still
   accepts a v1.34.0 pin. The extension from 1.35.1 needs a v1.35.0 CLI.

## v1.34

1. **The release between setup attempts runs like the final teardown**
   ([stop](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-stop-during-the-release-between-attempts)):
   under the fixture's `Timeout`, failing the run when it overruns a
   declared one, and not cut short by a stop, after which no further
   attempt starts and `AfterAll` does not run again.
2. **A shared fixture started late comes up after its dependencies** —
   one started for the exclusive tail or a bench slot comes up after a
   dependency only that phase needs, is torn down before it, and is
   released under the teardown budget its declared `Timeout` gives
   ([dependencies](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-late-fixtures-dependencies),
   [teardown](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-late-fixtures-teardown-budget)).
   Below v1.34 one whose name sorted before that dependency's waited for
   a dependency not yet started, and its release got the default's
   budget whatever it declared. A stop or `--timeout` ends the wait for a
   late fixture.
3. **A suite or shared fixture whose CLI died tears down by itself**
   ([suites](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-a-suite-whose-runner-is-gone),
   [shared fixtures](https://github.com/mvrahden/go-test/blob/main/specs/fixtures/spec.md#requirement-a-shared-fixture-process-whose-cli-is-killed)):
   after `kill -9`, an OOM kill or a crash of the CLI, a suite ends the
   holding method's context, starts no further method and runs
   `AfterAll`; a shared fixture process runs the `AfterAll` of every
   fixture whose `BeforeAll` ran, and on Linux exits with status 1 once
   its budgets have passed if a hook never returns. Below v1.34 nothing
   told them the CLI was gone: a suite could die without teardown, and
   the shared fixture process ran on, holding what its fixtures built.
   So does a suite whose stdout nobody reads any more under an `-exec`
   wrapper.
4. **No process a test started outlives the run**, even one it never
   waits for
   ([processes](https://github.com/mvrahden/go-test/blob/main/specs/lifecycle/spec.md#requirement-no-process-outlives-the-run)).
   Below v1.34 such a process kept running after the run ended on Unix.
5. **The runtime floor is v1.34.0** — a v1.34 CLI refuses a pin below
   v1.34.0, which only a CLI not run through `go tool gotest` can meet.
   The v1.34 harness calls runtime functions v1.33.0 lacks, and the
   extension needs a v1.34 CLI.
6. **Output follows its reader** — no color into a pipe whatever
   `--no-color` says, JSON documents carry no color or `::error` lines,
   and two output modes (`--spec -json`, `summary -json`) or an unknown
   `--format` exit 2. `version` and `help` print despite a broken
   `.gotest.yml`; `gotest sumary` and `gotest test` suggest the command
   meant (exit 2); every mode notes stdlib-only packages on stderr.
7. **An expired `--timeout` names only what was running** — not a test
   whose process had ended, nor one waiting for a `-parallel` slot; a hang
   in `BeforeAll` names the suite.
8. **A run sweeps what crashed runs left** — the `gotest-work-*` dirs of a
   dead owner in this boot and pid namespace go; unstamped ones after 7
   days, `--debug` ones are kept 7 days. A test binary briefly "text file
   busy" is retried.
9. **Each run writes its overlay into its own work dir** — nothing
   generated is cached, so concurrent runs sharing a cache never break
   each other's build
   ([generated files](https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-generated-files-are-the-runs-own));
   `--no-cache` turns off only the test binary cache, and `--debug`
   prints the overlay inside the work dir. Below v1.34 overlays were
   shared under `~/.cache/gotest/overlays`; entries left there drain
   after 7 days unused.
10. **The binary cache is capped** — each run's start evicts the binaries
    of a checkout that no longer exists, those unused for 7 days, then the
    least recently used until the cache is under 5 GiB. A binary a run is linking or
    copying is never evicted, and a run whose lock was evicted while it
    waited links without the cache. Below v1.34 a key any run touched kept
    every binary.
11. **`GOTEST_SLOW_BUILD_THRESHOLD`** sets when a build is reported as
    still running (a Go duration, default `15s`).

## v1.33

1. **The runtime floor is v1.33.0** — a v1.33 CLI refuses a pin below
   v1.33.0, which only a CLI not run through `go tool gotest` can meet.
   Drift refusals name the version to pin instead of `@latest`.
2. **Run output takes `go test`'s shape** — bare import paths and meta
   patterns (`fmt`, `std`, `all`) run instead of being dropped, standard
   library packages get `[no suites]`/`[no test files]`, a package whose
   suites are all `X_` or deselected reads `[no tests to run]`, and every
   package's `-json` events open with a `start` event.
3. **`generate` and `clean` touch only the main module's packages**
   (go.work modules count) — `generate all` no longer writes harness
   files into dependencies. Bare `gotest clean` cleans the current
   package, like `generate`; use `gotest clean ./...` for the tree.

## v1.32

1. **The runtime floor is v1.32.0** — a v1.32 CLI refuses a pin below it,
   which only a CLI not run through `go tool gotest` can meet.
