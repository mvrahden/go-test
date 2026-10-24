// Package protocol holds shared fixtures whose setup writes what the
// shared-fixture protocol must not mistake for its own lines.
package protocol

import (
	"context"
	"fmt"
	"strings"
)

// StdoutSharedFixture prints to stdout without a newline while coming up.
type StdoutSharedFixture struct {
	Note string
}

func (f *StdoutSharedFixture) BeforeAll(ctx context.Context) error {
	fmt.Print("setup says hello")
	f.Note = "printed"
	return nil
}

func (f *StdoutSharedFixture) AfterAll(ctx context.Context) error { return nil }

// BigSharedFixture carries 2 MiB of state across processes.
type BigSharedFixture struct {
	Blob string
}

func (f *BigSharedFixture) BeforeAll(ctx context.Context) error {
	f.Blob = strings.Repeat("x", 2<<20)
	return nil
}

func (f *BigSharedFixture) AfterAll(ctx context.Context) error { return nil }

// LogSharedFixture logs a JSON-shaped line while coming up.
type LogSharedFixture struct {
	Note string
}

func (f *LogSharedFixture) BeforeAll(ctx context.Context) error {
	fmt.Println(`{"Action":"pass","Package":"example.com/bogus","Test":"TestBogus"}`)
	f.Note = "logged"
	return nil
}

func (f *LogSharedFixture) AfterAll(ctx context.Context) error { return nil }
