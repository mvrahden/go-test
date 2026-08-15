package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Marker struct{}

type NoLeavesFuzzTestSuite struct{}

func (s *NoLeavesFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *NoLeavesFuzzTestSuite) FuzzMarker(f *gotest.F) {
	f.Add(Marker{})
	gotest.Fuzz(f, func(t *gotest.T, m Marker) {})
}
