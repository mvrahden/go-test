package gotestrunner_test

import (
	"strings"

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
	gotestrunner.ExportAssignBudgetFiles(targets, 0)
	t.When("two suites of one binary are given their sidecar files", func(w *gotest.T) {
		w.It("gives each its own stop file, so a stop signals that suite's process and no sibling's", func(it *gotest.T) {
			gotest.NotEqual(it, targets[0].StopFile, targets[1].StopFile)
			gotest.NotEmpty(it, targets[0].StopFile)
		})
		w.It("gives each its own budget file", func(it *gotest.T) {
			gotest.NotEqual(it, targets[0].BudgetFile, targets[1].BudgetFile)
		})
	})

	cased := []gotestrunner.SuiteTarget{
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestCacheTestSuite"}, BinaryPath: "/tmp/pkg.test"},
		{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestcacheTestSuite"}, BinaryPath: "/tmp/pkg.test"},
	}
	gotestrunner.ExportAssignBudgetFiles(cased, 0)
	t.When("two suites' names differ in case alone", func(w *gotest.T) {
		w.It("gives them files that differ where the file system ignores case", func(it *gotest.T) {
			gotest.False(it, strings.EqualFold(cased[0].StopFile, cased[1].StopFile), "stop files %s and %s", cased[0].StopFile, cased[1].StopFile)
			gotest.False(it, strings.EqualFold(cased[0].BudgetFile, cased[1].BudgetFile), "budget files %s and %s", cased[0].BudgetFile, cased[1].BudgetFile)
		})
	})

	first := []gotestrunner.SuiteTarget{{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/a", SuiteName: "TestATestSuite"}, BinaryPath: "/tmp/a.test"}}
	later := []gotestrunner.SuiteTarget{{SuiteSpec: gotestrunner.SuiteSpec{Package: "example.com/b", SuiteName: "TestATestSuite"}, BinaryPath: "/tmp/b.test"}}
	gotestrunner.ExportAssignBudgetFiles(first, 0)
	gotestrunner.ExportAssignBudgetFiles(later, len(first))
	t.When("a later package's suites are given their sidecar files", func(w *gotest.T) {
		w.It("names them by their index in the run", func(it *gotest.T) {
			gotest.Equal(it, "/tmp/b.test.1.stop", later[0].StopFile)
			gotest.Equal(it, "/tmp/b.test.1.budget", later[0].BudgetFile)
		})
	})
}
