package zerodurationsfixture

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// report prints the deadline ctx carries, rounded so scheduling noise does not show.
func report(label string, ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if !ok {
		fmt.Printf("MARK:%s no deadline\n", label)
		return
	}
	fmt.Printf("MARK:%s deadline %s\n", label, time.Until(deadline).Round(10*time.Second))
}

// PartialFixture leaves its Timeout at zero.
type PartialFixture struct{}

func (f *PartialFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{RetryDelay: time.Second}
}

func (f *PartialFixture) BeforeAll(ctx context.Context) error {
	report("partial fixture beforeall", ctx)
	return nil
}

func (f *PartialFixture) AfterAll(ctx context.Context) error {
	report("partial fixture afterall", ctx)
	return nil
}

// NoDeadlineFixture disables its deadline.
type NoDeadlineFixture struct{}

func (f *NoDeadlineFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.FixtureConfig{Timeout: gotest.NoDeadline}
}

func (f *NoDeadlineFixture) BeforeAll(ctx context.Context) error {
	report("nodeadline fixture beforeall", ctx)
	return nil
}

func (f *NoDeadlineFixture) AfterAll(ctx context.Context) error {
	report("nodeadline fixture afterall", ctx)
	return nil
}

// FixtureBoundTestSuite leaves its durations at zero while bound to both fixtures.
type FixtureBoundTestSuite struct {
	*PartialFixture
	*NoDeadlineFixture
}

func (s *FixtureBoundTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{FailFast: true}
}

func (s *FixtureBoundTestSuite) BeforeAll(t *gotest.T) { report("bound beforeall", t.Context()) }
func (s *FixtureBoundTestSuite) TestMethod(t *gotest.T) {
	report("bound test", t.Context())
}
