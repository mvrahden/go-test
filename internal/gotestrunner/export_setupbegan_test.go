package gotestrunner

import "time"

// ExportNoteSetupBudget records a setup starting now with budget, as the
// process reports it.
func ExportNoteSetupBudget(p *SharedFixtureProcess, budget time.Duration) { p.noteSetupBudget(budget) }

// reportingStdin reports a setup with budget as it receives the start
// command, and returns only after lag: the report beats the wait.
type reportingStdin struct {
	p           *SharedFixtureProcess
	budget, lag time.Duration
}

func (w reportingStdin) Write(b []byte) (int, error) {
	w.p.noteSetupBudget(w.budget)
	time.Sleep(w.lag)
	return len(b), nil
}

func (reportingStdin) Close() error { return nil }

// ExportNewReportingProcess is a process that reports a setup declaring budget
// as soon as a command reaches it, lag before the command's write returns,
// and never acknowledges.
func ExportNewReportingProcess(budget, lag time.Duration) *SharedFixtureProcess {
	p := ExportNewSilentProcess()
	p.done = make(chan struct{})
	p.cmdResp = make(chan string, 1)
	p.stdin = reportingStdin{p: p, budget: budget, lag: lag}
	return p
}
