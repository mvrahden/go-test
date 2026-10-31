// Package other declares the type an alias names beside a type named like
// the alias.
package other

import "github.com/mvrahden/go-test/pkg/gotest"

// Thing holds the case the alias runs.
type Thing struct{}

func (t *Thing) TestRemote(tt *gotest.T) {}

// ThingTestSuite is named like the alias, and is another type.
type ThingTestSuite struct{}

func (s *ThingTestSuite) TestOther(t *gotest.T) {}
