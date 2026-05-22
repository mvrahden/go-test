package testpkg

import "testing"

type HookTestSuite struct{}

func (s *HookTestSuite) BeforeAll(t *testing.T)  {}
func (s *HookTestSuite) AfterAll(t *testing.T)   {}
func (s *HookTestSuite) BeforeEach(t *testing.T) {}
func (s *HookTestSuite) AfterEach(t *testing.T)  {}
func (s *HookTestSuite) TestOne(t *testing.T)    {}
