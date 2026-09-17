package gotestrunner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

func StdlibRunTestsJSON(ctx context.Context, args []string, extraEnv ...map[string]string) ([]byte, int, error) {
	return StdlibRunTestsJSONIn(ctx, "", args, extraEnv...)
}

// StdlibRunTestsJSONIn is StdlibRunTestsJSON run from dir; an empty dir is the current directory.
func StdlibRunTestsJSONIn(ctx context.Context, dir string, args []string, extraEnv ...map[string]string) ([]byte, int, error) {
	jsonArgs := make([]string, 0, len(args)+3)
	jsonArgs = append(jsonArgs, "test", "-json", "-ldflags=-checklinkname=0")
	jsonArgs = append(jsonArgs, args...)

	cmd := exec.CommandContext(ctx, "go", jsonArgs...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if len(extraEnv) > 0 && len(extraEnv[0]) > 0 {
		cmd.Env = os.Environ()
		for k, v := range extraEnv[0] {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	err := runTree(cmd)
	if cmd.ProcessState != nil {
		return stdout.Bytes(), cmd.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return nil, 2, err
	}
	return stdout.Bytes(), 0, nil
}
