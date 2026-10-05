package panics_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/lifecycle/testdata/panics"
)

// StandaloneTestSuite binds no fixture, in the external test package, and
// panics after the internal package set the binary's fixture up.
type StandaloneTestSuite struct{}

func (s *StandaloneTestSuite) TestBoom(t *gotest.T) {
	if panics.Is("xtest") {
		panic("external package panic")
	}
}
