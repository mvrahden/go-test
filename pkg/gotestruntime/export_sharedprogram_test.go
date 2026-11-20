package gotestruntime

import (
	"context"
	"io"
	"time"
)

// ExportRunSharedProgram runs p in this process: commands come from stdin,
// protocol lines go to proto, and signals ending stands in for SIGTERM. The
// runner never counts as gone, and the process never exits.
func ExportRunSharedProgram(p SharedProgram, stdin io.Reader, proto io.Writer, signals context.Context) (failed bool) { //nolint:gocritic // hugeParam: as RunSharedFixtures
	returned := make(chan struct{})
	defer close(returned)
	names := make([]string, len(p.Fixtures))
	for i := range p.Fixtures {
		names[i] = p.Fixtures[i].Name
	}
	life := newSharedLifetime(io.Discard, func(int) {}, func(time.Duration) { <-returned }, sharedTeardownHeadroom, names)
	return runSharedProgram(p, sharedEnv{
		stdin:    stdin,
		proto:    proto,
		signals:  signals,
		orphaned: func() bool { return false },
		life:     life,
	})
}
