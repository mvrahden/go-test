// Package assertcountred fails leaves that made no assertion, by Errorf and
// by a panic, and one that declares an empty reason.
package assertcountred

import "github.com/mvrahden/go-test/pkg/gotest"

type FailTestSuite struct{}

func (s *FailTestSuite) TestFails(t *gotest.T) { t.Errorf("failed by hand") }

type PanicTestSuite struct{}

func (s *PanicTestSuite) TestPanics(t *gotest.T) { panic("no assertion, a panic") }

type EmptyReasonTestSuite struct{}

func (s *EmptyReasonTestSuite) TestEmpty(t *gotest.T) { t.AssertsBy(" \t") }
