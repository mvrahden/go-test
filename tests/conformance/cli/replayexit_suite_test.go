package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIReplayExitTestSuite: spec and summary given --input exit as the run
// their stream records ended.
type CLIReplayExitTestSuite struct{}

func (s *CLIReplayExitTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

// Streams written by hand: the green tests of passedRun under another close.
const (
	calcPasses = `
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"attr","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds","Key":"gotest.assertions","Value":"2"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"pass","Package":"example.com/m/calc"}
`
	closedByPolicy = runOpens + calcPasses + `{"Action":"fail","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":1,"Packages":{"Passed":1,"Failed":0,"Skipped":0},"Tests":{"Passed":2,"Failed":0,"Skipped":0}}}
`
	closedWithTwo = runOpens + calcPasses + `{"Action":"fail","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":2,"Packages":{"Passed":1,"Failed":0,"Skipped":0},"Tests":{"Passed":2,"Failed":0,"Skipped":0}}}
`
	closedStopped = runOpens + calcPasses + `{"Action":"start","Package":"interrupted","RunLevel":true}
{"Action":"output","Package":"interrupted","RunLevel":true,"Output":"FAIL: run interrupted while running: example.com/m/slow TestSlowTestSuite\n"}
{"Action":"fail","Package":"interrupted","RunLevel":true}
{"Action":"fail","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":130,"Packages":{"Passed":1,"Failed":1,"Skipped":0},"Tests":{"Passed":2,"Failed":0,"Skipped":0}}}
`
)

// replayStream writes stream to a file and runs cmd over it with --input.
func replayStream(t *gotest.T, stream, cmd string, args ...string) testkit.Run {
	file := filepath.Join(t.TempDir(), "run.json")
	gotest.NoError(t, os.WriteFile(file, []byte(stream), 0o600))
	run, err := testkit.RunCLI(t.Context(), ".", append([]string{cmd, "--no-color", "--input=" + file}, args...)...)
	gotest.NoError(t, err)
	return run
}

// withoutRunLevel drops the events a run booked for itself, as a filter
// on RunLevel does.
func withoutRunLevel(stream string) string {
	var kept strings.Builder
	for line := range strings.Lines(stream) {
		if !strings.Contains(line, `"RunLevel":true`) {
			kept.WriteString(line)
		}
	}
	return kept.String()
}

// vacuousRun is the stream of a --ci run that failed only on tests that
// passed without an assertion.
func vacuousRun(t *gotest.T) string {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), vacuousPkg, "--ci")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, run.Exit, "stderr: %s", run.Stderr)
	return run.Stdout
}

var replayCommands = []string{"summary", "spec"}

func (s *CLIReplayExitTestSuite) TestARunThatFailedOnlyOnItsPolicy(t *gotest.T) {
	stream := vacuousRun(t)
	t.When("the saved stream of a --ci run that failed only on tests that passed without an assertion is replayed", func(w *gotest.T) {
		w.It("summary --input and spec --input exit 1, as the run did, and 0 with --render-only", func(it *gotest.T) {
			for _, cmd := range replayCommands {
				run := replayStream(it, stream, cmd)
				gotest.Equal(it, 1, run.Exit, "%s: %s", cmd, run.Stderr)
				rendered := replayStream(it, stream, cmd, "--render-only")
				gotest.Equal(it, 0, rendered.Exit, "%s --render-only: %s", cmd, rendered.Stderr)
			}
		})
	})
}

func (s *CLIReplayExitTestSuite) TestARunThatEndedWithAnotherCode(t *gotest.T) {
	t.When("the saved stream of a run that ended with exit code 2 or 130 is replayed", func(w *gotest.T) {
		w.It("summary --input and spec --input exit with that code", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc   string
				stream string
				exit   int
			}{
				{Desc: "exit code 2", stream: closedWithTwo, exit: 2},
				{Desc: "a stopped run, exit code 130", stream: closedStopped, exit: 130},
			}) {
				for _, cmd := range replayCommands {
					run := replayStream(sub, r.stream, cmd)
					gotest.Equal(sub, r.exit, run.Exit, "%s: %s", cmd, run.Stderr)
				}
			}
		})
	})
}

