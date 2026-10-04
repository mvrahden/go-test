package testmainuser

import "context"

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { return nil }
