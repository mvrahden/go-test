package report_test

import (
	"mutmod/report"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ReportTestSuite is the only test of calc.Grade, through report.
type ReportTestSuite struct{}

func (s *ReportTestSuite) TestGrades(t *gotest.T) {
	gotest.Equal(t, "pass", report.Line(50))
	gotest.Equal(t, "fail", report.Line(49))
}
