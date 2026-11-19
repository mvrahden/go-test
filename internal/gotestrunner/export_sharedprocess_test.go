package gotestrunner

import (
	"context"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/protocol/sharedwire"
)

// fixtureStateEntry is the protocol line as the runner decodes it, for the
// ExportFixtureStateEntry door.
type fixtureStateEntry = sharedwire.Line

// ExportLaunchSharedProcess starts name with args as a shared fixture process
// that serves no fixtures, so a test can speak the protocol itself.
func ExportLaunchSharedProcess(ctx context.Context, dir, name string, args ...string) (*SharedFixtureProcess, error) {
	return launchSharedFixtures(exec.CommandContext(ctx, name, args...), dir, nil, 0, nil)
}

// ExportStartTapped is StartSharedFixtures with every line of the wire shown
// to tap: '>' for a line written to the process, '<' for one read from it.
func ExportStartTapped(ctx context.Context, tmpDir string, fixtures []gotestgen.SharedFixtureInfo, setupTimeout time.Duration, tap func(dir byte, line []byte), buildFlags ...string) (*SharedFixtureProcess, error) {
	return startSharedFixtures(ctx, tmpDir, fixtures, setupTimeout, tap, buildFlags...)
}

// ExportProtocolErr is the protocol error the process made after its setup.
func ExportProtocolErr(p *SharedFixtureProcess) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.protoErr
}

// ExportPid is the shared fixture process's pid.
func ExportPid(p *SharedFixtureProcess) int { return p.cmd.Process.Pid }
