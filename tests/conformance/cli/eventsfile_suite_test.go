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
