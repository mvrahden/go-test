// Package clihookcalls has a suite with a fuzz method whose hooks call t.T(),
// which is nil under the fuzz wrapper; the generator refuses it.
package clihookcalls

import "github.com/mvrahden/go-test/pkg/gotest"

// HookCallsTestSuite declares AfterEach above BeforeAll, so source order and
// hook order differ.
type HookCallsTestSuite struct{}

func (s *HookCallsTestSuite) AfterEach(t *gotest.T) { _ = t.T() }

func (s *HookCallsTestSuite) BeforeAll(t *gotest.T) { _ = t.T() }

func (s *HookCallsTestSuite) AfterAll(t *gotest.T) {}

func (s *HookCallsTestSuite) TestOne(t *gotest.T) {}

func (s *HookCallsTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
