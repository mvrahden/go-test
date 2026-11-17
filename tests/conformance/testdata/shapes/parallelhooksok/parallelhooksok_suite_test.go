package parallelhooksok

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StubFixture declares per-method hooks that never use the fixture.
type StubFixture struct{ Opened bool }

func (f *StubFixture) BeforeAll(ctx context.Context) error {
	f.Opened = true
	return nil
}

func (f *StubFixture) BeforeEach(ctx context.Context) error { return nil }

func (*StubFixture) AfterEach(ctx context.Context) error { return ctx.Err() }

// StubHooksTestSuite is parallel with two methods.
type StubHooksTestSuite struct{ Stub *StubFixture }

func (s *StubHooksTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *StubHooksTestSuite) TestA(t *gotest.T) { gotest.True(t, s.Stub.Opened) }

func (s *StubHooksTestSuite) TestB(t *gotest.T) { gotest.True(t, s.Stub.Opened) }

// CountFixture counts the methods in its hook.
type CountFixture struct{ n int }

func (f *CountFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *CountFixture) BeforeEach(ctx context.Context) error {
	f.n++
	return nil
}

// SingleTestSuite is parallel on paper: one method, so no hook runs beside another.
type SingleTestSuite struct{ Count *CountFixture }

func (s *SingleTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *SingleTestSuite) TestOnly(t *gotest.T) { gotest.Equal(t, 1, s.Count.n) }
