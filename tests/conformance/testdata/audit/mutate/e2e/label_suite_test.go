package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LabelTestSuite builds the label binary, with -cover under GOCOVERDIR,
// and runs it: the only test of calc.Label.
type LabelTestSuite struct{}

func (s *LabelTestSuite) TestLabels(t *gotest.T) {
	bin := filepath.Join(t.TempDir(), "label")
	args := []string{"build", "-o", bin}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover")
	}
	out, err := exec.CommandContext(t.Context(), "go", append(args, "mutmod/cmd/label")...).CombinedOutput()
	gotest.NoError(t, err, "%s", out)
	for arg, want := range map[string]string{"1": "one", "2": "many"} {
		out, err := exec.CommandContext(t.Context(), bin, arg).Output()
		gotest.NoError(t, err)
		gotest.Equal(t, want, strings.TrimSpace(string(out)))
	}
}
