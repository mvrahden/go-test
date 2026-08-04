package about_test

import (
	"runtime/debug"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// VersionTestSuite tests the version resolution the skill's version gate keys
// on: ldflags stamp wins, then build info, with replace/source builds marked.
type VersionTestSuite struct{}

func (s *VersionTestSuite) TestResolveVersion(t *gotest.T) {
	moduleBI := &debug.BuildInfo{Main: debug.Module{Version: "v1.26.0"}}
	replacedBI := &debug.BuildInfo{Main: debug.Module{Version: "v1.26.0", Replace: &debug.Module{Path: "../go-test"}}}
	develBI := &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}
	emptyBI := &debug.BuildInfo{Main: debug.Module{}}

	for sub, tc := range gotest.Each(t, []struct {
		Desc    string
		Stamped string
		BI      *debug.BuildInfo
		OK      bool
		Want    string
	}{
		{Desc: "ldflags stamp wins over build info", Stamped: "v1.2.3", BI: replacedBI, OK: true, Want: "v1.2.3"},
		{Desc: "no build info falls back to dev", Stamped: "dev", BI: nil, OK: false, Want: "dev"},
		{Desc: "replace directive is marked", Stamped: "dev", BI: replacedBI, OK: true, Want: "dev (replace directive)"},
		{Desc: "devel build is a source checkout", Stamped: "dev", BI: develBI, OK: true, Want: "dev (source checkout)"},
		{Desc: "blank module version is a source checkout", Stamped: "dev", BI: emptyBI, OK: true, Want: "dev (source checkout)"},
		{Desc: "tool-directive build reports the module version", Stamped: "dev", BI: moduleBI, OK: true, Want: "v1.26.0"},
	}) {
		gotest.Equal(sub, tc.Want, about.ExportResolveVersion(tc.Stamped, tc.BI, tc.OK))
	}
}
