package gencontext

import (
	stdctx "context"

	gt "github.com/mvrahden/go-test/pkg/gotest"
)

// These declarations take the names of the packages the harness imports.
type context struct{ name string }

var (
	testing       = context{name: "testing"}
	time          = context{name: "time"}
	gotestruntime = context{name: "gotestruntime"}
	gotestfuzz    = context{name: "gotestfuzz"}
	strings       = context{name: "strings"}
	strconv       = context{name: "strconv"}
	math          = context{name: "math"}
)

func gotest() context { return context{name: "gotest"} }

// ClockFixture is a package fixture.
type ClockFixture struct{ Ticks int }

func (f *ClockFixture) BeforeAll(ctx stdctx.Context) error { f.Ticks++; return nil }
func (f *ClockFixture) AfterAll(ctx stdctx.Context) error  { return nil }

// point fans out to leaves, so the harness reaches for its fuzz helpers.
type point struct {
	X int
	Y float64
	L string
}

// ShadowsTestSuite runs in a package whose names shadow the harness imports.
type ShadowsTestSuite struct {
	Clock *ClockFixture
}

func (s *ShadowsTestSuite) TestRuns(t *gt.T) {
	gt.Equal(t, 1, s.Clock.Ticks)
	gt.Equal(t, "gotest", gotest().name)
	gt.Equal(t, "testing", testing.name)
	gt.NotZero(t, time.name+gotestruntime.name+gotestfuzz.name+strings.name+strconv.name+math.name)
}

func (s *ShadowsTestSuite) FuzzPoint(f *gt.F) {
	f.Add(point{X: 1, Y: 1.5, L: "a"})
	f.Fuzz(func(t *gt.T, p point) {
		gt.Equal(t, p.X, p.X)
	})
}
