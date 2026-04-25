package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type UserSuite struct {
	suite.Suite
	db string
}

func (s *UserSuite) SetupSuite() {
	s.db = "connected"
}

func (s *UserSuite) TearDownSuite() {
	s.db = ""
}

func (s *UserSuite) SetupTest() {}

func (s *UserSuite) TearDownTest() {}

func (s *UserSuite) TestCreate() {
	s.Require().Equal("connected", s.db)
	s.Require().NoError(nil)
	assert.True(s.T(), true)
}

func (s *UserSuite) TestDelete() {
	s.Assert().NotNil(s.db)
}

func TestUserSuite(t *testing.T) {
	suite.Run(t, new(UserSuite))
}
