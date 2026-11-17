// Package locshadow fails an assertion inside a helper that lives under a
// directory named pkg/gotest.
package locshadow

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	checks "github.com/mvrahden/go-test/tests/conformance/testdata/locshadow/pkg/gotest"
)

type ShadowTestSuite struct{}

func (s *ShadowTestSuite) TestThroughAHelper(t *gotest.T) {
	checks.Positive(t, -1) // the caller's line
}
