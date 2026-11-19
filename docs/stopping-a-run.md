# Stopping a Run

How a stop reaches gotest from a terminal, a CI job, a container and a program that drives it.

On Linux and macOS runners a cancelled or timed-out job stops the action's run as Ctrl-C does: the suites tear down, and the summary and the event stream are written ([Stopping a run](../specs/lifecycle/spec.md#requirement-stopping-a-run)). The runner's second signal follows 7.5 seconds after the first, and gotest takes it as a second Ctrl-C, so a teardown still running then is cut short ([A second Ctrl-C to gotest](../specs/lifecycle/spec.md#requirement-a-second-ctrl-c-to-gotest)). Outside the action the stop reaches gotest only when no shell holds it back:

- In a container entrypoint, `exec go tool gotest ./...`: a shell left as PID 1 receives `docker stop`'s signal and does not pass it on. With `docker run --init`, put no shell between the init and gotest.
- In Kubernetes, give `command:` as a list of arguments, not a shell string.
- In a CI step of your own, make gotest the step's process with `exec go tool gotest ...` in bash or sh (`go tool` passes a signal on, `go run` does not), set `--timeout` below the job's time limit so the run ends on its own before the runner cancels it, and upload the `--events-file` stream from a later step with `if: always()`.

gotest also stops on SIGHUP, when its terminal closes or an ssh session drops, unless it was started under `nohup` ([Every launcher stops cleanly](../specs/lifecycle/spec.md#requirement-every-launcher-stops-cleanly)), and on Windows when its console closes: the suites tear down and the summary is written.

A program that drives gotest and cannot signal it, as on Windows, starts it with `--control=stdin` and writes lines to its standard input: `stop` is a first Ctrl-C and `force` a second, and the input's end, the program gone, is a stop as well ([Stopping gotest through its standard input](../specs/lifecycle/spec.md#requirement-stopping-gotest-through-its-standard-input)). Every command that stops on Ctrl-C takes the flag; without it gotest never reads its standard input. The VS Code extension cancels a run this way.
