package nameistest

import "github.com/mvrahden/go-test/pkg/gotest"

// NameTestSuite has helpers whose names only begin like a case.
type NameTestSuite struct{ checks int }

func (s *NameTestSuite) TestRuns(t *gotest.T) {
	s.Testify(t)
	s.Benchmarked(t)
	s.Fuzzy(t)
	gotest.Equal(t, 3, s.checks)
}

func (s *NameTestSuite) Testify(t *gotest.T)     { s.checks++ }
func (s *NameTestSuite) Benchmarked(t *gotest.T) { s.checks++ }
func (s *NameTestSuite) Fuzzy(t *gotest.T)       { s.checks++ }
