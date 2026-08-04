package fixtures

import (
	"context"
	"os"
	"time"
)

// CrashTeardownSharedFixture crashes the whole process during teardown when
// armed — not by panicking inside AfterAll, where the harness contains it, but
// on a goroutine the fixture owns, the classic shutdown bug: a background
// worker sending on a channel its AfterAll just closed. No suite references
// it — the runner's own tests drive it directly, because a fixture that
// crashes on purpose must not fail the repository's own run.
//
// It is configured through the environment because the runner starts the setup
// program as a subprocess. Without EnvCrashTeardownArm its AfterAll succeeds.
type CrashTeardownSharedFixture struct {
	Marker string

	work chan struct{}
}

const EnvCrashTeardownArm = "GOTEST_TEST_CRASH_TEARDOWN_ARM"

func (f *CrashTeardownSharedFixture) BeforeAll(ctx context.Context) error {
	if os.Getenv(EnvCrashTeardownArm) == "" {
		return nil
	}
	f.work = make(chan struct{})
	go func() {
		// Blocks until AfterAll closes the channel, then panics with "send on
		// closed channel" — an unrecovered panic on a fixture-owned goroutine,
		// outside every containment frame the harness has.
		f.work <- struct{}{}
	}()
	return nil
}

func (f *CrashTeardownSharedFixture) AfterAll(ctx context.Context) error {
	if f.work == nil {
		return nil
	}
	close(f.work)
	// The panic on the worker goroutine aborts the process; this sleep only
	// guarantees AfterAll is still running when it does, so the crash lands in
	// the teardown window rather than after a clean exit.
	time.Sleep(5 * time.Second)
	return nil
}
