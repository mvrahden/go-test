package fixtureteardown

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// The fixture DAG lives in the suite's process and every suite runs in its
// own, so a marker is keyed by pid: two suites sharing the fixture must not
// share a file, or the first teardown would pull the other suite's marker.
func marker(name string) string {
	return filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), fmt.Sprintf("%s-%d", name, os.Getpid()))
}

// AlivePath is the file the fixture holds while it is set up.
func AlivePath() string { return marker("fixture-alive") }

// LedgerFixture writes a marker while it is up and another once it was torn
// down; the canary counts the second kind after the run.
type LedgerFixture struct{}

func (f *LedgerFixture) BeforeAll(ctx context.Context) error {
	return os.WriteFile(AlivePath(), []byte("alive"), 0o600)
}

func (f *LedgerFixture) AfterAll(ctx context.Context) error {
	if err := os.Remove(AlivePath()); err != nil {
		return err
	}
	return os.WriteFile(marker("fixture-afterall"), []byte("done"), 0o600)
}
