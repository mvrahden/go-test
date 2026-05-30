package sharedfixdep

import "context"

type AlphaSharedFixture struct {
	Value string
}

func (f *AlphaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Value = "alpha"
	return nil
}
func (f *AlphaSharedFixture) AfterAll(ctx context.Context) error { return nil }

type BetaSharedFixture struct {
	Alpha *AlphaSharedFixture
	Label string
}

func (f *BetaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Label = "beta-" + f.Alpha.Value
	return nil
}
func (f *BetaSharedFixture) AfterAll(ctx context.Context) error { return nil }
