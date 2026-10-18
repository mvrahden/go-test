package lintgenreject_test

import "github.com/mvrahden/go-test/pkg/gotest"

// ResultTestSuite has a case that returns a value.
type ResultTestSuite struct{}

func (s *ResultTestSuite) TestLoads(t *gotest.T) error { return nil }
