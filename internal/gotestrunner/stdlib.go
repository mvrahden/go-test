package gotestrunner

import (
	"os/exec"
)

func StdlibRunTests(args []string) (out []byte, code int, err error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	if len(args) > 0 {
		cmd.Args = append(cmd.Args, args...)
	}
	out, _ = cmd.CombinedOutput()
	return out, cmd.ProcessState.ExitCode(), nil
}
