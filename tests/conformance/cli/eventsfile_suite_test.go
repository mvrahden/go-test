package cli_test

import (
	"os"
	"path/filepath"
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
