// Package lcbhook has a suite that hangs in its BeforeAll, before any method
// runs, and one that hangs in a method.
package lcbhook

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type HookTestSuite struct{}

func (s *HookTestSuite) BeforeAll(t *gotest.T)     { hang() }
func (s *HookTestSuite) AfterAll(t *gotest.T)      {}
func (s *HookTestSuite) TestNeverRuns(t *gotest.T) {}

type MethodTestSuite struct{}

func (s *MethodTestSuite) TestHang(t *gotest.T) { hang() }

// hang ignores its context; LCB_WARM makes it return at once, for a run
// that only builds.
func hang() {
	if os.Getenv("LCB_WARM") == "" {
		time.Sleep(time.Hour)
	}
}
