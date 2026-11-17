package orphantagsnone

import "github.com/mvrahden/go-test/pkg/gotest"

// unnamed is named by no file, constrained or not.
type unnamed struct{}

func (u *unnamed) TestNobody(t *gotest.T) {}

type UnitTestSuite struct{}

func (s *UnitTestSuite) TestA(t *gotest.T) {}
