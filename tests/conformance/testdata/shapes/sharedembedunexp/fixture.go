package sharedembedunexp

import "context"

// info is unexported: a named field of it would not reach the suites either.
type info struct{ Addr string }

// StateSharedFixture embeds it.
type StateSharedFixture struct {
	info
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "mem://state"
	return nil
}
