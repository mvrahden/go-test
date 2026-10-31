package caseless

import "github.com/mvrahden/go-test/pkg/gotest"

// HelperTestSuite is named like a suite but declares no case: refused.
type HelperTestSuite struct{ Prefix string }

func (s *HelperTestSuite) name(n string) string { return s.Prefix + n }

// RealTestSuite uses the helper through a named field.
type RealTestSuite struct {
	helper HelperTestSuite
}

func (s *RealTestSuite) TestNames(t *gotest.T) {
	s.helper.Prefix = "x-"
	gotest.Equal(t, "x-a", s.helper.name("a"))
}
