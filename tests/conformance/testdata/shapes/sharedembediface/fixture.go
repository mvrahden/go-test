package sharedembediface

import "context"

// Clock is a handle the suites would call.
type Clock interface{ Now() int64 }

// ClockSharedFixture embeds it.
type ClockSharedFixture struct {
	Clock
}

func (f *ClockSharedFixture) BeforeAll(ctx context.Context) error { return nil }
