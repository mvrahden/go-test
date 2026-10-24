// Package fixbdetached binds a shared fixture whose setup starts a process
// that outlives it: the process records the protocol variable it sees, writes
// a failing _done line to descriptor 3 and stays.
package fixbdetached

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// DetachSharedFixture starts the process and comes up once it has written.
type DetachSharedFixture struct {
	State string
}

func (f *DetachSharedFixture) BeforeAll(ctx context.Context) error {
	dir := os.Getenv("GOTEST_CANARY_DIR")
	ready := filepath.Join(dir, "detached-ready")
	script := `printf 'env=%s\n' "$GOTEST_SHARED_PROTOCOL_FD" >> "$1/markers"
printf '{"key":"_done","error":"written by a process the fixture started"}\n' >&3 2>/dev/null
: > "$1/detached-ready"
exec sleep 30`
	cmd := exec.Command("sh", "-c", script, "sh", dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "detached-pid"), []byte(fmt.Sprint(cmd.Process.Pid)), 0o600); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			f.State = "up"
			return nil
		}
	}
	return errors.New("the started process never wrote")
}

func (f *DetachSharedFixture) AfterAll(ctx context.Context) error { return nil }
