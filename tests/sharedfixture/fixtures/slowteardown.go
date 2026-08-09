package fixtures

import (
	"context"
	"os"
	"time"
)

// SlowTeardownSharedFixture releases slowly, the way a fixture that stops a
// container or drains a connection pool does. It exists to prove the shared
// fixture process is given its configured teardown budget rather than some
// shorter one imposed by the runner, so no suite references it — the runner's
// own tests drive it directly.
//
// It is configured through the environment because the runner starts the setup
// program as a subprocess: EnvSlowTeardownDelay is how long AfterAll takes, and
// EnvSlowTeardownMarker is the file it writes once it has finished. A missing
// marker means teardown was cut short.
type SlowTeardownSharedFixture struct {
	Marker string
}

const (
	EnvSlowTeardownDelay  = "GOTEST_TEST_SLOW_TEARDOWN_DELAY"
	EnvSlowTeardownMarker = "GOTEST_TEST_SLOW_TEARDOWN_MARKER"
)

func (f *SlowTeardownSharedFixture) BeforeAll(ctx context.Context) error {
	f.Marker = os.Getenv(EnvSlowTeardownMarker)
	return nil
}

func (f *SlowTeardownSharedFixture) AfterAll(ctx context.Context) error {
	if d, err := time.ParseDuration(os.Getenv(EnvSlowTeardownDelay)); err == nil && d > 0 {
		time.Sleep(d)
	}
	if f.Marker == "" {
		return nil
	}
	return os.WriteFile(f.Marker, []byte("released"), 0o600)
}
