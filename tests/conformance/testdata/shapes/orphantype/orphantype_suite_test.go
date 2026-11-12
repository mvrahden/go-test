package orphantype

import "github.com/mvrahden/go-test/pkg/gotest"

// UserSuite reads as a suite and is none: its name does not end in TestSuite.
type UserSuite struct{}

func (s *UserSuite) TestCreate(t *gotest.T) { gotest.Fail(t, "never runs") }

func (s *UserSuite) TestDelete(t *gotest.T) { gotest.Fail(t, "never runs") }

func (s *UserSuite) BenchmarkList(b *gotest.B) {}

// RealTestSuite runs beside it.
type RealTestSuite struct{}

func (s *RealTestSuite) TestRuns(t *gotest.T) {}
