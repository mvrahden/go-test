package testmainxonly

import "context"

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *LedgerFixture) AfterAll(ctx context.Context) error  { return nil }
