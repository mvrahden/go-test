package lintfocusspread

func (s *F_SpreadTestSuite) double() int { return 2 * s.n }

var _ = (&F_SpreadTestSuite{n: 1}).double
