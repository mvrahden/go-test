// Package fzseedtype has a fuzz target whose seed is of the wrong type.
package fzseedtype

import "github.com/mvrahden/go-test/pkg/gotest"

type SeedTypeTestSuite struct{}

// FuzzCount takes an int; its seed is an int64.
func (s *SeedTypeTestSuite) FuzzCount(f *gotest.F) {
	f.Add(int64(5))
	f.Fuzz(func(t *gotest.T, n int) {})
}
