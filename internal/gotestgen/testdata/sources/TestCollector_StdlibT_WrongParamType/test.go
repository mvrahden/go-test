package testpkg

import "fmt"

type BadTestSuite struct{}

func (s *BadTestSuite) TestBad(f fmt.Stringer) {}
