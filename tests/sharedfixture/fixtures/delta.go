package fixtures

import "context"

// DeltaSharedFixture is referenced only by the exclusive tail suite
// (tests/sharedfixture/exclusive): under window scheduling the runner defers
// it past the parallel bulk and starts it at the bulk→tail barrier.
type DeltaSharedFixture struct {
	Stamp string
}

func (f *DeltaSharedFixture) BeforeAll(ctx context.Context) error {
	f.Stamp = "delta-shared"
	return nil
}

func (f *DeltaSharedFixture) AfterAll(ctx context.Context) error { return nil }
