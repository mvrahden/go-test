// Package fuzzseedtype has a fuzz target whose seed is of the wrong type; the
// canary reads where the failure is reported.
package fuzzseedtype

import "github.com/mvrahden/go-test/pkg/gotest"

type SeedTypeTestSuite struct{}

func (s *SeedTypeTestSuite) FuzzCount(f *gotest.F) {
	f.Add(int64(5))
	f.Fuzz(func(t *gotest.T, n int) {})
}
