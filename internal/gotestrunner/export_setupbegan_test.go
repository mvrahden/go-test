package gotestrunner

import "time"

// ExportNoteSetupBudget records a setup starting now with budget, as the
// process reports it.
func ExportNoteSetupBudget(p *SharedFixtureProcess, budget time.Duration) { p.noteSetupBudget(budget) }

// discardStdin takes every command and answers none.
type discardStdin struct{}

func (discardStdin) Write(b []byte) (int, error) { return len(b), nil }
func (discardStdin) Close() error                { return nil }

// ExportNewHungStartProcess is a process that takes start commands and never
// acknowledges them; it exits only when exit is called.
func ExportNewHungStartProcess() (p *SharedFixtureProcess, exit func()) {
	p = ExportNewSilentProcess()
	p.done = make(chan struct{})
	p.cmdResp = make(chan string, 1)
	p.stdin = discardStdin{}
	return p, func() { close(p.done) }
}
