package main_test

import (
	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunsListingTestSuite: the runs listing keys its fields as spec's
// document does.
type RunsListingTestSuite struct{}

func (s *RunsListingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *RunsListingTestSuite) TestANarrowedRun(t *gotest.T) {
	var k gotestrunner.KeptRun
	k.Record.Manifest.Changed = &protocol.Changed{Ran: 2, Unchanged: 3, Packages: []string{"m/a", "m/b"}}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"changed":{"ran":2,"unchanged":3,"packages":["m/a","m/b"]}`)

	k.Record.Manifest.Changed = &protocol.Changed{Packages: []string{}}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"changed":{"ran":0,"unchanged":0,"packages":[]}`)

	// A record older than the list holds none.
	k.Record.Manifest.Changed = &protocol.Changed{Ran: 4, NoRecord: true}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"changed":{"ran":4,"unchanged":0,"noRecord":true}`)

	k.Record.Manifest.Changed = nil
	gotest.NotContains(t, main.ExportRunsEntryJSON(&k), `"changed"`)
}

func (s *RunsListingTestSuite) TestTheNarrowingOfARun(t *gotest.T) {
	c := main.ExportNarrowedTo([]string{"m/c", "m/a", "m/b"}, 2)
	gotest.Equal(t, protocol.Changed{Ran: 3, Unchanged: 2, Packages: []string{"m/a", "m/b", "m/c"}}, *c)
	gotest.Equal(t, protocol.Changed{Packages: []string{}}, *main.ExportNarrowedTo(nil, 0))
}

func (s *RunsListingTestSuite) TestThePackagesARunJudged(t *gotest.T) {
	var k gotestrunner.KeptRun
	k.Record.Packages = []protocol.PackageVerdict{{Package: "m/b", Verdict: "fail", InputHash: "h2"}, {Package: "m/a", Verdict: "pass"}}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"ran":[{"package":"m/a","status":"pass"},{"package":"m/b","status":"fail","inputHash":"h2"}]`)

	k.Record.Packages = nil
	gotest.NotContains(t, main.ExportRunsEntryJSON(&k), `"ran"`)
}

func (s *RunsListingTestSuite) TestTheTestCountsOfARun(t *gotest.T) {
	var k gotestrunner.KeptRun
	gotest.NotContains(t, main.ExportRunsEntryJSON(&k), `"tests"`)

	k.Record.Result.Tests = &protocol.Tally{Passed: 3, Skipped: 1}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"tests":{"passed":3,"failed":0,"skipped":1}`)
}
