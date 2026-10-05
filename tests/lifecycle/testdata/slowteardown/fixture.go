// Package slowteardown has a fixture whose teardown outlives -test.timeout.
package slowteardown

import (
	"context"
	"time"
)

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }

func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	time.Sleep(30 * time.Second)
	return nil
}
