package standalone

import (
	"io"

	"github.com/mvrahden/go-test/internal/integration/sharedfixture/fixtures"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AlphaTestSuite struct {
	Alpha *fixtures.AlphaSharedFixture
}

func (s *AlphaTestSuite) TestDataPathSet(t *gotest.T) {
	gotest.NotEqual(t, "", s.Alpha.DataPath)
}

func (s *AlphaTestSuite) TestHandleHydrated(t *gotest.T) {
	gotest.NotEmpty(t, s.Alpha.Handle)
	_, err := s.Alpha.Handle.Seek(0, 0)
	gotest.NoError(t, err)
	b, err := io.ReadAll(s.Alpha.Handle)
	gotest.NoError(t, err)
	gotest.Equal(t, "alpha-data", string(b))
}

type MultiTestSuite struct {
	Alpha *fixtures.AlphaSharedFixture
	Beta  *fixtures.BetaSharedFixture
}

func (s *MultiTestSuite) TestAlphaAvailable(t *gotest.T) {
	gotest.NotEqual(t, "", s.Alpha.DataPath)
	gotest.NotEmpty(t, s.Alpha.Handle)
}

func (s *MultiTestSuite) TestBetaAvailable(t *gotest.T) {
	gotest.Equal(t, "beta-shared", s.Beta.Label)
	gotest.Equal(t, 42, s.Beta.Count)
}

type PlainTestSuite struct{}

func (s *PlainTestSuite) TestPlainWorks(t *gotest.T) {
	gotest.True(t, true)
}
