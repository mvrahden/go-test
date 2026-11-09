package gotestrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mvrahden/go-test/internal/control"
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
	// Link is the runner's end of a suite process's channel, which the
	// process owns from Start: the stop, the force and the teardown budget
	// travel on it. It is closed once the tree is released.
	Link *control.Link
	// CtrlC interrupts the tree as a terminal's Ctrl-C does, for a program
	// that stops gracefully on it (go test -fuzz); see proctree.
	CtrlC bool
	// HoldJob keeps the tree whole while its root lives, past the runner's
	// own end, for a process that tears its fixtures down; see proctree.
	HoldJob bool
	// DrainDelay bounds how long the output pipes are still read once the
	// process has exited, against a detached grandchild that keeps the write
	// ends open. Zero means OutputDrainDelay.
	DrainDelay time.Duration
	// AwaitOutput keeps the tree whole past its root's exit while the output
	// is still open, for the grace at most, unless the caller stopped it: a
	// root that runs the process the output is from, an -exec command, can
	// end before that process does.
	AwaitOutput bool
}

// OutputDrainDelay is the default DrainDelay. Everything the process wrote is
// in the pipe by the time it has exited, so the delay only covers reading it;
// it is what a detached grandchild can add to a run.
const OutputDrainDelay = 5 * time.Second

// sharedFixtureDrainDelay is the shared fixture process's DrainDelay, a
// variable so its tests need not wait the default out.
var sharedFixtureDrainDelay time.Duration = OutputDrainDelay

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

	// stopped: the caller stopped the process by a route of its own, which
	// reached the root's tree no further.
	stopped atomic.Bool

	// What the suite process said on its channel.
	started   time.Time
	hello     chan struct{} // closed once its hello came
	linesDone chan struct{} // closed once the channel's lines ended
	budget    atomic.Int64  // the largest teardown budget it reported
	stoppedBy atomic.Value  // string: the reason of its first stopped line
	// fellBack: the first stop was a Ctrl-C to the tree, the channel being
	// untaken or unwritable.
	fellBack atomic.Bool
}

// helloWithin is how long after its start a suite process has to take its
// channel; a stop before then waits for the rest of it, and one with no
// hello by then interrupts the tree as a Ctrl-C would.
var helloWithin = 3 * time.Second

var errNoChannel = errors.New("the suite process took no channel")

// outputPipe carries one output stream from the process to the writer the
// caller set.
type outputPipe struct {
	r, w *os.File
	dst  io.Writer
}

func NewManagedProcess(cmd *exec.Cmd, cfg ProcessConfig) *ManagedProcess {
	tree := proctree.New(cmd)
	if cfg.CtrlC {
		tree.InterruptLikeCtrlC()
	}
	if cfg.HoldJob {
		tree.HoldJobWithRoot()
	}
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
	p := &ManagedProcess{cmd: cmd, tree: tree, config: cfg, done: make(chan struct{}), copied: make(chan struct{}),
		hello: make(chan struct{}), linesDone: make(chan struct{})}
	p.pipeErr = p.takeOutput()
	return p
}

// takeOutput replaces every writer set as Stdout or Stderr with the write end
// of a pipe of its own. A file passes through untouched, as exec would pass
// it, and one writer set for both streams gets one pipe, so their order is
// kept: a suite's relay is one. When a pipe cannot be made the writers stay
// as they were.
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
		p.closeLink(false)
		return p.pipeErr
	}
	p.started = time.Now()
	if err := p.tree.Start(); err != nil {
		p.closePipes()
		p.closeLink(false)
		return err
	}
	p.track()
	p.copyOutput()
	go p.readLink()
	go p.wait()
	return nil
}

func (p *ManagedProcess) track() {
	l := liveTree{root: p.cmd.Process.Pid, done: p.done}
	if p.config.Link != nil {
		l.again = p.force
	}
	track(p.tree, l)
}

// Adopt takes over a command the caller started after NewManagedProcess.
func (p *ManagedProcess) Adopt() {
	p.started = time.Now()
	p.tree.Adopt()
	p.track()
	p.copyOutput()
	go p.readLink()
	go p.wait()
}

