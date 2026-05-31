package gotestrunner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

func StdlibRunTestsJSON(ctx context.Context, args []string, extraEnv ...map[string]string) ([]byte, int, error) {
	jsonArgs := make([]string, 0, len(args)+3)
	jsonArgs = append(jsonArgs, "test", "-json", "-ldflags=-checklinkname=0")
	jsonArgs = append(jsonArgs, args...)

	cmd := exec.CommandContext(ctx, "go", jsonArgs...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	setProcessGroup(cmd)

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
