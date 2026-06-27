package gotestrunner

import (
	"context"
	"os"
	"os/exec"
)

func StdlibRunTests(ctx context.Context, args []string, extraEnv ...map[string]string) (int, error) {
	cmd := exec.CommandContext(ctx, "go", append([]string{"test", "-ldflags=-checklinkname=0"}, args...)...) //nolint:gosec // G204: go tool with controlled arguments
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	SetProcessGroup(cmd)

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
