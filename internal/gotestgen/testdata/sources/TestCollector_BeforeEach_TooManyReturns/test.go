package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type myCtx struct{}

type BadTestSuite struct{}

func (s *BadTestSuite) BeforeEach(t *gotest.T) (*myCtx, error) { return nil, nil }
func (s *BadTestSuite) TestOne(t *gotest.T) {}
