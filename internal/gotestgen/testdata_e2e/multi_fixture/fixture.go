package multifixture

import (
	"context"
	"sync/atomic"
)

var DatabaseTornDown atomic.Bool

type DatabaseFixture struct{}

func (f *DatabaseFixture) BeforeAll(ctx context.Context) error {
	DatabaseTornDown.Store(false)
	return nil
}

func (f *DatabaseFixture) AfterAll(ctx context.Context) error {
	DatabaseTornDown.Store(true)
	return nil
}

type CacheFixture struct{}

func (f *CacheFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *CacheFixture) AfterAll(ctx context.Context) error  { return nil }

type ServiceFixture struct {
	DB    *DatabaseFixture
	Cache *CacheFixture
}

func (f *ServiceFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ServiceFixture) AfterAll(ctx context.Context) error  { return nil }
