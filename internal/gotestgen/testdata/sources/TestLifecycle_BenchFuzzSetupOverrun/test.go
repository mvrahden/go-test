package testpkg

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// BenchOverrunTestSuite blows its SetupTimeout in BeforeAll.
type BenchOverrunTestSuite struct{}

func (s *BenchOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 100 * time.Millisecond}
}

func (s *BenchOverrunTestSuite) BeforeAll(t *gotest.T) { time.Sleep(400 * time.Millisecond) }

func (s *BenchOverrunTestSuite) BenchmarkNoop(b *gotest.B) {
	for b.Loop() {
	}
}

// FuzzOverrunTestSuite blows its SetupTimeout in BeforeAll.
type FuzzOverrunTestSuite struct{}

func (s *FuzzOverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{SetupTimeout: 100 * time.Millisecond}
}

func (s *FuzzOverrunTestSuite) BeforeAll(t *gotest.T) { time.Sleep(400 * time.Millisecond) }

func (s *FuzzOverrunTestSuite) FuzzNoop(f *gotest.F) {
	f.Add(1)
	f.Fuzz(func(t *gotest.T, n int) {})
}
