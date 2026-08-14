package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type BenchTestSuite struct{}

func (s *BenchTestSuite) BeforeEach(t *gotest.T) {}
func (s *BenchTestSuite) TestOne(t *gotest.T)    {}
func (s *BenchTestSuite) BenchmarkParse(b *gotest.B) {
	for b.Loop() {
	}
}
func (s *BenchTestSuite) X_BenchmarkOld(b *gotest.B) {
	for b.Loop() {
	}
}
