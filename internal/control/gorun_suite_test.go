package control_test

import (
	"bytes"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoRunTestSuite proves go run hands the program its standard input, so a
// line written to a CLI started through go run reaches it.
type GoRunTestSuite struct{}

// SuiteConfig: go run compiles first, which a cold cache makes slow.
func (s *GoRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Timeout = 60 * time.Second
	return cfg
}

func (s *GoRunTestSuite) TestStdinReachesTheProgram(t *gotest.T) {
	cmd := exec.CommandContext(t.Context(), "go", "run", "./testdata/stdinecho")
	cmd.WaitDelay = 5 * time.Second
	stdin, err := cmd.StdinPipe()
	gotest.NoError(t, err)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	gotest.NoError(t, cmd.Start())
	_, writeErr := io.WriteString(stdin, "stop\n")
	waitErr := cmd.Wait()
	_ = stdin.Close()

	gotest.NoError(t, writeErr)
	gotest.NoError(t, waitErr, "go run: %s", stderr.String())
	gotest.Equal(t, "stop", strings.TrimRight(stdout.String(), "\r\n"))
}
