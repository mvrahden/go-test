package gotestrunner

import (
	"bytes"
	"os"
	"os/exec"
)

func StdlibRunTestsJSON(args []string, extraEnv ...map[string]string) ([]byte, int, error) {
	jsonArgs := make([]string, 0, len(args)+2)
	jsonArgs = append(jsonArgs, "test", "-json")
	jsonArgs = append(jsonArgs, args...)

	cmd := exec.Command("go", jsonArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if len(extraEnv) > 0 && len(extraEnv[0]) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv[0] {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	err := cmd.Run()
	if cmd.ProcessState != nil {
		return stdout.Bytes(), cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return nil, 2, err
	}
	return stdout.Bytes(), 0, nil
}
