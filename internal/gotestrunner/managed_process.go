package gotestrunner

import (
	"context"
	"os"
	"os/exec"
	"time"
)

type GraceStrategy int

const (
	GraceFixed  GraceStrategy = iota
	GraceBudget
	GraceKill
)

type ProcessConfig struct {
	Grace         GraceStrategy
	GraceDuration time.Duration
	BudgetFile    string
}

type ManagedProcess struct {
	cmd    *exec.Cmd
	config ProcessConfig
	done   chan struct{}
}

func NewManagedProcess(cmd *exec.Cmd, cfg ProcessConfig) *ManagedProcess {
	setProcessGroupAttr(cmd)
	cmd.WaitDelay = 0
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if cfg.Grace == GraceKill {
			if err := ForceKillProcessGroup(cmd.Process.Pid); err != nil {
				return os.ErrProcessDone
			}
			return nil
		}
		if err := TerminateProcessGroup(cmd.Process.Pid); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	return &ManagedProcess{cmd: cmd, config: cfg, done: make(chan struct{})}
}

func (p *ManagedProcess) Start() error {
	if err := p.cmd.Start(); err != nil {
		return err
	}
	go func() { p.cmd.Wait(); close(p.done) }()
	return nil
}

func (p *ManagedProcess) Adopt() {
	go func() { p.cmd.Wait(); close(p.done) }()
}

func (p *ManagedProcess) Done() <-chan struct{}       { return p.done }
func (p *ManagedProcess) Cmd() *exec.Cmd              { return p.cmd }
func (p *ManagedProcess) SetGraceDuration(d time.Duration) { p.config.GraceDuration = d }

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
		ForceKillProcessGroup(p.cmd.Process.Pid)
		<-p.done
	}
	if p.cmd.ProcessState != nil && !p.cmd.ProcessState.Success() {
		return &exec.ExitError{ProcessState: p.cmd.ProcessState}
	}
	return nil
}

func (p *ManagedProcess) Terminate() {
	TerminateProcessGroup(p.cmd.Process.Pid)
	grace := p.graceTimeout()
	select {
	case <-p.done:
	case <-time.After(grace):
		ForceKillProcessGroup(p.cmd.Process.Pid)
		<-p.done
	}
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
