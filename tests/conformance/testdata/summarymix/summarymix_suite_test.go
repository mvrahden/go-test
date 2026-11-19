// Package summarymix passes, fails and skips as SUMMARYMIX asks; see the
// summary conformance suite.
package summarymix

import (
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MixTestSuite struct{}

func skipsAll(t *gotest.T) {
	if os.Getenv("SUMMARYMIX") == "skips" {
		t.Skipf("everything skips")
	}
}

func (s *MixTestSuite) TestPasses(t *gotest.T) { skipsAll(t) }

func (s *MixTestSuite) TestFailsOnDemand(t *gotest.T) {
	skipsAll(t)
	if os.Getenv("SUMMARYMIX") == "fail" {
		gotest.Fail(t, "asked to fail")
	}
}

func (s *MixTestSuite) TestSkips(t *gotest.T) {
	skipsAll(t)
	t.Skipf("not for this run")
}

type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string { return "SUMMARYMIX_DB is not set" }

func (s *GuardedTestSuite) TestNeedsTheDatabase(t *gotest.T) {}
