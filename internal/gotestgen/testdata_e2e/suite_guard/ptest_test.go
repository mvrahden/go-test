package guardpkg

import (
	"context"
	"os"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type GuardedTestSuite struct{}

func (s *GuardedTestSuite) SuiteGuard() string {
	if os.Getenv("ENABLE_GUARDED_SUITE") == "" {
		return "ENABLE_GUARDED_SUITE not set"
	}
	return ""
}

func (s *GuardedTestSuite) BeforeAll(t *gotest.T) {}
func (s *GuardedTestSuite) AfterAll(t *gotest.T)  {}

func (s *GuardedTestSuite) TestHello(t *gotest.T) { HelloWorld() }
func (s *GuardedTestSuite) TestWorld(t *gotest.T) { HelloWorld() }

type GuardFixture struct{ Ready bool }

func (f *GuardFixture) BeforeAll(ctx context.Context) error {
	f.Ready = true
	return nil
}

// GuardedFixtureBoundTestSuite proves SuiteGuard is honored for fixture-bound
// suites: if the guard were ignored, TestMustNotRun would fail the build's tests.
type GuardedFixtureBoundTestSuite struct {
	Fixture *GuardFixture
}

func (s *GuardedFixtureBoundTestSuite) SuiteGuard() string {
	if os.Getenv("ENABLE_GUARDED_SUITE") == "" {
		return "ENABLE_GUARDED_SUITE not set"
	}
	return ""
}

func (s *GuardedFixtureBoundTestSuite) TestMustNotRun(t *gotest.T) {
	gotest.Fail(t, "guarded fixture-bound suite must be skipped, not run")
}
