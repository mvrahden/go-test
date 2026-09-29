package gotestrunner_test

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FailureLogTestSuite covers what the runner keeps of the shared fixture
// process's stderr: the lines in which the runtime reports a fixture's
// failure, so the run's verdict can name the cause where stderr is not read.
type FailureLogTestSuite struct{}

func (s *FailureLogTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *FailureLogTestSuite) TestFailureLog(t *gotest.T) {
	t.It("passes everything through and keeps the failure reports", func(it *gotest.T) {
		var passed bytes.Buffer
		log := gotestrunner.ExportNewFailureLog(&passed)
		written := "starting postgres\n" +
			"DBSharedFixture.BeforeAll attempt 1/2 failed: connection refused\n" +
			"FAIL: DBSharedFixture.BeforeAll failed after 2 attempt(s): connection refused\n" +
			"CacheSharedFixture.AfterAll failed: still in use\n" +
			"QueueSharedFixture.AfterAll panicked: nil map\n"
		_, err := log.Write([]byte(written))
		gotest.NoError(it, err)

		gotest.Equal(it, written, passed.String())
		gotest.Equal(it, []string{
			"DBSharedFixture.BeforeAll failed after 2 attempt(s): connection refused",
			"CacheSharedFixture.AfterAll failed: still in use",
			"QueueSharedFixture.AfterAll panicked: nil map",
		}, gotestrunner.ExportTakeFailures(log))
	})
	t.It("reads a report that arrives in pieces", func(it *gotest.T) {
		var passed bytes.Buffer
		log := gotestrunner.ExportNewFailureLog(&passed)
		for _, piece := range []string{"FAIL: DBShared", "Fixture.BeforeAll failed after 1 attempt(s): ", "refused\nand more"} {
			_, err := log.Write([]byte(piece))
			gotest.NoError(it, err)
		}
		gotest.Equal(it, []string{"DBSharedFixture.BeforeAll failed after 1 attempt(s): refused"}, gotestrunner.ExportTakeFailures(log))
	})
	t.It("hands each report out once", func(it *gotest.T) {
		log := gotestrunner.ExportNewFailureLog(&bytes.Buffer{})
		_, err := log.Write([]byte("FAIL: A.BeforeAll failed after 1 attempt(s): x\n"))
		gotest.NoError(it, err)
		gotest.Len(it, gotestrunner.ExportTakeFailures(log), 1)
		gotest.Empty(it, gotestrunner.ExportTakeFailures(log))
	})
	t.It("keeps a bounded number of reports", func(it *gotest.T) {
		log := gotestrunner.ExportNewFailureLog(&bytes.Buffer{})
		var all strings.Builder
		for i := range 100 {
			fmt.Fprintf(&all, "FAIL: F%d.BeforeAll failed after 1 attempt(s): x\n", i)
		}
		_, err := log.Write([]byte(all.String()))
		gotest.NoError(it, err)
		gotest.Len(it, gotestrunner.ExportTakeFailures(log), gotestrunner.ExportFailureLogLimit)
	})
}
