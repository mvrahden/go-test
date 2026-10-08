//go:build unix

package quick

import (
	"context"
	"os/exec"
	"syscall"
	"time"
)

// runPipeline runs a pipeline whose writer ends only on SIGPIPE — a failed
// echo does not stop the loop — so it never ends if this process handed its
// children an ignored SIGPIPE. Bounded, and killed as a group, so that case
// fails instead of hanging.
func runPipeline(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", "while :; do echo y; done | head -1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if len(out) > 0 {
		return string(out[:1]), err
	}
	return "", err
}
