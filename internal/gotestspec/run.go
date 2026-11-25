package gotestspec

import "github.com/mvrahden/go-test/internal/protocol"

// RunInfo is the run a document renders: what its stream's first event says
// it was and, once the run closed it, its exit code.
type RunInfo struct {
	Manifest protocol.Manifest
	// ExitCode is nil for a stream the run did not close.
	ExitCode *int
}

// WithRun names the run a document renders.
func WithRun(r RunInfo) RenderOption {
	return func(c *renderConfig) { c.run = &r }
}

// isRunEvent reports an event of the run's own package.
func isRunEvent(ev *TestEvent) bool {
	return ev.RunLevel && ev.Package == protocol.RunPackage
}

// RunOf is the run events record: its manifest and, when the run closed the
// stream, its exit code. ok is false for a stream with no manifest.
func RunOf(events []TestEvent) (r RunInfo, ok bool) {
	for i := range events {
		ev := &events[i]
		if !isRunEvent(ev) {
			continue
		}
		if ev.Manifest != nil && !ok {
			r.Manifest, ok = *ev.Manifest, true
		}
		if ev.Result != nil {
			code := ev.Result.ExitCode
			r.ExitCode = &code
		}
	}
	if !ok {
		return RunInfo{}, false
	}
	return r, true
}

func (r *RunInfo) json() *jsonRun {
	if r == nil {
		return nil
	}
	m := r.Manifest
	return &jsonRun{
		Version:   m.Version,
		GoVersion: m.GoVersion,
		Args:      m.Args,
		Commit:    m.Commit,
		Dirty:     m.Dirty,
		Agent:     m.Agent,
		ExitCode:  r.ExitCode,
	}
}
