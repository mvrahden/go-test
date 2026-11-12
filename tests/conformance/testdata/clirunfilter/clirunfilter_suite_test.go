package clirunfilter

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AlphaTestSuite struct{}

// The hooks name the function they run under: a fuzz wrapper opens the suite
// once more, even when no seed matches.
func (s *AlphaTestSuite) BeforeAll(t *gotest.T) { mark("Alpha.BeforeAll " + t.Name()) }
func (s *AlphaTestSuite) AfterAll(t *gotest.T)  { mark("Alpha.AfterAll " + t.Name()) }
func (s *AlphaTestSuite) TestOne(t *gotest.T)   { mark("Alpha.TestOne") }
func (s *AlphaTestSuite) TestTwo(t *gotest.T)   { mark("Alpha.TestTwo") }

func (s *AlphaTestSuite) FuzzSeed(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) { mark("Alpha.FuzzSeed") })
}

type BetaTestSuite struct{}

func (s *BetaTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Exclusive = true
	return cfg
}

func (s *BetaTestSuite) BeforeAll(t *gotest.T) { mark("Beta.BeforeAll") }
func (s *BetaTestSuite) AfterAll(t *gotest.T)  {}
func (s *BetaTestSuite) TestOne(t *gotest.T)   { mark("Beta.TestOne") }

type StateTestSuite struct {
	State *StateSharedFixture
}

func (s *StateTestSuite) TestState(t *gotest.T) {
	mark("State.TestState")
	gotest.Equal(t, "up", s.State.Value)
}
