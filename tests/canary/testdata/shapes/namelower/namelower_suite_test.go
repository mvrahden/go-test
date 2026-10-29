package namelower

import "github.com/mvrahden/go-test/pkg/gotest"

// LowerTestSuite has a method taking a T whose name go test would not run.
type LowerTestSuite struct{}

func (s *LowerTestSuite) TestRuns(t *gotest.T) {}

func (s *LowerTestSuite) Testcreate(t *gotest.T) {}
