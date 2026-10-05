// Package interrupt is stopped from outside while a fixture-bound test runs:
// the fixture must be torn down before the process ends.
package interrupt

import (
	"context"
	"fmt"
	"os"
	"time"
)

func logLine(event string) {
	f, err := os.OpenFile(os.Getenv("INTERRUPT_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, event)
}

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	logLine("setup")
	return nil
}

// AfterAll takes a moment, as releasing a real resource does: whoever waits
// for the process has to wait for it too.
func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	time.Sleep(200 * time.Millisecond)
	fmt.Println("released")
	logLine("teardown")
	return nil
}
