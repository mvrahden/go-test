package hooks

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GridTestSuite binds the fixture; its hooks and bodies mark themselves and
// misbehave where HOOKS_AT names them.
type GridTestSuite struct {
	Grid *GridFixture
}

func (s *GridTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 300 * time.Millisecond
	return cfg
}

// suiteHook marks the hook and goes wrong when told to: a failed assertion,
// a panic, an overrun or a wait.
func suiteHook(t *gotest.T, hook string) {
	mark(hook)
	if !misbehaves(hook) {
		return
	}
	switch os.Getenv("HOOKS_MODE") {
	case "panic":
		panic(hook + " panicked")
	case "overrun":
		time.Sleep(time.Second)
	case "wait":
		waitOn(t.Context(), hook)
	default:
		gotest.Fail(t, "%s failed", hook)
	}
}

func (s *GridTestSuite) BeforeAll(t *gotest.T)  { suiteHook(t, "suite.BeforeAll") }
func (s *GridTestSuite) AfterAll(t *gotest.T)   { suiteHook(t, "suite.AfterAll") }
func (s *GridTestSuite) BeforeEach(t *gotest.T) { suiteHook(t, "suite.BeforeEach") }
func (s *GridTestSuite) AfterEach(t *gotest.T)  { suiteHook(t, "suite.AfterEach") }
func (s *GridTestSuite) TestBody(t *gotest.T)   { suiteHook(t, "body") }
func (s *GridTestSuite) TestNext(t *gotest.T)   { mark("next") }
