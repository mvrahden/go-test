package fixturechain

import "context"

// TokenSharedFixture carries one value into the test process.
type TokenSharedFixture struct {
	Token string
}

func (f *TokenSharedFixture) BeforeAll(ctx context.Context) error {
	f.Token = "hydrated"
	return nil
}

func (f *TokenSharedFixture) AfterAll(ctx context.Context) error { return nil }

// ParentFixture is the only fixture that names the shared fixture.
type ParentFixture struct {
	Shared *TokenSharedFixture
}

func (f *ParentFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ParentFixture) AfterAll(ctx context.Context) error  { return nil }

// ChildFixture reaches it through ParentFixture.
type ChildFixture struct {
	Parent *ParentFixture
}

func (f *ChildFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *ChildFixture) AfterAll(ctx context.Context) error  { return nil }
