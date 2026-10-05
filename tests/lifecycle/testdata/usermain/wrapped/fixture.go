// Package wrapped binds a fixture and declares its own TestMain.
package wrapped

import (
	"context"
	"fmt"
	"os"
)

func logLine(event string) {
	f, err := os.OpenFile(os.Getenv("TESTMAIN_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, event)
}

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error { logLine("setup"); return nil }
func (f *LedgerFixture) AfterAll(ctx context.Context) error  { logLine("teardown"); return nil }
