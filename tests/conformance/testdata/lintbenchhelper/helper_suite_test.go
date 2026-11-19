package lintbenchhelper

import "github.com/mvrahden/go-test/pkg/gotest"

// runLoop holds the measured loop for every benchmark that hands it b.
func runLoop(b *gotest.B, fn func()) {
	for b.Loop() {
		fn()
	}
}

// sized reaches the loop through runLoop.
func sized(b *gotest.B, n int) { runLoop(b, func() { _ = make([]byte, n) }) }

// check only asserts on b.
func check(b *gotest.B) { gotest.True(b, true) }

type HelperTestSuite struct{}

func (s *HelperTestSuite) BenchmarkDelegated(b *gotest.B) { runLoop(b, func() {}) }

func (s *HelperTestSuite) BenchmarkTwoDeep(b *gotest.B) { sized(b, 64) }

func (s *HelperTestSuite) BenchmarkAsserted(b *gotest.B) { check(b) }
