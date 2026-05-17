package repository

import (
	"context"
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type DatabaseFixture struct {
	mu    sync.Mutex
	store map[string]any
}

func (f *DatabaseFixture) FixtureConfig() gotest.FixtureConfig {
	return gotest.DefaultFixtureConfig()
}

func (f *DatabaseFixture) BeforeAll(_ context.Context) error {
	f.store = make(map[string]any)
	return nil
}

func (f *DatabaseFixture) AfterAll(_ context.Context) error {
	f.store = nil
	return nil
}

func (f *DatabaseFixture) BeforeEach(_ context.Context) error {
	return nil
}

func (f *DatabaseFixture) AfterEach(_ context.Context) error {
	return nil
}

func (f *DatabaseFixture) Put(key string, value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[key] = value
}

func (f *DatabaseFixture) Get(key string) (any, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.store[key]
	return v, ok
}

func (f *DatabaseFixture) Delete(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.store, key)
}

func (f *DatabaseFixture) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.store)
}
