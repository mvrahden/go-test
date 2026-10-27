package gotestrunner

import "time"

// ExportNoteSetupBudget records a setup starting now with budget, as the
// process reports it.
func ExportNoteSetupBudget(p *SharedFixtureProcess, budget time.Duration) { p.noteSetupBudget(budget) }

// reportingStdin reports a setup with budget delay after it receives the
// start command, and returns lag after the report.
type reportingStdin struct {
	p                  *SharedFixtureProcess
	budget, delay, lag time.Duration
}

func (w reportingStdin) Write(b []byte) (int, error) {
	time.Sleep(w.delay)
	w.p.noteSetupBudget(w.budget)
	time.Sleep(w.lag)
	return len(b), nil
}

func (reportingStdin) Close() error { return nil }

// ExportNewReportingProcess is a process that reports a setup declaring budget
// delay after a command reaches it, lag before the command's write returns,
// and never acknowledges.
func ExportNewReportingProcess(budget, delay, lag time.Duration) *SharedFixtureProcess {
	p := ExportNewSilentProcess()
	p.done = make(chan struct{})
	p.stdin = reportingStdin{p: p, budget: budget, delay: delay, lag: lag}
	return p
}
