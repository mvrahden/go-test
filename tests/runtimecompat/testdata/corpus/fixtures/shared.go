package fixtures

import (
	"context"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreSharedFixture struct {
	Addr string           // transfer: serialized to the suites
	Log  *strings.Builder // local: assigned by Hydrate
}

func (f *StoreSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}

func (f *StoreSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "mem://store"
	return nil
}

func (f *StoreSharedFixture) AfterAll(ctx context.Context) error { return nil }

func (f *StoreSharedFixture) Hydrate(ctx context.Context) error {
	f.Log = &strings.Builder{}
	return nil
}

func (f *StoreSharedFixture) Dehydrate(ctx context.Context) error { return nil }
