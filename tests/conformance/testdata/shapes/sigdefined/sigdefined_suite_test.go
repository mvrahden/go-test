package sigdefined

import "github.com/mvrahden/go-test/pkg/gotest"

type ATestSuite struct{}

func (s *ATestSuite) TestRuns(t *gotest.T) {}

// CopyTestSuite is a defined type: it has none of ATestSuite's methods.
type CopyTestSuite ATestSuite
