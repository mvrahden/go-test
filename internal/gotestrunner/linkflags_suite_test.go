package gotestrunner_test

import (
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinkFlagsTestSuite: test binaries are linked without the symbol table and
// debug information, as go test links them, unless something needs them.
type LinkFlagsTestSuite struct{}

func (s *LinkFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *LinkFlagsTestSuite) TestLinkFlags(t *gotest.T) {
	type row struct {
		Desc    string
		Build   []string
		Run     []string
		GOFLAGS string
		Kept    bool
		Strip   bool
	}
	for t, r := range gotest.Each(t, []row{
		{Desc: "no flags", Strip: true},
		{Desc: "-race", Build: []string{"-race"}, Strip: true},
		{Desc: "-cover", Build: []string{"-cover"}, Strip: true},
		{Desc: "-ldflags=", Build: []string{"-ldflags=-X main.v=1"}},
		{Desc: "-ldflags and a value", Build: []string{"-ldflags", "-X main.v=1"}},
		{Desc: "--ldflags", Build: []string{"--ldflags=-X main.v=1"}},
		{Desc: "GOFLAGS -ldflags", GOFLAGS: "-mod=mod -ldflags=-X=main.v=1"},
		{Desc: "GOFLAGS quoted -ldflags", GOFLAGS: `'-ldflags=-X main.v=1'`},
		{Desc: "GOFLAGS without it", GOFLAGS: "-mod=mod -tags=x", Strip: true},
		{Desc: "-cpuprofile", Run: []string{"-cpuprofile=cpu.out"}},
		{Desc: "-cpuprofile and a value", Run: []string{"-cpuprofile", "cpu.out"}},
		{Desc: "-test.memprofile", Run: []string{"-test.memprofile=m.out"}},
		{Desc: "-blockprofile", Run: []string{"-blockprofile=b.out"}},
		{Desc: "-mutexprofile", Run: []string{"-mutexprofile=m.out"}},
		{Desc: "-coverprofile is no reason", Run: []string{"-coverprofile=c.out"}, Strip: true},
		{Desc: "-trace is no reason", Run: []string{"-trace=t.out"}, Strip: true},
		{Desc: "--debug keeps the binaries", Kept: true},
	}) {
		got := gotestrunner.ExportLinkFlags(r.Build, r.Run, r.GOFLAGS, r.Kept)
		if r.Strip {
			gotest.Equal(t, append(slices.Clone(r.Build), "-ldflags=-s -w"), got)
		} else {
			gotest.Equal(t, r.Build, got)
		}
	}
}

func (s *LinkFlagsTestSuite) TestFuzzBuildsAreNotStripped(t *gotest.T) {
	target := gotestrunner.FuzzTarget{Package: "example.com/pkg", Dir: "/abs/pkg", Func: "FuzzFooTestSuite_FuzzBar"}
	cfg := gotestrunner.FuzzRunConfig{OverlayFlag: "-overlay=/tmp/overlay.json"}
	gotest.NotContains(t, strings.Join(gotestrunner.ExportBuildFuzzArgs(target, cfg, 5*time.Second), " "), "-ldflags")
}
