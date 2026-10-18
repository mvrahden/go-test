package withhookcall

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}

func (f *DBFixture) BeforeAll(context.Context) error  { return nil }
func (f *DBFixture) BeforeEach(context.Context) error { return nil }

type StoreFixture struct {
	DB    *DBFixture
	Cache *CacheSharedFixture
}

func (f *StoreFixture) BeforeAll(ctx context.Context) error {
	return f.DB.BeforeAll(ctx) // want `BeforeAll is called by the harness; calling it here runs it twice`
}

func (f *StoreFixture) BeforeEach(ctx context.Context) error { return nil }

// client has methods named like hooks, but is no fixture.
type client struct{}

func (c *client) BeforeEach() {}

type HookTestSuite struct {
	Store  *StoreFixture
	client *client
}

func (s *HookTestSuite) BeforeAll(t *gotest.T) {}
func (s *HookTestSuite) AfterAll(t *gotest.T)  {}

func (s *HookTestSuite) BeforeEach(t *gotest.T) {
	gotest.NoError(t, s.Store.BeforeEach(t.Context())) // want `BeforeEach is called by the harness; calling it here runs it twice`
}

func (s *HookTestSuite) TestReset(t *gotest.T) {
	s.AfterAll(t)                                         // want `AfterAll is called by the harness; calling it here runs it twice`
	gotest.NoError(t, s.Store.Cache.Hydrate(t.Context())) // want `Hydrate is called by the harness; calling it here runs it twice`
	s.client.BeforeEach()
	local := &DBFixture{}
	gotest.NoError(t, local.BeforeAll(t.Context()))
	s.helper()
}

func (s *HookTestSuite) TestSuppressed(t *gotest.T) {
	s.AfterAll(t) //nolint:fixture-hook-call
}

func (s *HookTestSuite) helper() {}

// A function outside any suite or fixture method is not the harness's.
func standalone(ctx context.Context, f *DBFixture) error { return f.BeforeAll(ctx) }
