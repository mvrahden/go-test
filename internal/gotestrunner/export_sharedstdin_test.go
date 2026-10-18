package gotestrunner

// ExportCloseStdin closes the runner's end of the process's command pipe, as
// a runner that is gone does.
func ExportCloseStdin(p *SharedFixtureProcess) error { return p.stdin.Close() }
