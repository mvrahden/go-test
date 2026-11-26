package skippable

import "github.com/mvrahden/go-test/pkg/gotest"

// Both fuzz targets never call f.Fuzz, which the generator rejects.
type StubTestSuite struct{}

func (s *StubTestSuite) FuzzShown(f *gotest.F) {}

func (s *StubTestSuite) FuzzQuiet(f *gotest.F) {} //nolint:fuzz-seed // suppressed, so only the rejection is left
