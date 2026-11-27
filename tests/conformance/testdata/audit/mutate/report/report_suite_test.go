package report_test

import (
	"os"
	"path/filepath"

	"mutmod/report"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReportTestSuite is the only test of calc.Grade, through report. Under
// MUTMOD_STALL it logs each mutant's run there, and the first to name
// itself in stalled waits until it is stopped.
type ReportTestSuite struct{}

func (s *ReportTestSuite) TestGrades(t *gotest.T) {
	if dir, id := os.Getenv("MUTMOD_STALL"), os.Getenv("GOTEST_MUTANT"); dir != "" && id != "" {
		log, err := os.OpenFile(filepath.Join(dir, "runs"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		gotest.NoError(t, err)
		_, err = log.WriteString(id + "\n")
		gotest.NoError(t, err)
		gotest.NoError(t, log.Close())
		if f, err := os.OpenFile(filepath.Join(dir, "stalled"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600); err == nil {
			_, err = f.WriteString(id)
			gotest.NoError(t, err)
			gotest.NoError(t, f.Close())
			<-t.Context().Done()
		}
	}
	gotest.Equal(t, "pass", report.Line(50))
	gotest.Equal(t, "fail", report.Line(49))
}
