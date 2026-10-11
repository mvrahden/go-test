package fixtures

import "context"

// BetaSharedFixture has no AfterAll: teardown is optional.
type BetaSharedFixture struct {
	Label string
	Count int
}

func (f *BetaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Label = "beta-shared"
	f.Count = 42
	return nil
}
