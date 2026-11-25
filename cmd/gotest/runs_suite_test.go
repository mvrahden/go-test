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

func (s *RunsListingTestSuite) TestANarrowedRun(t *gotest.T) {
	var k gotestrunner.KeptRun
	k.Record.Manifest.Changed = &protocol.Changed{Ran: 2, Unchanged: 3}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"changed":{"ran":2,"unchanged":3}`)

	k.Record.Manifest.Changed = &protocol.Changed{Ran: 4, NoRecord: true}
	gotest.Contains(t, main.ExportRunsEntryJSON(&k), `"changed":{"ran":4,"unchanged":0,"noRecord":true}`)

	k.Record.Manifest.Changed = nil
	gotest.NotContains(t, main.ExportRunsEntryJSON(&k), `"changed"`)
}
