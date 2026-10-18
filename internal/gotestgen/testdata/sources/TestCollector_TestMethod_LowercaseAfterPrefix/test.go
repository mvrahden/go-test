package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type NameTestSuite struct{}

func (s *NameTestSuite) TestRuns(t *gotest.T)    {}
func (s *NameTestSuite) Test_under(t *gotest.T)  {}
func (s *NameTestSuite) Testify(t *gotest.T)     {}
func (s *NameTestSuite) Benchmarked(t *gotest.T) {}
func (s *NameTestSuite) Fuzzy(t *gotest.T)       {}
func (s *NameTestSuite) X_Testing(t *gotest.T)   {}
