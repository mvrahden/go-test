//go:build linux

package proctree_test

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LingerTestSuite: after an interrupted root exits, the tree waits for the rest
// of its group — and only for what is still running.
type LingerTestSuite struct{}

func (s *LingerTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// interruptedRoot starts a root in its own group, a member of that group
// started by fn, interrupts the tree and reaps the root.
func interruptedRoot(t *gotest.T, member func(pgid int) *exec.Cmd) (*proctree.Tree, *exec.Cmd) {
	root := exec.Command("sleep", "30")
	tree := proctree.New(root)
	gotest.NoError(t, tree.Start())
	m := member(root.Process.Pid)
	gotest.NoError(t, m.Start())
	return tree, m
}

func inGroup(pgid int, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
	return cmd
}

func isZombie(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(stat)
	return strings.Contains(s[strings.LastIndexByte(s, ')'):], ") Z ")
}

// A container whose PID 1 never reaps leaves an exited member a zombie; it
// still answers a probe of the group, but nothing is left to wait for.
func (s *LingerTestSuite) TestAGroupOfZombiesIsNotWaitedFor(t *gotest.T) {
	tree, zombie := interruptedRoot(t, func(pgid int) *exec.Cmd { return inGroup(pgid, "true") })
	gotest.Eventually(t, 5*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.True(poll, isZombie(zombie.Process.Pid), "the member never became a zombie")
	})
	gotest.NoError(t, tree.Interrupt())

	start := time.Now()
	tree.Linger(10 * time.Second)
	waited := time.Since(start)

	_ = zombie.Wait()
	tree.Release()
	gotest.Less(t, waited, 2*time.Second, "waited %s for a group of zombies", waited)
}

func (s *LingerTestSuite) TestALiveMemberIsWaitedFor(t *gotest.T) {
	tree, member := interruptedRoot(t, func(pgid int) *exec.Cmd {
		// Ignores the group's SIGTERM, as a child finishing its own
		// shutdown would, and ends a moment later: the shell sets the
		// ignore and becomes sleep, which keeps it.
		return inGroup(pgid, "sh", "-c", "trap '' TERM; exec sleep 1")
	})
	gotest.Eventually(t, 5*time.Second, 5*time.Millisecond, func(poll *gotest.R) {
		comm, _ := os.ReadFile("/proc/" + strconv.Itoa(member.Process.Pid) + "/comm")
		gotest.Equal(poll, "sleep", strings.TrimSpace(string(comm)), "the member is not sleeping yet")
	})
	gotest.NoError(t, tree.Interrupt())

	start := time.Now()
	tree.Linger(10 * time.Second)
	waited := time.Since(start)

	_ = member.Wait()
	tree.Release()
	gotest.GreaterOrEqual(t, waited, 500*time.Millisecond, "returned while a member still ran")
	gotest.Less(t, waited, 9*time.Second)
}
