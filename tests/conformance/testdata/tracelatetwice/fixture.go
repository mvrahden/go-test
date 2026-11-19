package tracelatetwice

import "context"

// LateSharedFixture is read by two exclusive suites and by nothing before
// them: it starts for the first one's slot and stays up for the second.
type LateSharedFixture struct {
	State string
}

func (f *LateSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *LateSharedFixture) AfterAll(ctx context.Context) error { return nil }
