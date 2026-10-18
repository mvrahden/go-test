package store

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type StoreTestSuite struct {
	suite.Suite
}

func (s *StoreTestSuite) SetupSuite() {}

func (s StoreTestSuite) TestValue() {}

func (s *StoreTestSuite) F_TestCreate() {
	s.T().Cleanup(func() {})
	s.T().Errorf("boom")
	s.T().Helper()
}

func (s *StoreTestSuite) X_AfterEach() {}

func (s *StoreTestSuite) BeforAll() {}

func TestStore(t *testing.T) { //nolint:stdlib-test
	suite.Run(t, new(StoreTestSuite))
}
