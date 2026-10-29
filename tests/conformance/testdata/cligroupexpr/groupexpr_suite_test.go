// Package cligroupexpr has a suite whose ExclusiveGroup is read from the
// environment, which the generator cannot resolve.
package cligroupexpr

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type GroupExprTestSuite struct{}

func (s *GroupExprTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.ExclusiveGroup = os.Getenv("GROUP")
	return cfg
}

func (s *GroupExprTestSuite) TestRuns(t *gotest.T) {}