// readLink records what the suite process says on its channel until the
// channel ends.
func (p *ManagedProcess) readLink() {
	defer close(p.linesDone)
	if p.config.Link == nil {
		return
	}
	for l := range p.config.Link.Lines() {
		switch l.Kind {
		case control.Hello:
			select {
			case <-p.hello:
			default:
				close(p.hello)
			}
		case control.Budget:
			for {
				was := p.budget.Load()
				if int64(l.Duration) <= was || p.budget.CompareAndSwap(was, int64(l.Duration)) {
					break
				}
			}
		case control.Stopped:
			p.stoppedBy.CompareAndSwap(nil, l.Reason)
		}
	}
}

// closeLink closes the channel, and once the process ran, waits for its
// lines to end.
func (p *ManagedProcess) closeLink(ran bool) {
	if p.config.Link == nil {
		return
	}
	p.config.Link.Close()
	if ran {
		<-p.linesDone
	}
}

// RequestStop asks the suite process to stop: by the stop line once it has
// taken its channel, waiting for its hello up to helloWithin past its start.
// Without a hello, or when the line cannot be written, the tree gets one
// Ctrl-C instead; the process then stops as under plain go test.
func (p *ManagedProcess) RequestStop() error {
	if p.config.Link != nil && p.awaitHello() && p.config.Link.Stop() == nil {
		// The rest of the tree is ended once the root has exited.
		p.Stopped()
		return nil
	}
	p.fellBack.Store(true)
	if err := p.tree.CtrlC(); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: a stopped suite process could not be interrupted: %v\n", err)
		return err
	}
	return nil
}

// awaitHello reports whether the suite process took its channel, waiting at
// most until helloWithin past its start, or until the channel ends.
func (p *ManagedProcess) awaitHello() bool {
	timer := time.NewTimer(time.Until(p.started.Add(helloWithin)))
	defer timer.Stop()
	select {
	case <-p.hello:
		return true
	case <-p.linesDone:
	case <-timer.C:
	}
	return p.saidHello()
}

func (p *ManagedProcess) saidHello() bool {
	select {
	case <-p.hello:
		return true
	default:
		return false
	}
}

// force is the second stop: the force line, to a process that took its
// channel.
func (p *ManagedProcess) force() error {
	if !p.saidHello() {
		return errNoChannel
	}
	return p.config.Link.Force()
}

// StoppedBy is the reason the suite process gave for a stop the runner did
// not send, such as "signal:SIGTERM"; empty when there was none, or when the
// runner's own Ctrl-C began the stop.
func (p *ManagedProcess) StoppedBy() string {
	reason, _ := p.stoppedBy.Load().(string)
	if p.fellBack.Load() {
		return ""
	}
	return reason
}

// wait ends ownership once the root has been reaped, since its pid may be
// reused from then on, and only then drains what is left in the pipes.
func (p *ManagedProcess) wait() {
	_ = p.cmd.Wait()
	if p.stopped.Load() {
		// The root is done, and so is its output: what it leaves running is
		// interrupted, given the grace and killed.
		p.tree.End(p.graceTimeout())
	} else {
		grace := p.graceTimeout()
		deadline := time.Now().Add(grace)
		if p.config.AwaitOutput {
			// The process the root ran may outlive it.
			p.awaitOutput(grace)
		}
		// An interrupted root can exit before the rest of its tree is done
		// shutting down; the grace it was given covers them too.
		p.tree.Linger(time.Until(deadline))
	}
	untrack(p.tree)
	p.tree.Release()
	// Never before the release: a process still holding the channel reads
	// its end as the runner gone, and stops.
	p.closeLink(true)
	p.drain()
	close(p.done)
}

// awaitOutput waits until every holder of the output pipes has closed them,
// at most timeout.
func (p *ManagedProcess) awaitOutput(timeout time.Duration) {
	if len(p.pipes) == 0 {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.copied:
	case <-timer.C:
	}
}

// Stopped records that the caller stopped the process without interrupting
// its tree, so the rest of the tree is stopped once the root has exited.
func (p *ManagedProcess) Stopped() { p.stopped.Store(true) }

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
		if b := time.Duration(p.budget.Load()); b > 0 {
			return b
		}
		return GracefulShutdownDelay
	case GraceFixed:
		return p.config.GraceDuration
	case GraceKill:
		return 0
	}
	return GracefulShutdownDelay
}
