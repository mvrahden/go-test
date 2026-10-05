// Package panics panics, as PANICS selects, in the places a hook on the
// panicking frame cannot reach. Each must still release the fixture.
package panics

import (
	"context"
	"fmt"
	"os"
)

func logLine(event string) {
	f, err := os.OpenFile(os.Getenv("PANICS_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, event)
}

// Is reports whether PANICS selects scenario.
func Is(scenario string) bool { return os.Getenv("PANICS") == scenario }

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { logLine("setup"); return nil }
func (f *LedgerFixture) AfterAll(ctx context.Context) error  { logLine("teardown"); return nil }
