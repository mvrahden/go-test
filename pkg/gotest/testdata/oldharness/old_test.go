package oldharness

import "github.com/mvrahden/go-test/pkg/gotest"

// Pair is a struct-typed fuzz input, so the harness carries a fan.
type Pair struct {
	Name  string
	Count int
}

// OldTestSuite has the shapes whose harness calls the user package's
// constructors: a benchmark and a struct-typed fuzz target.
type OldTestSuite struct{}

func (s *OldTestSuite) BenchmarkAdd(b *gotest.B) {
	for b.Loop() {
		_ = Pair{Name: "a", Count: 1}
	}
}

func (s *OldTestSuite) FuzzPair(f *gotest.F) {
	f.Add(Pair{Name: "a", Count: 1})
	f.Fuzz(func(t *gotest.T, p Pair) {
		gotest.Equal(t, p, p)
	})
}
