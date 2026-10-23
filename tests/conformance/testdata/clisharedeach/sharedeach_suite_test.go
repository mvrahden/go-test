package clisharedeach

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/clisharedeach/shared"
)

type SharedEachTestSuite struct {
	Cache *shared.CacheSharedFixture
}

func (s *SharedEachTestSuite) TestBody(t *gotest.T) {}
