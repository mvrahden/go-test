// Package contract holds a generic suite with a benchmark method whose hooks
// call t.T(), which is nil under the benchmark wrapper.
package contract

import "github.com/mvrahden/go-test/pkg/gotest"

// HookContractTestSuite declares AfterEach above BeforeAll, so source order
// and hook order differ.
type HookContractTestSuite[V any] struct{}

func (s *HookContractTestSuite[V]) AfterEach(t *gotest.T) { _ = t.T() }

func (s *HookContractTestSuite[V]) BeforeAll(t *gotest.T) { _ = t.T() }

func (s *HookContractTestSuite[V]) TestOne(t *gotest.T) {}

func (s *HookContractTestSuite[V]) BenchmarkOne(b *gotest.B) {
	for b.Loop() {
	}
}
