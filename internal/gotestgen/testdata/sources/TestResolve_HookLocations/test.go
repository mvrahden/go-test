package TestResolve_HookLocations

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
	"testpkg/extfixtures"
)

// LocalFixture declares its BeforeEach.
type LocalFixture struct{}

func (f *LocalFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *LocalFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *LocalFixture) AfterAll(ctx context.Context) error   { return nil }

type HookLocationsTestSuite struct {
	Local  *LocalFixture
	Remote *extfixtures.RemoteFixture
	Store  *StoreSharedFixture
}

func (s *HookLocationsTestSuite) TestA(t *gotest.T) {}

// StoreSharedFixture declares the hooks a shared fixture runs in the test
// process.
type StoreSharedFixture struct {
	Addr string
}

func (f *StoreSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *StoreSharedFixture) Hydrate(ctx context.Context) error   { return nil }
func (f *StoreSharedFixture) Dehydrate(ctx context.Context) error { return nil }
