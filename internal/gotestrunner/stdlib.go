package gotestrunner

import (
	"os/exec"
)

func StdlibRunTests(args []string) (out []byte, code int, err error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	out, _ = cmd.CombinedOutput()
	return out, cmd.ProcessState.ExitCode(), nil
}
