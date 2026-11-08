package gotest_test

import (
	"os"
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// Sequential: Setenv.
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

func (s *BWrapperTestSuite) TestSetenvAndTempDir(t *gotest.T) {
	const key = "GOTEST_TEST_B_SETENV"
	t.When("a benchmark sets a variable and makes a temporary directory through its B", func(w *gotest.T) {
		var during, dir string
		var dirErr error
		testing.Benchmark(func(b *testing.B) {
			gb := gotest.NewB(b)
			gb.Setenv(key, "set")
			during = os.Getenv(key)
			dir = gb.TempDir()
			_, dirErr = os.Stat(dir)
			for gb.Loop() {
			}
		})
		w.It("both hold while the benchmark runs", func(it *gotest.T) {
			gotest.Equal(it, "set", during)
			gotest.NotEmpty(it, dir)
			gotest.NoError(it, dirErr)
		})
		w.It("the variable is restored and the directory removed after it", func(it *gotest.T) {
			_, set := os.LookupEnv(key)
			gotest.False(it, set)
			_, err := os.Stat(dir)
			gotest.True(it, os.IsNotExist(err), "%s outlived the benchmark", dir)
		})
	})
}
