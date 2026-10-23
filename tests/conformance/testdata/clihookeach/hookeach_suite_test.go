// Package clihookeach has a suite with a fuzz method whose BeforeEach and
// AfterEach call t.T(): they run per input with a real *testing.T.
package clihookeach

import "github.com/mvrahden/go-test/pkg/gotest"

type HookEachTestSuite struct{}

func (s *HookEachTestSuite) BeforeEach(t *gotest.T) { t.T().Log("input starts") }

func (s *HookEachTestSuite) AfterEach(t *gotest.T) { t.T().Log("input ends") }

func (s *HookEachTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
