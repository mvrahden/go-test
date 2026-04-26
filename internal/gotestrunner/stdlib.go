package gotestrunner

import (
	"os"
	"os/exec"
)

func StdlibRunTests(args []string, extraEnv ...map[string]string) (int, error) {
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if len(extraEnv) > 0 && len(extraEnv[0]) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv[0] {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	err := cmd.Run()
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return 2, err
	}
	return 0, nil
}
