package gotestrunner

// ExportSetupReported reports whether the process has said a setup started.
func ExportSetupReported(p *SharedFixtureProcess) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.setupUnbounded || !p.setupDeadline.IsZero()
}
