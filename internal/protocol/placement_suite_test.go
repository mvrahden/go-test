package protocol_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlacementTestSuite covers how a go test name is placed under its parent
// from the order of the stream's events.
type PlacementTestSuite struct{}

func (s *PlacementTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// place2 feeds lines "action test" to one Placer and returns a placement per line.
func place2(lines ...string) []protocol.Placement {
	p := protocol.NewPlacer()
	out := make([]protocol.Placement, 0, len(lines))
	for _, l := range lines {
		action, test, _ := strings.Cut(l, " ")
		out = append(out, p.Place(action, test))
	}
	return out
}

// place returns the placements the run events got, by test name.
func place(lines ...string) map[string]protocol.Placement {
	p := protocol.NewPlacer()
	got := map[string]protocol.Placement{}
	for _, l := range lines {
		action, test, _ := strings.Cut(l, " ")
		if pl := p.Place(action, test); action == "run" {
			got[test] = pl
		}
	}
	return got
}

func (s *PlacementTestSuite) TestATrailingSlashIsPartOfTheLabel(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/gets_./", "pass S/M/gets_./")
	gotest.Equal(t, protocol.Placement{Parent: "S/M", Own: "gets_./", Written: "gets_./", Instance: 1}, got["S/M/gets_./"])
}

func (s *PlacementTestSuite) TestALeadingSlashIsPartOfTheLabel(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M//usr/local/go", "pass S/M//usr/local/go")
	gotest.Equal(t, "S/M", got["S/M//usr/local/go"].Parent)
	gotest.Equal(t, "/usr/local/go", got["S/M//usr/local/go"].Own)
}

func (s *PlacementTestSuite) TestAGroupEndingInASlashHoldsItsChildren(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/reads_specs/", "run S/M/reads_specs//holds_clauses")
	gotest.Equal(t, "S/M/reads_specs/", got["S/M/reads_specs//holds_clauses"].Parent)
	gotest.Equal(t, "holds_clauses", got["S/M/reads_specs//holds_clauses"].Own)
}

func (s *PlacementTestSuite) TestAFinishedSiblingAdoptsNothing(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/.", "pass S/M/.", "run S/M/./pkg")
	gotest.Equal(t, "S/M", got["S/M/./pkg"].Parent)
	gotest.Equal(t, "./pkg", got["S/M/./pkg"].Own)
}

func (s *PlacementTestSuite) TestAPausedSiblingAdoptsNothing(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/.", "pause S/M/.", "run S/M/./pkg", "cont S/M/.")
	gotest.Equal(t, "S/M", got["S/M/./pkg"].Parent)
}

func (s *PlacementTestSuite) TestARepeatedNameDropsItsSuffix(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/a", "pass S/M/a", "run S/M/a#01")
	gotest.Equal(t, protocol.Placement{Parent: "S/M", Own: "a#01", Written: "a", Repeat: 1, Instance: 1}, got["S/M/a#01"])
}

func (s *PlacementTestSuite) TestAnExplicitSuffixIsNoRepeat(t *gotest.T) {
	got := place("run S", "run S/M", "run S/M/a#01", "run F", "run F/seed#0")
	gotest.Equal(t, "a#01", got["S/M/a#01"].Written)
	gotest.Equal(t, 0, got["S/M/a#01"].Repeat)
	gotest.Equal(t, "seed#0", got["F/seed#0"].Written)
}

func (s *PlacementTestSuite) TestAWhenAndAnItSpellingTheSamePath(t *gotest.T) {
	// When "a" › It "b", then It "a/b": testing names the second a/b#01
	got := place("run S", "run S/M", "run S/M/a", "run S/M/a/b", "pass S/M/a/b", "pass S/M/a", "run S/M/a/b#01")
	gotest.Equal(t, protocol.Placement{Parent: "S/M", Own: "a/b#01", Written: "a/b", Repeat: 1, Instance: 1}, got["S/M/a/b#01"])
}

func (s *PlacementTestSuite) TestATwinSuiteIsASecondInstance(t *gotest.T) {
	evs := place2("run S", "run S/TestCreate", "pass S/TestCreate", "pass S",
		"run S", "run S/TestCreate#01", "run S/TestCreate#01/when_x", "pass S/TestCreate#01", "pass S")
	gotest.Equal(t, 2, evs[5].Instance) // index = event position
	gotest.Equal(t, "TestCreate", evs[5].Written)
	gotest.Equal(t, 1, evs[5].Repeat)
	gotest.Equal(t, 2, evs[6].Instance)
	gotest.Equal(t, 2, evs[8].Instance) // the second top-level verdict goes to the open instance
}

func (s *PlacementTestSuite) TestASubBenchmarkStaysOpenUntilItsParentEnds(t *gotest.T) {
	// Sub-benchmarks report no verdict, so B/a/x stays open until B ends: a
	// later sibling labelled "x/..." would be read as its child.
	got := place("run B", "run B/a", "run B/a/x", "run B/a/b", "fail B", "run C")
	gotest.Equal(t, "B/a", got["B/a/b"].Parent)

	// B's verdict closes B/a and B/a/x too: neither adopts a later name.
	got = place("run B", "run B/a", "run B/a/x", "fail B", "run B/a/x/y")
	gotest.True(t, got["B/a/x/y"].Guessed)
}

func (s *PlacementTestSuite) TestANameWithoutAnOpenParentFallsBack(t *gotest.T) {
	got := place("run S/M/a/b")
	gotest.Equal(t, protocol.Placement{Parent: "S/M/a", Own: "b", Written: "b", Instance: 1, Guessed: true}, got["S/M/a/b"])
}

// The fallback split cuts at single slashes only; a run of slashes stays in
// its level, and a trailing slash is dropped.
func (s *PlacementTestSuite) TestSplitTestPathCutsAtSingleSlashes(t *gotest.T) {
	for sub, tc := range gotest.Each(t, []struct {
		Desc string
		path string
		want []string
	}{
		{Desc: "single segment", path: "TestFoo", want: []string{"TestFoo"}},
		{Desc: "two segments", path: "TestFoo/bar", want: []string{"TestFoo", "bar"}},
		{Desc: "three segments", path: "TestFoo/bar/baz", want: []string{"TestFoo", "bar", "baz"}},
		{Desc: "consecutive double slash preserved", path: "TestFoo/https://example.com", want: []string{"TestFoo", "https://example.com"}},
		{Desc: "consecutive triple slash preserved", path: "TestFoo/a///b", want: []string{"TestFoo", "a///b"}},
		{Desc: "empty string", path: "", want: nil},
		{Desc: "trailing slash", path: "TestFoo/bar/", want: []string{"TestFoo", "bar"}},
		{Desc: "mixed normal and double slash", path: "TestSuite/method/https://host/path", want: []string{"TestSuite", "method", "https://host", "path"}},
	}) {
		gotest.Equal(sub, tc.want, protocol.SplitTestPath(tc.path))
	}
}

func (s *PlacementTestSuite) TestAStampPlacesWithoutTheOrder(t *gotest.T) {
	gotest.Equal(t, protocol.Placement{Parent: "S", Own: "TestCreate#01", Written: "TestCreate", Repeat: 1, Instance: 2},
		protocol.PlacementFromStamp("S/TestCreate#01", protocol.Stamp{Subtest: "TestCreate#01", Repeat: 1, Instance: 2}))
	gotest.Equal(t, protocol.Placement{Parent: "S/M", Own: "a#01", Written: "a#01", Instance: 1},
		protocol.PlacementFromStamp("S/M/a#01", protocol.Stamp{Subtest: "a#01"}))
	gotest.Equal(t, protocol.Placement{Parent: "S/M", Own: "./pkg", Written: "./pkg", Instance: 1},
		protocol.PlacementFromStamp("S/M/./pkg", protocol.Stamp{Subtest: "./pkg"}))
	gotest.Equal(t, protocol.Placement{Own: "S", Written: "S", Instance: 1},
		protocol.PlacementFromStamp("S", protocol.Stamp{Subtest: "S"}))
	// A stamp the name does not end in is no placement.
	gotest.True(t, protocol.PlacementFromStamp("S/M/b", protocol.Stamp{Subtest: "a"}).Guessed)
}
