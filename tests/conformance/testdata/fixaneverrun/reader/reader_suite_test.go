package reader_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixaneverrun/fixtures"
)

// ReaderTestSuite is the fixture's reader that runs.
type ReaderTestSuite struct {
	A *fixtures.ASharedFixture
}

func (s *ReaderTestSuite) TestReadsA(t *gotest.T) {
	gotest.Equal(t, "up", s.A.State)
	fixtures.Mark("reader ran")
}
