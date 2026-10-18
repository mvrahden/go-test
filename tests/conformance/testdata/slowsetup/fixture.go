// Package slowsetup has a shared fixture that comes up slowly, within the
// budget it declares.
package slowsetup

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ContainerSharedFixture declares ContainerFixtureConfig and takes 130s to
// come up, as a container pull might.
type ContainerSharedFixture struct {
	State string
}

func (f *ContainerSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

func (f *ContainerSharedFixture) BeforeAll(ctx context.Context) error {
	select {
	case <-time.After(130 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	f.State = "up"
	return nil
}

func (f *ContainerSharedFixture) AfterAll(ctx context.Context) error { return nil }
