// Package fixtureorder has suites whose fixture with only an AfterEach
// comes before, or after, a fixture whose BeforeEach fails; the canary reads
// which AfterEach ran.
package fixtureorder

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

var markMu sync.Mutex

// mark appends one line to the markers file the canary reads back.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// AheadFixture has only an AfterEach and comes before BrokenFixture.
type AheadFixture struct{}

func (f *AheadFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *AheadFixture) AfterEach(ctx context.Context) error { mark("ahead AfterEach"); return nil }

// BrokenFixture's BeforeEach fails.
type BrokenFixture struct {
	Ahead *AheadFixture
}

func (f *BrokenFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BrokenFixture) BeforeEach(ctx context.Context) error {
	return errors.New("the broken fixture is not ready")
}

// FailingFixture's BeforeEach fails.
type FailingFixture struct{}

func (f *FailingFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *FailingFixture) BeforeEach(ctx context.Context) error {
	return errors.New("the failing fixture is not ready")
}

// BehindFixture has only an AfterEach and comes after FailingFixture.
type BehindFixture struct {
	Failing *FailingFixture
}

func (f *BehindFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *BehindFixture) AfterEach(ctx context.Context) error { mark("behind AfterEach"); return nil }
