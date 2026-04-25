package gotestrunner

import (
	"os"
	"os/exec"
)

func StdlibRunTests(args []string) (int, error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return 2, err
	}
	return 0, nil
}
