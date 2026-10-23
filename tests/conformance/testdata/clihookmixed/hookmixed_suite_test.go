// Package clihookmixed has a suite with fuzz and benchmark methods whose
// BeforeEach and BeforeAll call t.T(); only the benchmark wrapper makes
// BeforeEach's nil, so the error names the benchmark methods.
package clihookmixed

import "github.com/mvrahden/go-test/pkg/gotest"

type HookMixedTestSuite struct{}

func (s *HookMixedTestSuite) BeforeEach(t *gotest.T) { _ = t.T() }

func (s *HookMixedTestSuite) BeforeAll(t *gotest.T) { _ = t.T() }

func (s *HookMixedTestSuite) TestOne(t *gotest.T) {}

func (s *HookMixedTestSuite) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}

func (s *HookMixedTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
