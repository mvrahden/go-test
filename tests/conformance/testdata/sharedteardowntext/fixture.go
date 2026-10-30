// Package sharedteardowntext binds a shared fixture whose AfterAll writes the
// line a failed AfterAll is reported with, then returns nil; see the fixtures
// conformance suite.
package sharedteardowntext

import (
	"context"
	"log"
)

// WordySharedFixture releases cleanly while logging a failure's words.
type WordySharedFixture struct {
	State string
}

func (f *WordySharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *WordySharedFixture) AfterAll(ctx context.Context) error {
	log.Printf("CacheSharedFixture.AfterAll failed: only words")
	log.Printf("CacheSharedFixture.AfterAll panicked: only words")
	return nil
}
