package hello_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HelloTestSuite builds the hello binary as a coverage run asks, with
// -cover where GOCOVERDIR is set, and runs it.
type HelloTestSuite struct{}

func (s *HelloTestSuite) TestGreets(t *gotest.T) {
	bin := filepath.Join(t.TempDir(), "hello")
	args := []string{"build", "-o", bin}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover")
	}
	build := exec.CommandContext(t.Context(), "go", append(args, "childcov/cmd/hello")...)
	out, err := build.CombinedOutput()
	gotest.NoError(t, err, "%s", out)
	out, err = exec.CommandContext(t.Context(), bin, "gotest").Output()
	gotest.NoError(t, err)
	gotest.Equal(t, "hello, gotest", strings.TrimSpace(string(out)))
}
