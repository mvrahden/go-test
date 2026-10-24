package drill

import (
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// maxHolders bounds how many holders a refusal names.
const maxHolders = 8

// lockHolders finds, best effort and within a second, the processes holding
// path open: a drill killed outright leaves it with whatever it started.
// more reports holders beyond the first maxHolders.
func lockHolders(path string) (pids []int, more bool) {
	var want unix.Stat_t
	if unix.Stat(path, &want) != nil {
		return nil, false
	}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	deadline := time.Now().Add(time.Second)
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		if time.Now().After(deadline) {
			return pids, true
		}
		dir := "/proc/" + p.Name() + "/fd/"
		fds, err := os.ReadDir(dir) // another user's: not readable
		if err != nil {
			continue
		}
		for _, fd := range fds {
			var st unix.Stat_t
			if unix.Stat(dir+fd.Name(), &st) == nil && st.Dev == want.Dev && st.Ino == want.Ino {
				if len(pids) == maxHolders {
					return pids, true
				}
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, false
}
