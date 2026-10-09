// Package holds is driven by plain go test, not the gotest runner: every
// suite shares one process, and each top-level function holds the fixture
// for itself, whatever -run, -count or -shuffle selected.
package holds

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func logLine(event string) {
	f, err := os.OpenFile(os.Getenv("HOLDS_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintln(f, event)
}

// Up reports whether this variant's fixture is set up and not yet torn down.
var Up bool

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	if Up {
		return errors.New("set up twice without a teardown")
	}
	logLine("setup")
	Up = true
	return nil
}

func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	logLine("teardown")
	Up = false
	if os.Getenv("HOLDS_FAIL_TEARDOWN") == "1" {
		return errors.New("teardown refused")
	}
	return nil
}
