// Package teardownfail holds a shared fixture that starts cleanly and fails to
// release what it holds, so a test can drive a run that fails after its last
// verdict is in.
package teardownfail

import (
	"context"
	"errors"
	"os"
)

// ProbeSharedFixture fails its teardown when GOTEST_TEARDOWN_FAIL is set.
type ProbeSharedFixture struct {
	State string
}

func (f *ProbeSharedFixture) BeforeAll(ctx context.Context) error {
	f.State = "up"
	return nil
}

func (f *ProbeSharedFixture) AfterAll(ctx context.Context) error {
	if os.Getenv("GOTEST_TEARDOWN_FAIL") == "1" {
		return errors.New("the fixture would not release what it holds")
	}
	return nil
}
