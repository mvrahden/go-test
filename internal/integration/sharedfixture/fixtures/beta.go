package fixtures

import "context"

type BetaSharedFixture struct {
	Label string
	Count int
}

func (f *BetaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Label = "beta-shared"
	f.Count = 42
	return nil
}

func (f *BetaSharedFixture) AfterAll(ctx context.Context) error {
	return nil
}
