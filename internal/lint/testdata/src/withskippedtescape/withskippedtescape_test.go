package withskippedtescape //nolint:stdlib-test

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SkippedEscapeTestSuite struct{}

// With t-escape skipped project-wide its claims must not stand fail-guard
// down — the active rule takes the construct.
func (s *SkippedEscapeTestSuite) TestGuard(it *gotest.T) {
	err := errors.New("boom")
	if err != nil { // want `use NoError instead of if\+Fatal for error nil check`
		it.T().Fatal(err)
	}
}
