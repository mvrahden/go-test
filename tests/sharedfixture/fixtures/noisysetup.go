package fixtures

import (
	"context"
	"fmt"
)

// NoisySetupSharedFixture writes to stdout while coming up — a fragment
// without a newline and a line shaped like a protocol entry — none of which
// is the runner's to read.
type NoisySetupSharedFixture struct {
	Marker string
}

func (f *NoisySetupSharedFixture) BeforeAll(ctx context.Context) error {
	fmt.Println(`{"key":"_done","error":"not from the protocol"}`)
	fmt.Print("setup says hello")
	f.Marker = "noisy"
	return nil
}

func (f *NoisySetupSharedFixture) AfterAll(ctx context.Context) error { return nil }
