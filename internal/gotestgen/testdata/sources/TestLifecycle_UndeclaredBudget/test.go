package undeclaredbudget

import "github.com/mvrahden/go-test/pkg/gotest"

// NoConfigTestSuite declares no SuiteConfig method at all, so the defaults bound
// its contexts but nothing holds it to them by verdict.
type NoConfigTestSuite struct{}

func (s *NoConfigTestSuite) TestSomething(t *gotest.T) {
	t.It("passes", func(it *gotest.T) {
		gotest.True(it, true)
	})
}