func (s *CLIReplayExitTestSuite) TestAStreamOfSeveralClosedRuns(t *gotest.T) {
	stream := closedByPolicy + passedRun
	t.When("a stream holds a run that failed only on its policy and a later run that passed", func(w *gotest.T) {
		w.It("summary --input and spec --input exit 1", func(it *gotest.T) {
			for _, cmd := range replayCommands {
				run := replayStream(it, stream, cmd)
				gotest.Equal(it, 1, run.Exit, "%s: %s", cmd, run.Stderr)
			}
		})
		w.It("summary --format=json names exit code 1 as the run's", func(it *gotest.T) {
			run := replayStream(it, stream, "summary", "--format=json")
			gotest.Equal(it, 1, run.Exit, "stderr: %s", run.Stderr)
			var doc struct {
				Run struct {
					ExitCode *int `json:"exitCode"`
				} `json:"run"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s", run.Stdout)
			gotest.NotZero(it, doc.Run.ExitCode)
			gotest.Equal(it, 1, *doc.Run.ExitCode)
		})
		w.It("the test-result predicate of the stream says FAILED", func(it *gotest.T) {
			run := testResult(it, stream)
			p := readPredicate(it, &run)
			gotest.Equal(it, "FAILED", p.Result)
			gotest.Empty(it, p.FailedTests)
		})
	})
}

func (s *CLIReplayExitTestSuite) TestAStreamWithoutAClosingResult(t *gotest.T) {
	failing, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/failing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, failing.Exit, "stderr: %s", failing.Stderr)
	vacuous := vacuousRun(t)
	t.When("a stream that go test wrote, or gotest wrote and a filter stripped of its RunLevel events, is replayed", func(w *gotest.T) {
		w.It("exits 1 when it holds a failed test and 0 when it holds none", func(it *gotest.T) {
			gotest.NotContains(it, withoutRunLevel(vacuous), "RunLevel")
			gotest.Contains(it, vacuous, `"ExitCode":1`)
			for _, cmd := range replayCommands {
				failed, err := testkit.RunCLI(it.Context(), ".", cmd, "--no-color", "--input=../testdata/specstreams/failed.jsonl")
				gotest.NoError(it, err)
				gotest.Equal(it, 1, failed.Exit, "%s failed.jsonl: %s", cmd, failed.Stderr)
				green, err := testkit.RunCLI(it.Context(), ".", cmd, "--no-color", "--input=../testdata/specstreams/green.jsonl")
				gotest.NoError(it, err)
				gotest.Equal(it, 0, green.Exit, "%s green.jsonl: %s", cmd, green.Stderr)
				filtered := replayStream(it, withoutRunLevel(failing.Stdout), cmd)
				gotest.Equal(it, 1, filtered.Exit, "%s failing, filtered: %s", cmd, filtered.Stderr)
				policy := replayStream(it, withoutRunLevel(vacuous), cmd)
				gotest.Equal(it, 0, policy.Exit, "%s vacuous, filtered: %s", cmd, policy.Stderr)
			}
		})
	})
}

func (s *CLIReplayExitTestSuite) TestARunThatDidNotCloseItsStream(t *gotest.T) {
	stream := vacuousRun(t)
	open := stream[:strings.LastIndex(strings.TrimRight(stream, "\n"), "\n")+1]
	t.When("the saved stream of a run that never closed it is replayed", func(w *gotest.T) {
		w.It("exits 2, naming gotest run as the package without a verdict", func(it *gotest.T) {
			gotest.Contains(it, stream[len(open):], `"Result"`)
			for _, cmd := range replayCommands {
				run := replayStream(it, open, cmd)
				gotest.Equal(it, 2, run.Exit, "%s: %s", cmd, run.Stderr)
				gotest.Equal(it, "FAIL: input ends before these packages reported a verdict (stream cut short?):\n  gotest run\n", run.Stderr)
			}
		})
	})
}
