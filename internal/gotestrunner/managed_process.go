package gotestrunner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
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
	// DrainDelay bounds how long the output pipes are still read once the
	// process has exited, against a detached grandchild that keeps the write
	// ends open. Zero means OutputDrainDelay.
	DrainDelay time.Duration
}

// OutputDrainDelay is the default DrainDelay. Everything the process wrote is
// in the pipe by the time it has exited, so the delay only covers reading it;
// it is what a detached grandchild can add to a run.
const OutputDrainDelay = 5 * time.Second

// ManagedProcess runs a command as the root of its own process tree and stops
// the tree by its grace strategy.
//
// It owns the command's output pipes: a writer set as Stdout or Stderr is fed
// through a pipe the process reads itself, not through exec. exec's copy would
// hold Wait until every holder of the pipe has closed it, and its only bound,
// WaitDelay, is a kill timer that would have to be set before the process
// wrote its teardown budget. Here the tree's grace strategy is the only kill,
// and the drain after exit is bounded on its own.
type ManagedProcess struct {
	cmd    *exec.Cmd
	tree   *proctree.Tree
	config ProcessConfig
	done   chan struct{}

	pipes   []*outputPipe
	pipeErr error
	copied  chan struct{}
}

// outputPipe carries one output stream from the process to the writer the
// caller set.
type outputPipe struct {
	r, w *os.File
	dst  io.Writer
}

func NewManagedProcess(cmd *exec.Cmd, cfg ProcessConfig) *ManagedProcess {
	tree := proctree.New(cmd)
	// exec's own kill timer never runs: the grace strategy is the only kill,
	// so a teardown budget is never cut short by a bound set before it was
	// known.
	cmd.WaitDelay = 0
	if cfg.Grace == GraceKill && cmd.Cancel != nil {
		cmd.Cancel = func() error {
			_ = tree.Kill()
			return nil
		}
	}
	p := &ManagedProcess{cmd: cmd, tree: tree, config: cfg, done: make(chan struct{}), copied: make(chan struct{})}
	p.pipeErr = p.takeOutput()
	return p
}

// takeOutput replaces every writer set as Stdout or Stderr with the write end
// of a pipe of its own. A file passes through untouched, as exec would pass
// it, and one writer set for both streams gets one pipe, so their order is
// kept. When a pipe cannot be made the writers stay as they were.
func (p *ManagedProcess) takeOutput() (err error) {
	stdout, stderr := p.cmd.Stdout, p.cmd.Stderr
	defer func() {
		if err != nil {
			p.closePipes()
			p.pipes = nil
			p.cmd.Stdout, p.cmd.Stderr = stdout, stderr
		}
	}()
	if isWriter(stdout) {
		pipe, err := newOutputPipe(stdout)
		if err != nil {
			return err
		}
		p.pipes = append(p.pipes, pipe)
		p.cmd.Stdout = pipe.w
		if sameWriter(stderr, stdout) {
			p.cmd.Stderr = pipe.w
			return nil
		}
	}
	if isWriter(stderr) {
		pipe, err := newOutputPipe(stderr)
		if err != nil {
			return err
		}
		p.pipes = append(p.pipes, pipe)
		p.cmd.Stderr = pipe.w
	}
	return nil
}

func isWriter(w io.Writer) bool {
	if w == nil {
		return false
	}
	_, isFile := w.(*os.File)
	return !isFile
}

// sameWriter reports whether a and b are the same writer; an uncomparable
// writer is never the same as another.
func sameWriter(a, b io.Writer) (same bool) {
	if a == nil || b == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

func newOutputPipe(dst io.Writer) (*outputPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &outputPipe{r: r, w: w, dst: dst}, nil
}

// closePipes releases both ends of every pipe, for a command that never ran.
func (p *ManagedProcess) closePipes() {
	for _, pipe := range p.pipes {
		_ = pipe.r.Close()
		_ = pipe.w.Close()
	}
}

// copyOutput starts reading the pipes once the process holds their write ends.
// The parent's copies close first, or EOF would never come.
func (p *ManagedProcess) copyOutput() {
	var wg sync.WaitGroup
	for _, pipe := range p.pipes {
		_ = pipe.w.Close()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(pipe.dst, pipe.r)
		}()
	}
	go func() {
		wg.Wait()
		close(p.copied)
	}()
}

// drain waits for the pipes to reach EOF, for at most the drain delay past
// the process's exit, then closes the read ends, which ends the copies.
func (p *ManagedProcess) drain() {
	if len(p.pipes) == 0 {
		return
	}
	readEnds := make([]*os.File, 0, len(p.pipes))
	for _, pipe := range p.pipes {
		readEnds = append(readEnds, pipe.r)
	}
	drainOutput(p.copied, p.drainDelay(), readEnds...)
}

// drainOutput waits for the readers of readEnds to finish, signalled by
// finished, for at most delay, then closes the read ends, which ends a read a
// detached grandchild's write end would otherwise hold open, and waits for
// them. Called once the process has exited, so everything it wrote is already
// in the pipes.
func drainOutput(finished <-chan struct{}, delay time.Duration, readEnds ...*os.File) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-finished:
	case <-timer.C:
		for _, r := range readEnds {
			_ = r.Close()
		}
		<-finished
	}
	for _, r := range readEnds {
		_ = r.Close()
	}
}

func (p *ManagedProcess) drainDelay() time.Duration {
	if p.config.DrainDelay > 0 {
		return p.config.DrainDelay
	}
	return OutputDrainDelay
}

func (p *ManagedProcess) Start() error {
	if p.pipeErr != nil {
		return p.pipeErr
	}
	if err := p.tree.Start(); err != nil {
		p.closePipes()
		return err
	}
	p.copyOutput()
	go p.wait()
	return nil
}

// Adopt takes over a command the caller started after NewManagedProcess.
func (p *ManagedProcess) Adopt() {
	p.tree.Adopt()
	p.copyOutput()
	go p.wait()
}

// wait ends ownership as soon as the root has been reaped, since its pid may
// be reused from then on, and only then drains what is left in the pipes.
func (p *ManagedProcess) wait() {
	_ = p.cmd.Wait()
	p.tree.Release()
	p.drain()
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
