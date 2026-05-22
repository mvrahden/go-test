package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type OrderTestSuite struct{}

func (s *OrderTestSuite) BeforeEach(t *gotest.T) {}
func (s *OrderTestSuite) AfterEach(t *gotest.T)  {}
func (s *OrderTestSuite) TestOne(t *gotest.T)    {}
