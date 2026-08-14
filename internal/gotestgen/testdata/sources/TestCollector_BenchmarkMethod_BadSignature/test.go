package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BadBenchTestSuite struct{}

func (s *BadBenchTestSuite) BenchmarkBad(t *gotest.T) {}
