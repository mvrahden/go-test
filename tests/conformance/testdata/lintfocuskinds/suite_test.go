package lintfocuskinds

import (
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type KindsTestSuite struct{}

func (s *KindsTestSuite) TestUpper(t *gotest.T) { gotest.Equal(t, "A", strings.ToUpper("a")) }

func (s *KindsTestSuite) F_BenchmarkUpper(b *gotest.B) {
	for b.Loop() {
		strings.ToUpper("a")
	}
}

func (s *KindsTestSuite) F_FuzzUpper(f *gotest.F) {
	f.Add("a")
	f.Fuzz(func(t *gotest.T, in string) {
		gotest.Equal(t, strings.ToUpper(in), strings.ToUpper(strings.ToUpper(in)))
	})
}
