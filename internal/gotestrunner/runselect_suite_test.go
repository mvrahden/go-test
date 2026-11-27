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
