package drill_test

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// envChild makes a run of this test binary one of the child programs in
// children_linux_test.go, named by its first argument.
const envChild = "DRILL_TEST_CHILD"

// A child that watches signals does so on the main thread.
func init() {
	if os.Getenv(envChild) == "1" {
		runtime.LockOSThread()
	}
}

func TestMain(m *testing.M) {
	if os.Getenv(envChild) == "1" {
		os.Exit(runChild(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// childCommand runs this test binary as the child program args name.
func childCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(self(), args...) //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), envChild+"=1")
	return cmd
}

func self() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	return exe
}
