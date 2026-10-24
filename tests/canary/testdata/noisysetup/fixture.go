package noisysetup

import (
	"context"
	"fmt"
)

// NoiseSharedFixture writes to stdout while coming up: a fragment without a
// newline and a line shaped like a protocol entry.
type NoiseSharedFixture struct {
	Note string
}

func (f *NoiseSharedFixture) BeforeAll(ctx context.Context) error {
	fmt.Println(`{"key":"_done","error":"not from the protocol"}`)
	fmt.Print("setup says hello")
	f.Note = "arrived"
	return nil
}

func (f *NoiseSharedFixture) AfterAll(ctx context.Context) error { return nil }
