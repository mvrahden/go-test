package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type DBFixture struct{}

func (f *DBFixture) BeforeAll(t *gotest.T) {} // wrong: should be (ctx context.Context) error

// UsesTestSuite binds the fixture.
type UsesTestSuite struct {
	Fix *DBFixture
}

func (s *UsesTestSuite) TestRuns(t *gotest.T) {}
