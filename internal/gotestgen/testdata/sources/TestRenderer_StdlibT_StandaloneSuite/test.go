package testpkg

import "testing"

type PlainTestSuite struct{}

func (s *PlainTestSuite) BeforeEach(t *testing.T) {}
func (s *PlainTestSuite) AfterEach(t *testing.T)  {}
func (s *PlainTestSuite) TestFoo(t *testing.T)    {}
func (s *PlainTestSuite) TestBar(t *testing.T)    {}
