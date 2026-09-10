package main_test

import (
	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FuzzTargetSelectionTestSuite pins how --target narrows a session's targets.
type FuzzTargetSelectionTestSuite struct{}

func (s *FuzzTargetSelectionTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func fuzzTargetsFixture() []gotestrunner.FuzzTarget {
	return []gotestrunner.FuzzTarget{
		{Package: "example.com/a", Dir: "/a", Func: "FuzzATestSuite_FuzzOne"},
		{Package: "example.com/a", Dir: "/a", Func: "FuzzATestSuite_FuzzTwo"},
		{Package: "example.com/b", Dir: "/b", Func: "FuzzBTestSuite_FuzzOne"},
	}
}

func (s *FuzzTargetSelectionTestSuite) TestSelectFuzzTargets(t *gotest.T) {
	t.It("narrows to the exactly named wrapper", func(it *gotest.T) {
		got, err := main.ExportSelectFuzzTargets(fuzzTargetsFixture(), "FuzzATestSuite_FuzzTwo")
		gotest.NoError(it, err)
		gotest.Len(it, got, 1)
		gotest.Equal(it, "FuzzATestSuite_FuzzTwo", got[0].Func)
	})

	t.It("errors on an unmatched name and lists what exists", func(it *gotest.T) {
		_, err := main.ExportSelectFuzzTargets(fuzzTargetsFixture(), "FuzzATestSuite_FuzzTypo")
		gotest.ErrorContains(it, err, `"FuzzATestSuite_FuzzTypo"`)
		gotest.ErrorContains(it, err, "FuzzATestSuite_FuzzOne, FuzzATestSuite_FuzzTwo, FuzzBTestSuite_FuzzOne")
	})

	t.It("says the packages declare none when there are no targets", func(it *gotest.T) {
		_, err := main.ExportSelectFuzzTargets(nil, "FuzzX")
		gotest.ErrorContains(it, err, "declare no fuzz targets")
	})
}
