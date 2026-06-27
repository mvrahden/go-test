package gotestrunner

import (
	"context"
	"os"
	"os/exec"
	"time"
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

type ManagedProcess struct {
	cmd    *exec.Cmd
	config ProcessConfig
	done   chan struct{}
	job    *jobObject
}

func NewManagedProcess(cmd *exec.Cmd, cfg ProcessConfig) *ManagedProcess {
	setProcessGroupAttr(cmd)
	cmd.WaitDelay = 0
	job, _ := newJobObject()
	mp := &ManagedProcess{cmd: cmd, config: cfg, done: make(chan struct{}), job: job}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if cfg.Grace == GraceKill {
			mp.forceKill()
			return nil
		}
		if err := TerminateProcessGroup(cmd.Process.Pid); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	return mp
}

func (p *ManagedProcess) Start() error {
	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.assignJob()
	go func() { _ = p.cmd.Wait(); p.closeJob(); close(p.done) }()
	return nil
}

func (p *ManagedProcess) Adopt() {
	p.assignJob()
	go func() { _ = p.cmd.Wait(); p.closeJob(); close(p.done) }()
}

func (p *ManagedProcess) assignJob() {
	if p.job != nil && p.cmd.Process != nil {
		_ = p.job.assign(p.cmd.Process.Pid)
	}
}

func (p *ManagedProcess) closeJob() {
	if p.job != nil {
		p.job.close()
	}
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
	_ = TerminateProcessGroup(p.cmd.Process.Pid)
	grace := p.graceTimeout()
	select {
	case <-p.done:
	case <-time.After(grace):
		p.forceKill()
		<-p.done
	}
}

func (p *ManagedProcess) forceKill() {
	if p.job != nil {
		_ = p.job.terminate(1)
		return
	}
	if p.cmd.Process != nil {
		_ = ForceKillProcessGroup(p.cmd.Process.Pid)
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
