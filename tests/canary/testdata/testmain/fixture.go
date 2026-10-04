// Package testmain is driven by plain go test, not the gotest runner: every
// suite shares one process, so the fixture's teardown has to wait for the
// last of them, whatever -run, -count or -shuffle selected.
package testmain

import (
	"context"
	"errors"
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

// Up reports whether this variant's fixture is set up and not yet torn down.
var Up bool

type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	logLine("setup")
	Up = true
	return nil
}

func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	logLine("teardown")
	Up = false
	if os.Getenv("TESTMAIN_FAIL_TEARDOWN") == "1" {
		return errors.New("teardown refused")
	}
	return nil
}
