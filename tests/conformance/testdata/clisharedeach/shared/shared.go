// Package shared declares a shared fixture with per-test hooks, which
// shared fixtures do not have.
package shared

import (
	"context"
	"errors"
)

// CacheSharedFixture declares a BeforeEach.
type CacheSharedFixture struct {
	Addr string
}

func (f *CacheSharedFixture) BeforeAll(ctx context.Context) error { f.Addr = "cache"; return nil }

func (f *CacheSharedFixture) BeforeEach(ctx context.Context) error {
	return errors.New("never called")
}
