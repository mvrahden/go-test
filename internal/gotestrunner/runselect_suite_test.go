package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunSelectTestSuite: what a run's flags make of each package's tests.
type RunSelectTestSuite struct{}

func (s *RunSelectTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *RunSelectTestSuite) TestShortMode(t *gotest.T) {
	for sub, r := range gotest.Each(t, []struct {
		Desc string
		args []string
		want bool
	}{
		{"no flags", []string{}, false},
		{"-short", []string{"-short"}, true},
		{"-short=true", []string{"-short=true"}, true},
		{"-short=false", []string{"-short=false"}, false},
		{"-short, then -short=false", []string{"-short", "-short=false"}, false},
		{"-test.short", []string{"-test.short"}, true},
		{"-test.short after -args", []string{"./...", "-args", "-test.short"}, true},
		{"-short, then -test.short=false after -args", []string{"-short", "./...", "-args", "-test.short=false"}, false},
		{"-short after -args", []string{"./...", "-args", "-short"}, false},
		{"-shortcut", []string{"-shortcut"}, false},
		{"-short=maybe", []string{"-short=maybe"}, true},
	}) {
		gotest.Equal(sub, r.want, gotestrunner.ShortMode(r.args))
	}
}

func (s *RunSelectTestSuite) TestFlagsThatSelectTests(t *gotest.T) {
	for sub, r := range gotest.Each(t, []struct {
		Desc string
		args []string
		want bool
	}{
		{"no flags", []string{"./..."}, false},
		{"-run", []string{"-run", "X", "./..."}, true},
		{"-skip", []string{"-skip=X", "./..."}, true},
		{"-count=0", []string{"-count=0", "./..."}, true},
		{"-count=1", []string{"-count=1", "./..."}, false},
		{"-short, -v and -race", []string{"-short", "-v", "-race", "./..."}, false},
		{"-test.run after -args", []string{"./...", "-args", "-test.run=X"}, true},
		{"-test.count=0 after -args", []string{"./...", "-args", "-test.count=0"}, true},
		{"-test.bench after -args", []string{"./...", "-args", "-test.bench=."}, true},
		{"-h after -args", []string{"./...", "-args", "-h"}, true},
		{"a flag of the test's own after -args", []string{"./...", "-args", "-custom"}, false},
	}) {
		gotest.Equal(sub, r.want, gotestrunner.SelectsTests(r.args))
	}
}

func (s *RunSelectTestSuite) TestPackagesAFailedRunDidNotCount(t *gotest.T) {
	const stream = `{"Action":"run","Package":"m/a","Test":"TestATestSuite"}
{"Action":"run","Package":"m/a","Test":"TestATestSuite/TestOne"}
{"Action":"pass","Package":"m/a","Test":"TestATestSuite/TestOne"}
{"Action":"pass","Package":"m/a","Test":"TestATestSuite"}
{"Action":"pass","Package":"m/a"}
`
	declared := gotestrunner.DeclaredUnits{Tests: []gotestrunner.CensusCase{{Pkg: "m/a", Path: "TestATestSuite/TestOne"}, {Pkg: "m/b", Path: "TestBTestSuite/TestOne"}}}
	t.It("names, of a failed run, the package holding a declared test without a verdict", func(it *gotest.T) {
		gotest.Equal(it, []string{"m/b"}, gotestrunner.ExportUncounted(stream, declared, 1))
	})
	t.It("names none of a run that passed", func(it *gotest.T) {
		gotest.Empty(it, gotestrunner.ExportUncounted(stream, declared, 0))
	})
}
