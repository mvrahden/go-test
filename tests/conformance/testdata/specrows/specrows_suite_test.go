// Package specrows declares Each tables whose rows a constant names and one
// whose row only a run can name; see the spec conformance suite.
package specrows

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/specrows/names"
)

type SpecRowsTestSuite struct{}

type row struct {
	Desc string
	Name string
}

func (s *SpecRowsTestSuite) TestRowsNamedByConstants(t *gotest.T) {
	for it := range gotest.Each(t, []row{{Name: names.PSuite}, {Name: "local " + "name"}}) {
		gotest.True(it, true)
	}
}

var runtimeName = "known late"

func (s *SpecRowsTestSuite) TestRowNamedAtRunTime(t *gotest.T) {
	for it := range gotest.Each(t, []row{{Name: "literal"}, {Name: runtimeName}}) {
		gotest.True(it, true)
	}
}
