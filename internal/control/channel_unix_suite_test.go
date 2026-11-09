//go:build unix

package control_test

import (
	"os/exec"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ChannelProcessTestSuite covers the channel's ends across processes the
// runner starts with a shell: what a wrapper that lets go of the channel
// leaves the runner, and what a sibling process inherits.
type ChannelProcessTestSuite struct{}

func (s *ChannelProcessTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// startShell starts sh running script with the channel attached and the
// runner's copy of the child end closed, as the runner does.
func startShell(t *gotest.T, script string) (*control.Link, *exec.Cmd) {
	link, end, err := control.New()
	gotest.NoError(t, err)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	end.Attach(cmd)
	gotest.NoError(t, cmd.Start())
	end.Close()
	return link, cmd
}

// endTree kills what sh left in its process group and reaps sh.
func endTree(link *control.Link, cmd *exec.Cmd) {
	link.Close()
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Wait()
}

func (s *ChannelProcessTestSuite) TestAWrapperThatExecsWithTheChannelClosed(t *gotest.T) {
	// The close is an exec of its own: macOS's /bin/sh, bash 3.2, keeps a
	// copy of a descriptor closed on an exec that also runs a command.
	link, cmd := startShell(t, `printf 'hello 1 %d\n' $$ >&3; exec 3>&-; exec sleep 30`)
	defer endTree(link, cmd)
	var hello control.Line
	select {
	case hello = <-link.Lines():
	case <-time.After(testkit.Scaled(5 * time.Second)):
	}
	closed := false
	select {
	case _, open := <-link.Lines():
		closed = !open
	case <-time.After(testkit.Scaled(2 * time.Second)):
	}
	err := link.Stop()

	t.When("a shell says hello on the channel, then execs the suite with the channel closed", func(w *gotest.T) {
		w.It("the hello arrives, then the channel's end, nothing holding it any more", func(it *gotest.T) {
			gotest.Equal(it, control.Hello, hello.Kind)
			gotest.True(it, closed, "the lines did not end after the exec")
		})
		w.It("a stop line cannot be written", func(it *gotest.T) {
			gotest.Error(it, err)
		})
	})
}

func (s *ChannelProcessTestSuite) TestASiblingInheritsNothing(t *gotest.T) {
	link, end, err := control.New()
	gotest.NoError(t, err)
	// A sibling the runner starts after the channel exists, as the converter
	// is started: it must not keep the child end open.
	sibling := exec.Command("/bin/sh", "-c", "sleep 30")
	sibling.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	gotest.NoError(t, sibling.Start())
	defer func() {
		_ = syscall.Kill(-sibling.Process.Pid, syscall.SIGKILL)
		_ = sibling.Wait()
	}()
	end.Close()
	closed := false
	select {
	case _, open := <-link.Lines():
		closed = !open
	case <-time.After(testkit.Scaled(2 * time.Second)):
	}
	stopErr := link.Stop()
	link.Close()

	t.When("the runner's copy of the child end is closed while a sibling it started lives on", func(w *gotest.T) {
		w.It("the channel ends: the sibling inherited no end of it", func(it *gotest.T) {
			gotest.True(it, closed, "the lines did not end")
			gotest.Error(it, stopErr)
		})
	})
}
