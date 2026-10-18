package withtestifysuite

import (
	"testing"

	"github.com/stretchr/testify/suite" // want `testify import github.com/stretchr/testify/suite — consider migrating to gotest`
)

// StoreTestSuite is a testify suite: no gotest rule reads it, and the
// generator skips it.
type StoreTestSuite struct {
	suite.Suite
}

func (s *StoreTestSuite) BeforeAll() {}

func (s *StoreTestSuite) BeforeEach() {}

func (s StoreTestSuite) TestValue() {}

func (s *StoreTestSuite) TestCreate() {
	s.T().Cleanup(func() {})
	s.T().Parallel()
	s.T().Errorf("boom")
	s.T().Helper()
	s.T().Run("sub", func(t *testing.T) {})
}

func (s *StoreTestSuite) F_TestFocused() {}

func (s *StoreTestSuite) X_AfterEach() {}

func (s *StoreTestSuite) BeforAll() {}

func TestStore(t *testing.T) { //nolint:stdlib-test
	suite.Run(t, new(StoreTestSuite))
}
