package zerodurations

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

// NoMarkerTestSuite declares no SuiteConfig, the baseline the others compare to.
type NoMarkerTestSuite struct{}

func (s *NoMarkerTestSuite) BeforeAll(t *gotest.T) { report("nomarker beforeall", t.Context()) }
func (s *NoMarkerTestSuite) AfterAll(t *gotest.T)  { report("nomarker afterall", t.Context()) }
func (s *NoMarkerTestSuite) TestMethod(t *gotest.T) {
	report("nomarker test", t.Context())
}

// PartialLiteralTestSuite leaves both durations at zero.
type PartialLiteralTestSuite struct{}

func (s *PartialLiteralTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{FailFast: true}
}

func (s *PartialLiteralTestSuite) BeforeAll(t *gotest.T) { report("partial beforeall", t.Context()) }
func (s *PartialLiteralTestSuite) AfterAll(t *gotest.T)  { report("partial afterall", t.Context()) }
func (s *PartialLiteralTestSuite) TestMethod(t *gotest.T) {
	report("partial test", t.Context())
}

// NoDeadlineTestSuite disables both deadlines.
type NoDeadlineTestSuite struct{}

func (s *NoDeadlineTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: gotest.NoDeadline, SetupTimeout: gotest.NoDeadline}
}

func (s *NoDeadlineTestSuite) BeforeAll(t *gotest.T) { report("nodeadline beforeall", t.Context()) }
func (s *NoDeadlineTestSuite) AfterAll(t *gotest.T)  { report("nodeadline afterall", t.Context()) }
func (s *NoDeadlineTestSuite) TestMethod(t *gotest.T) {
	report("nodeadline test", t.Context())
}
