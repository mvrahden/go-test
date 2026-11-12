// Package benchnumbered holds a sub-benchmark whose name ends in a number;
// see the bench conformance suite.
package benchnumbered

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type NumberedTestSuite struct{}

func (s *NumberedTestSuite) BenchmarkCopy(b *gotest.B) {
	b.B().Run("size-1024", func(sb *testing.B) {
		for sb.Loop() {
		}
	})
}
