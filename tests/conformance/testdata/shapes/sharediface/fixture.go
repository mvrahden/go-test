package sharediface

import "context"

// Clock is an interface with a method: JSON cannot rebuild its value.
type Clock interface{ Now() int64 }

// ClockSharedFixture transfers a field of that type.
type ClockSharedFixture struct {
	Clock Clock
}

func (f *ClockSharedFixture) BeforeAll(ctx context.Context) error { return nil }
