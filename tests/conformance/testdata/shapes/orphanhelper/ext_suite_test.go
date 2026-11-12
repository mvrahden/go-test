package orphanhelper_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/shapes/orphanhelper"
)

type ExtTestSuite struct{}

func (s *ExtTestSuite) TestCallsExported(t *gotest.T) {
	(&orphanhelper.Exported{}).TestFromOutside(t)
}
