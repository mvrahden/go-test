package fixtures

import (
	"context"
	"os"
)

// PanickySetupSharedFixture panics in BeforeAll when armed, the way a fixture
// that indexes an empty response or dereferences a half-built client does. No
// suite references it — the runner's own tests drive it directly, because a
// fixture that fails on purpose must not fail the repository's own run.
//
// It is configured through the environment because the runner starts the setup
// program as a subprocess. Without EnvPanicSetupArm its BeforeAll succeeds.
type PanickySetupSharedFixture struct {
	Marker string
}

const EnvPanicSetupArm = "GOTEST_TEST_PANIC_SETUP_ARM"

func (f *PanickySetupSharedFixture) BeforeAll(ctx context.Context) error {
	if os.Getenv(EnvPanicSetupArm) != "" {
		panic("boom during shared setup")
	}
	return nil
}

func (f *PanickySetupSharedFixture) AfterAll(ctx context.Context) error { return nil }
