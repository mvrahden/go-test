//go:build unix

package gotestruntime_test

import (
	"os"
	"testing"
)

// TestMain: a scenario's suite process may wait before m.Run, and act after
// it.
func TestMain(m *testing.M) {
	awaitLinkGate()
	code := m.Run()
	afterLinkRun()
	os.Exit(code)
}
