package gotestrunner

import "sync"

// The process trees running now, for a CLI that gives up on them.
var (
	liveMu sync.Mutex
	live   = map[*ManagedProcess]struct{}{}
)

func track(p *ManagedProcess) {
	liveMu.Lock()
	defer liveMu.Unlock()
	live[p] = struct{}{}
}

func untrack(p *ManagedProcess) {
	liveMu.Lock()
	defer liveMu.Unlock()
	delete(live, p)
}

// KillAll force-kills every process tree still running, for a CLI about to
// exit without waiting for them: the teardown they were running is given up
// on, and no process outlives the CLI.
func KillAll() {
	liveMu.Lock()
	defer liveMu.Unlock()
	for p := range live {
		p.forceKill()
	}
}
