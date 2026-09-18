// Package proctree starts a command as the root of its own process tree and
// stops that tree and nothing else: a shutdown request or a force-kill never
// reaches the caller or a sibling tree.
//
// On Unix the tree is a process group. On Windows it is a hidden console of its
// own, which scopes the console control event a shutdown request is, plus a job
// object, which scopes the force-kill.
package proctree

import (
	"os"
	"os/exec"
	"sync"
)

// Tree owns one started command and every process it starts.
type Tree struct {
	cmd *exec.Cmd

	mu       sync.Mutex
	adopted  bool
	released bool
	sys      sysTree
}

// New prepares cmd to start as the root of its own tree. For a command made by
// exec.CommandContext, a canceled context asks the tree to shut down instead of
// killing the root. Call it before cmd starts.
func New(cmd *exec.Cmd) *Tree {
	t := &Tree{cmd: cmd}
	prepare(cmd)
	if cmd.Cancel == nil {
		return t
	}
	cmd.Cancel = func() error {
		if err := t.Interrupt(); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	return t
}

// Start starts the command and takes ownership of its tree.
func (t *Tree) Start() error {
	if err := t.cmd.Start(); err != nil {
		return err
	}
	t.Adopt()
	return nil
}

// Adopt takes ownership of a command the caller started itself. It must run
// before anything waits for the command.
func (t *Tree) Adopt() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.adoptLocked()
}

// adoptLocked runs at most once. A canceled context can call Cancel between
// cmd.Start and Adopt; the command is not waited for yet, so its pid is still
// the command's.
func (t *Tree) adoptLocked() {
	if t.adopted || t.released || t.cmd.Process == nil {
		return
	}
	t.adopted = true
	t.sys.adopt(t.cmd.Process.Pid)
}

// Interrupt asks every process in the tree to shut down: SIGTERM to the process
// group on Unix, CTRL_BREAK on the tree's console on Windows. It returns
// os.ErrProcessDone once the tree is released or its root has exited, and nil
// for a command that never started.
func (t *Tree) Interrupt() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.released {
		return os.ErrProcessDone
	}
	t.adoptLocked()
	if !t.adopted {
		return nil
	}
	return t.sys.interrupt(t.cmd.Process.Pid)
}

// Kill stops every process in the tree at once. It returns os.ErrProcessDone
// once the tree is released, and nil for a command that never started.
func (t *Tree) Kill() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.released {
		return os.ErrProcessDone
	}
	t.adoptLocked()
	if !t.adopted {
		return nil
	}
	return t.sys.kill(t.cmd.Process.Pid)
}

// Release ends ownership; call it once the command has been waited for. Its pid
// may belong to another process from then on, so nothing is signalled after.
// On Windows it also kills what the root left running in its tree.
func (t *Tree) Release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.released {
		return
	}
	t.released = true
	if t.adopted {
		t.sys.release()
	}
}
