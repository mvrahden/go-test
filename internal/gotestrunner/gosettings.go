package gotestrunner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

// GoSettings are the go command's settings a run reads before it builds.
type GoSettings struct {
	GOOS    string // the operating system go builds for
	GOARCH  string // the architecture go builds for
	GOFLAGS string // as go reads it: the environment, else go's env file
}

// ReadGoSettings is `go env GOOS GOARCH GOFLAGS`, asked once per process.
func ReadGoSettings() GoSettings { return goSettings() }

var goSettings = sync.OnceValue(readGoSettings) // tests replace it

// readGoSettings asks go; without an answer it gives the host's platform and
// the GOFLAGS of the environment.
func readGoSettings() GoSettings {
	out, err := exec.Command("go", "env", "-json", "GOOS", "GOARCH", "GOFLAGS").Output()
	var env map[string]string
	if err != nil || json.Unmarshal(out, &env) != nil || env["GOOS"] == "" || env["GOARCH"] == "" {
		return GoSettings{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GOFLAGS: os.Getenv("GOFLAGS")}
	}
	return GoSettings{GOOS: env["GOOS"], GOARCH: env["GOARCH"], GOFLAGS: env["GOFLAGS"]}
}

// foreignTargetNote explains a binary that did not start: it is built for
// another platform than the host's and no -exec runs it. "" otherwise.
func foreignTargetNote(s GoSettings, hostOS, hostArch string, execPrefix []string) string {
	if len(execPrefix) > 0 || (s.GOOS == hostOS && s.GOARCH == hostArch) {
		return ""
	}
	return fmt.Sprintf("gotest: the test binary is built for %s/%s and this host is %s/%s: unset GOOS and GOARCH to test on the host, or give -exec a command that runs the binary\n", s.GOOS, s.GOARCH, hostOS, hostArch)
}
