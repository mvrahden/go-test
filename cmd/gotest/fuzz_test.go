package main //nolint:stdlib-test

import (
	"testing"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

func fuzzTargetsFixture() []gotestrunner.FuzzTarget {
	return []gotestrunner.FuzzTarget{
		{Package: "example.com/a", Dir: "/a", Func: "FuzzATestSuite_FuzzOne"},
		{Package: "example.com/a", Dir: "/a", Func: "FuzzATestSuite_FuzzTwo"},
		{Package: "example.com/b", Dir: "/b", Func: "FuzzBTestSuite_FuzzOne"},
	}
}

func TestSelectFuzzTargets(t *testing.T) {
	t.Run("narrows to the exactly named wrapper", func(t *testing.T) {
		got, err := selectFuzzTargets(fuzzTargetsFixture(), "FuzzATestSuite_FuzzTwo")
		gotest.NoError(t, err)
		gotest.Len(t, got, 1)
		gotest.Equal(t, "FuzzATestSuite_FuzzTwo", got[0].Func)
	})

	t.Run("an unmatched name errors and lists what exists", func(t *testing.T) {
		_, err := selectFuzzTargets(fuzzTargetsFixture(), "FuzzATestSuite_FuzzTypo")
		gotest.ErrorContains(t, err, `"FuzzATestSuite_FuzzTypo"`)
		gotest.ErrorContains(t, err, "FuzzATestSuite_FuzzOne, FuzzATestSuite_FuzzTwo, FuzzBTestSuite_FuzzOne")
	})

	t.Run("a name against zero targets says the packages declare none", func(t *testing.T) {
		_, err := selectFuzzTargets(nil, "FuzzX")
		gotest.ErrorContains(t, err, "declare no fuzz targets")
	})
}
