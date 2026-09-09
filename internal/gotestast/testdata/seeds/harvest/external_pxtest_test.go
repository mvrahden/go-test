// External (pxtest) test package for the ptest-vs-pxtest boundary
// regression test: FuzzParse lives in the internal (ptest) "harvest"
// package; this literal table lives in the external "harvest_test" package.
// HarvestSeeds, called with the ptest *packages.Package, must never see it —
// harvesting never crosses *packages.Package boundaries.
package harvest_test

import (
	"github.com/mvrahden/go-test/internal/gotestast/testdata/seeds/harvest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type ExternalParseTestSuite struct{}

func (s *ExternalParseTestSuite) TestParseTableExternal(t *gotest.T) {
	type parseCase struct {
		Desc  string
		Input string
	}
	for t, tc := range gotest.Each(t, []parseCase{
		{"external-only literal", "external-only"},
	}) {
		t.It("parses", func(t *gotest.T) {
			harvest.Parse(tc.Input)
		})
	}
}
