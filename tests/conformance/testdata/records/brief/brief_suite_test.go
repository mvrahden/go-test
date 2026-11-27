package brief_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BriefTestSuite struct{}

func (s *BriefTestSuite) TestLong(t *gotest.T) {
	gotest.True(t, testing.Short(), "the long path fails")
}
