// Package clihookbench has a suite with a benchmark method whose hooks call
// t.T(), which is nil under the benchmark wrapper; the generator refuses it.
package clihookbench

import "github.com/mvrahden/go-test/pkg/gotest"

// HookBenchTestSuite declares AfterAll above BeforeEach, so source order and
// hook order differ.
type HookBenchTestSuite struct{}

func (s *HookBenchTestSuite) AfterAll(t *gotest.T) { _ = t.T() }

func (s *HookBenchTestSuite) BeforeAll(t *gotest.T) {}

func (s *HookBenchTestSuite) BeforeEach(t *gotest.T) { _ = t.T() }

func (s *HookBenchTestSuite) TestOne(t *gotest.T) {}

func (s *HookBenchTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
