package gotestrunner

import (
	"context"
	"os/exec"
)

// ExportLaunchSharedProcess starts name with args as a shared fixture process
// that serves no fixtures, so a test can speak the protocol itself.
func ExportLaunchSharedProcess(ctx context.Context, dir, name string, args ...string) (*SharedFixtureProcess, error) {
	return launchSharedFixtures(exec.CommandContext(ctx, name, args...), dir, nil, 0)
}

// ExportProtocolErr is the protocol error the process made after its setup.
func ExportProtocolErr(p *SharedFixtureProcess) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.protoErr
}
