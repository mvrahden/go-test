package gotest_test

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type BWrapperTestSuite struct{}

func (s *BWrapperTestSuite) TestNewB(t *gotest.T) {
	t.It("exposes the underlying *testing.B and satisfies assertions", func(it *gotest.T) {
		res := testing.Benchmark(func(b *testing.B) {
			gb := gotest.NewB(b)
			gotest.NotZero(it, gb.B())
			gotest.Equal(it, b, gb.B())
			for gb.Loop() {
				_ = 1 + 1
			}
		})
		gotest.Greater(it, res.N, 0)
	})
}
