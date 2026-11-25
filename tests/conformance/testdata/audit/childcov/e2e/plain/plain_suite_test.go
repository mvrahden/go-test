package plain_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PlainTestSuite builds the plain binary without -cover and runs it. Under
// CHILDCOV_HOLD it first says it is under way and waits to be stopped.
type PlainTestSuite struct{}

func (s *PlainTestSuite) TestSaysBye(t *gotest.T) {
	if hold := os.Getenv("CHILDCOV_HOLD"); hold != "" {
		gotest.NoError(t, os.WriteFile(filepath.Join(hold, "ready"), nil, 0o600))
		<-t.Context().Done()
		return
	}
	bin := filepath.Join(t.TempDir(), "plain")
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", bin, "childcov/cmd/plain").CombinedOutput()
	gotest.NoError(t, err, "%s", out)
	out, err = exec.CommandContext(t.Context(), bin, "gotest").Output()
	gotest.NoError(t, err)
	gotest.Equal(t, "bye, gotest", strings.TrimSpace(string(out)))
}
