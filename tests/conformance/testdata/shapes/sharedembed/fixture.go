package sharedembed

import "context"

// Info is state the fixture means to hand to its suites.
type Info struct{ Addr string }

// StateSharedFixture embeds it.
type StateSharedFixture struct {
	Info
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "mem://state"
	return nil
}
