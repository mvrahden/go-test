package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIEventsFileTestSuite: --events-file saves the run's go test -json event
// stream.
type CLIEventsFileTestSuite struct{}

func (s *CLIEventsFileTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Estimated for Windows CI: the slowest method ~15s; the budget is twice that, rounded up.
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLIEventsFileTestSuite) TestTheFileHoldsTheStream(t *gotest.T) {
	path := filepath.Join(t.TempDir(), "not", "yet", "events.json")
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/passing/", "--events-file="+path)
	gotest.NoError(t, err)
	got, readErr := os.ReadFile(path)

	t.When("a -json run is given --events-file", func(w *gotest.T) {
		w.It("writes the bytes it prints to the file, making its directories", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stderr: %s", run.Stderr)
			gotest.NoError(it, readErr)
			gotest.NotEmpty(it, run.Stdout)
			gotest.Equal(it, run.Stdout, string(got))
		})
	})
}

func (s *CLIEventsFileTestSuite) TestTheFileReplaysAsTheRun(t *gotest.T) {
	type row struct {
		Desc   string
		format string
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "the summary as JSON", format: "json"},
		{Desc: "the summary as Markdown", format: "md"},
		{Desc: "the summary for a terminal", format: "terminal"},
	}) {
		path := filepath.Join(t.TempDir(), "events.json")
		live, err := testkit.RunCLI(t.Context(), ".", "summary", "--no-color", "--format="+r.format, "--events-file="+path, "../testdata/failing/")
		gotest.NoError(t, err)
		replay, err := testkit.RunCLI(t.Context(), ".", "summary", "--no-color", "--format="+r.format, "--input="+path)
		gotest.NoError(t, err)

		t.When("a summary run's --events-file is read back with summary --input", func(w *gotest.T) {
			w.It("renders the summary the run rendered, with its exit code", func(it *gotest.T) {
				gotest.Equal(it, 1, live.Exit, "stderr: %s", live.Stderr)
				gotest.Equal(it, live.Exit, replay.Exit, "stderr: %s", replay.Stderr)
				gotest.NotEmpty(it, live.Stdout)
				gotest.Equal(it, live.Stdout, replay.Stdout)
			})
		})
	}
}

// elapsed masks the durations a summary prints, which differ between two runs.
var elapsed = regexp.MustCompile(`\(<?[0-9][0-9.hmnsµ]*\)`)

func (s *CLIEventsFileTestSuite) TestTheFileChangesNoOtherOutput(t *gotest.T) {
	type outputs struct {
		run                          testkit.Run
		stepSummary, profile, stdout string
	}
	summarize := func(events bool) outputs {
		dir := t.TempDir()
		args := []string{"summary", "--no-color", "--github", "--coverage=" + filepath.Join(dir, "cover.out"), "--min=100"}
		if events {
			args = append(args, "--events-file="+filepath.Join(dir, "events.json"))
		}
		args = append(args, "--", "-coverprofile="+filepath.Join(dir, "cover.out"), "../testdata/failing/", "../testdata/covered/")
		run, err := testkit.RunCLIWithEnv(t.Context(), ".", []string{"GITHUB_ACTIONS=true", "GITHUB_STEP_SUMMARY=" + filepath.Join(dir, "step.md")}, args...)
		gotest.NoError(t, err)
		step, err := os.ReadFile(filepath.Join(dir, "step.md"))
		gotest.NoError(t, err)
		profile, err := os.ReadFile(filepath.Join(dir, "cover.out"))
		gotest.NoError(t, err)
		return outputs{
			run:         run,
			stepSummary: elapsed.ReplaceAllString(string(step), "(…)"),
			profile:     string(profile),
			stdout:      elapsed.ReplaceAllString(run.Stdout, "(…)"),
		}
	}
	without, with := summarize(false), summarize(true)

	t.When("a summary with annotations and coverage is run with and without --events-file", func(w *gotest.T) {
		w.It("prints, annotates, covers and exits the same", func(it *gotest.T) {
			gotest.Equal(it, 1, without.run.Exit, "stderr: %s", without.run.Stderr)
			gotest.Contains(it, without.stdout, "::error ")
			gotest.Contains(it, without.stdout, "100.0%")
			gotest.NotEmpty(it, without.stepSummary)
			gotest.Equal(it, without.run.Exit, with.run.Exit, "stderr: %s", with.run.Stderr)
			gotest.Equal(it, without.stdout, with.stdout)
			gotest.Equal(it, without.run.Stderr, with.run.Stderr)
			gotest.Equal(it, without.stepSummary, with.stepSummary)
			gotest.Equal(it, without.profile, with.profile)
		})
	})
}

func (s *CLIEventsFileTestSuite) TestEveryOutcomeWritesTheFile(t *gotest.T) {
	type row struct {
		Desc  string
		pkg   string
		exit  int
		event string
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "a failing run", pkg: "../testdata/failing/", exit: 1, event: `"Action":"fail"`},
		{Desc: "a build failure", pkg: "../../canary/testdata/broken/", exit: 2, event: `"Action":"build-fail"`},
	}) {
		path := filepath.Join(t.TempDir(), "events.json")
		run, err := testkit.RunPackage(t.Context(), t.TempDir(), r.pkg, "--events-file="+path)
		gotest.NoError(t, err)
		got, readErr := os.ReadFile(path)

		t.When("a -json run that does not pass is given --events-file", func(w *gotest.T) {
			w.It("writes its stream to the file, verdict included, and exits as without it", func(it *gotest.T) {
				gotest.Equal(it, r.exit, run.Exit, "stderr: %s", run.Stderr)
				gotest.NoError(it, readErr)
				gotest.Contains(it, string(got), r.event)
				gotest.Equal(it, run.Stdout, string(got))
			})
		})
	}
}

func (s *CLIEventsFileTestSuite) TestAnUnwritablePathFailsFirst(t *gotest.T) {
	type row struct {
		Desc string
		args []string
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "a -json run", args: []string{"-json"}},
		{Desc: "a summary", args: []string{"summary", "--no-color"}},
	}) {
		// A path below a regular file can be neither made nor created, on
		// every platform.
		blocker := filepath.Join(t.TempDir(), "a-file")
		gotest.NoError(t, os.WriteFile(blocker, nil, 0o600))
		args := slices.Concat(r.args, []string{"--events-file=" + filepath.Join(blocker, "events.json"), "../../canary/testdata/broken/"})
		run, err := testkit.RunCLI(t.Context(), ".", args...)
		gotest.NoError(t, err)

		t.When("--events-file names a path that cannot be written", func(w *gotest.T) {
			w.It("exits 2 naming the flag before any package is built", func(it *gotest.T) {
				gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Regexp(it, "^FAIL: --events-file: ", run.Stderr)
				gotest.Empty(it, run.Stdout)
			})
		})
	}
}
