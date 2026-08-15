package broken

import "github.com/mvrahden/go-test/pkg/gotest"

// Deliberately uncompilable: `undefinedHelper` does not exist. A package that
// fails to load is booked as a failed package and the rest of the run
// continues, so this fixture proves one broken package cannot take the corpus
// down with it. It carries no failing behavior, only a package-level verdict.
type BrokenTestSuite struct{}

func (s *BrokenTestSuite) TestDoesNotCompile(t *gotest.T) {
	t.It("never gets to run", func(t *gotest.T) {
		gotest.Equal(t, 1, undefinedHelper())
	})
}
