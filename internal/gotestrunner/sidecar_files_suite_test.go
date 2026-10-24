package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SidecarFilesTestSuite: the files the runner shares with a suite process are
// that process's own, not its binary's, since suites of one binary run as
// concurrent processes.
type SidecarFilesTestSuite struct{}

func (s *SidecarFilesTestSuite) TestStopAndBudgetFilesArePerSuite(t *gotest.T) {
	targets := []gotestrunner.SuiteTarget{
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestATestSuite"}, BinaryPath: "/tmp/pkg.test"},
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestBTestSuite"}, BinaryPath: "/tmp/pkg.test"},
	}
	gotestrunner.ExportAssignBudgetFiles(targets)
	t.When("two suites of one binary are given their sidecar files", func(w *gotest.T) {
		w.It("gives each its own stop file, so a stop signals that suite's process and no sibling's", func(it *gotest.T) {
			gotest.NotEqual(it, targets[0].StopFile, targets[1].StopFile)
			gotest.NotEmpty(it, targets[0].StopFile)
		})
		w.It("gives each its own budget file", func(it *gotest.T) {
			gotest.NotEqual(it, targets[0].BudgetFile, targets[1].BudgetFile)
		})
	})
}
