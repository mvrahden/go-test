//go:build !windows

package fixtures

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// DetachingSharedFixture starts a session-leader child on the inherited stdout
// in BeforeAll and leaves it running, the shape a daemonizing helper takes.
// That stdout is the runner's protocol pipe, so the child holds it open after
// the fixture process has exited. It exists to prove the runner's verdict on
// the fixture does not wait for that child, so no suite references it — the
// runner's own tests drive it directly.
//
// EnvDetachingStop names the file whose appearance lets the child exit.
type DetachingSharedFixture struct {
	Marker string
}

const EnvDetachingStop = "GOTEST_TEST_DETACHING_STOP"

func (f *DetachingSharedFixture) BeforeAll(ctx context.Context) error {
	stop := os.Getenv(EnvDetachingStop)
	child := exec.Command("sh", "-c", `for i in $(seq 200); do [ -e "$STOP" ] && exit 0; sleep 0.1; done`)
	child.Env = append(os.Environ(), "STOP="+stop)
	child.Stdout = os.Stdout
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return child.Start()
}

func (f *DetachingSharedFixture) AfterAll(ctx context.Context) error {
	return nil
}
