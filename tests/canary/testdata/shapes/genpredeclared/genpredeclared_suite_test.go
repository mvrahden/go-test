package genpredeclared

import "github.com/mvrahden/go-test/pkg/gotest"

// any takes a name Go predeclares, which the harness uses as Go's.
type any struct{}

// PredeclaredTestSuite would run in a package whose any is its own.
type PredeclaredTestSuite struct{}

func (s *PredeclaredTestSuite) TestRuns(t *gotest.T) {}
