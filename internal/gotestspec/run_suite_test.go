// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RunEventsTestSuite covers the run's own events: they describe the run, so
// no package of the tree holds them, and RunOf reads them back.
type RunEventsTestSuite struct{}

func (s *RunEventsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

const runStream = `{"Action":"start","Package":"gotest run","RunLevel":true,"Manifest":{"Version":"v1.2.3","GoVersion":"go1.26.0","Args":["-json","./..."],"Commit":"abc1234","Dirty":true,"Agent":"claude-code"}}
{"Action":"start","Package":"example.com/p"}
{"Action":"run","Package":"example.com/p","Test":"TestS"}
{"Action":"fail","Package":"example.com/p","Test":"TestS"}
{"Action":"fail","Package":"example.com/p"}
{"Action":"fail","Package":"gotest run","RunLevel":true,"Elapsed":1,"Result":{"ExitCode":1,"Packages":{"Passed":0,"Failed":1,"Skipped":0},"Tests":{"Passed":0,"Failed":1,"Skipped":0}}}
`

func (s *RunEventsTestSuite) TestTheTreeHoldsNoRunPackage(t *gotest.T) {
	tree := treeOf(t, runStream)
	mustLen(t, "packages", tree, 1)
	mustEq(t, "package", tree[0].Path, "example.com/p")
}

func (s *RunEventsTestSuite) TestAnImportPathNamedLikeTheRunStays(t *gotest.T) {
	// Only the run's own events, marked RunLevel, are left out.
	tree := treeOf(t, `{"Action":"start","Package":"gotest run"}
{"Action":"run","Package":"gotest run","Test":"TestS"}
{"Action":"pass","Package":"gotest run","Test":"TestS"}
{"Action":"pass","Package":"gotest run"}
`)
	mustLen(t, "packages", tree, 1)
}

func (s *RunEventsTestSuite) TestRunOfReadsTheManifestAndTheExitCode(t *gotest.T) {
	events, err := gotestspec.ParseEvents(strings.NewReader(runStream))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	run, ok := gotestspec.RunOf(events)
	if !ok {
		fatalf(t, "RunOf found no run")
	}
	mustEq(t, "version", run.Manifest.Version, "v1.2.3")
	mustEq(t, "commit", run.Manifest.Commit, "abc1234")
	if run.Manifest.Dirty == nil || !*run.Manifest.Dirty {
		t.Errorf("dirty = %v, want true", run.Manifest.Dirty)
	}
	if run.ExitCode == nil {
		fatalf(t, "no exit code")
	}
	mustEq(t, "exit code", *run.ExitCode, 1)
}

func (s *RunEventsTestSuite) TestRunOfAStreamWithoutAManifest(t *gotest.T) {
	cut := runStream[strings.Index(runStream, "\n")+1:]
	events, err := gotestspec.ParseEvents(strings.NewReader(cut))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	if _, ok := gotestspec.RunOf(events); ok {
		t.Errorf("RunOf found a run in a stream that opens none")
	}
}

func (s *RunEventsTestSuite) TestRunOfAStreamTheRunDidNotClose(t *gotest.T) {
	open := runStream[:strings.LastIndex(strings.TrimRight(runStream, "\n"), "\n")+1]
	events, err := gotestspec.ParseEvents(strings.NewReader(open))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	run, ok := gotestspec.RunOf(events)
	if !ok {
		fatalf(t, "RunOf found no run")
	}
	if run.ExitCode != nil {
		t.Errorf("exit code = %d, want none", *run.ExitCode)
	}
}
