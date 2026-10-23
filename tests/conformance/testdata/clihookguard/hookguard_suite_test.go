// Package clihookguard has a suite with a fuzz method whose BeforeAll and
// AfterAll check t.T() for nil before using it; it runs.
package clihookguard

import "github.com/mvrahden/go-test/pkg/gotest"

type HookGuardTestSuite struct{}

func (s *HookGuardTestSuite) BeforeAll(t *gotest.T) {
	if tt := t.T(); tt != nil {
		tt.Log("setup")
	}
}

func (s *HookGuardTestSuite) AfterAll(t *gotest.T) {
	if t.T() == nil {
		return
	}
	t.T().Log("teardown")
}

func (s *HookGuardTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
