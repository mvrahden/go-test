// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestspec_test //nolint:fail-guard

import (
	"strings"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EventTestSuite covers the go test -json event parser that feeds every
// replay verdict; what it drops decides which failures are ever seen.
type EventTestSuite struct{}

func (s *EventTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type eventCtx struct{}

func (s *EventTestSuite) BeforeEach(t *gotest.T) *eventCtx { return &eventCtx{} }

func parseEvents(t *gotest.T, input string) []gotestspec.TestEvent {
	events, err := gotestspec.ParseEvents(strings.NewReader(input))
	if err != nil {
		fatalf(t, "ParseEvents: %v", err)
	}
	return events
}

func (s *EventTestSuite) TestParseEvents_Empty(t *gotest.T, _ *eventCtx) {
	mustLen(t, "events", parseEvents(t, ""), 0)
}

func (s *EventTestSuite) TestParseEvents_BlankLinesSkipped(t *gotest.T, _ *eventCtx) {
	events := parseEvents(t, "\n\n"+`{"Action":"run","Package":"p","Test":"TestFoo"}`+"\n\n")
	mustLen(t, "events", events, 1)
	mustEq(t, "test", events[0].Test, "TestFoo")
}

func (s *EventTestSuite) TestParseEvents_MalformedJSONSkipped(t *gotest.T, _ *eventCtx) {
	events := parseEvents(t, `not json at all
{"Action":"run","Package":"p","Test":"TestA"}
{truncated
{"Action":"pass","Package":"p","Test":"TestA","Elapsed":0.01}`)
	mustLen(t, "events", events, 2)
	mustEq(t, "events[0].Action", events[0].Action, gotestspec.ActionRun)
	mustEq(t, "events[1].Action", events[1].Action, gotestspec.ActionPass)
}

func (s *EventTestSuite) TestParseEvents_OutputCaptured(t *gotest.T, _ *eventCtx) {
	events := parseEvents(t, `{"Action":"output","Package":"p","Test":"TestFoo","Output":"hello world\n"}`)
	mustLen(t, "events", events, 1)
	mustEq(t, "output", events[0].Output, "hello world\n")
}

func (s *EventTestSuite) TestParseEvents_BuildEvents(t *gotest.T, _ *eventCtx) {
	events := parseEvents(t, `{"ImportPath":"m/p [m/p.test]","Action":"build-output","Output":"x_test.go:5:2: undefined: f\n"}
{"ImportPath":"m/p [m/p.test]","Action":"build-fail"}
{"Action":"fail","Package":"m/p","Elapsed":0,"FailedBuild":"m/p [m/p.test]"}`)
	mustLen(t, "events", events, 3)
	mustEq(t, "events[0].Action", events[0].Action, gotestspec.ActionBuildOutput)
	mustEq(t, "events[0].ImportPath", events[0].ImportPath, "m/p [m/p.test]")
	mustEq(t, "events[1].Action", events[1].Action, gotestspec.ActionBuildFail)
	mustEq(t, "events[2].FailedBuild", events[2].FailedBuild, "m/p [m/p.test]")
}
