package gotestrunner

import (
	"context"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

type GraceStrategy int

const (
	GraceFixed GraceStrategy = iota
	GraceBudget
	GraceKill
)

type ProcessConfig struct {
	Grace         GraceStrategy
	GraceDuration time.Duration
	BudgetFile    string
}

// ManagedProcess runs a command as the root of its own process tree and stops
// the tree by its grace strategy.
type ManagedProcess struct {
	cmd    *exec.Cmd
	tree   *proctree.Tree
	config ProcessConfig
	done   chan struct{}
}

func NewManagedProcess(cmd *exec.Cmd, cfg ProcessConfig) *ManagedProcess {
	tree := proctree.New(cmd)
	cmd.WaitDelay = 0
	if cfg.Grace == GraceKill && cmd.Cancel != nil {
		cmd.Cancel = func() error {
			_ = tree.Kill()
			return nil
		}
	}
	return &ManagedProcess{cmd: cmd, tree: tree, config: cfg, done: make(chan struct{})}
}

func (p *ManagedProcess) Start() error {
	if err := p.tree.Start(); err != nil {
		return err
	}
	go p.wait()
	return nil
}

// Adopt takes over a command the caller started after NewManagedProcess.
func (p *ManagedProcess) Adopt() {
	p.tree.Adopt()
	go p.wait()
}

func (p *ManagedProcess) wait() {
	_ = p.cmd.Wait()
	p.tree.Release()
	close(p.done)
}

func (p *ManagedProcess) Done() <-chan struct{} { return p.done }
func (p *ManagedProcess) Cmd() *exec.Cmd        { return p.cmd }
func (p *ManagedProcess) SetGraceDuration(d time.Duration) {
	if d <= 0 {
		return
	}
	p.config.GraceDuration = d
}

func (p *ManagedProcess) WaitWithGrace(ctx context.Context) error {
	select {
	case <-p.done:
		if p.cmd.ProcessState != nil && !p.cmd.ProcessState.Success() {
			return &exec.ExitError{ProcessState: p.cmd.ProcessState}
		}
		return nil
	case <-ctx.Done():
	}
	grace := p.graceTimeout()
	select {
	case <-p.done:
	case <-time.After(grace):
		p.forceKill()
		<-p.done
	}
	if p.cmd.ProcessState != nil && !p.cmd.ProcessState.Success() {
		return &exec.ExitError{ProcessState: p.cmd.ProcessState}
	}
	return nil
}

func (p *ManagedProcess) Terminate() {
	if p.cmd.Process == nil {
		return
	}
	select {
	case <-p.done:
		return
	default:
	}
	_ = p.tree.Interrupt()
	grace := p.graceTimeout()
	select {
	case <-p.done:
	case <-time.After(grace):
		p.forceKill()
		<-p.done
	}
}

func (p *ManagedProcess) forceKill() {
	_ = p.tree.Kill()
}

func (p *ManagedProcess) graceTimeout() time.Duration {
	switch p.config.Grace {
	case GraceBudget:
		return readTeardownBudget(p.config.BudgetFile)
	case GraceFixed:
		return p.config.GraceDuration
	case GraceKill:
		return 0
	}
	return GracefulShutdownDelay
}
