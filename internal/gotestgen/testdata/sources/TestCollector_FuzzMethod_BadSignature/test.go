package testpkg

import "testing"

type BadFuzzTestSuite struct{}

func (s *BadFuzzTestSuite) FuzzBad(f *testing.F) {}
