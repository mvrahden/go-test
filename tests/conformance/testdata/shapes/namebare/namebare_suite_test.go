package namebare

import "github.com/mvrahden/go-test/pkg/gotest"

// BareTestSuite has methods named by a case prefix alone that take its T, B or F.
type BareTestSuite struct{}

func (s *BareTestSuite) TestRuns(t *gotest.T) {}

func (s *BareTestSuite) Test(t *gotest.T) {}

func (s *BareTestSuite) Benchmark(b *gotest.B) {}

func (s *BareTestSuite) Fuzz(f *gotest.F) {}
