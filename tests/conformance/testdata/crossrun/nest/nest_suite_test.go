// Package nest has an Exclusive suite that runs the gotest CLI CROSSRUN_CLI
// names on the hold package's Exclusive suite, under the run's own cache.
package nest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type NestTestSuite struct{}

func (s *NestTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Exclusive: true, Timeout: 30 * time.Second}
}

func (s *NestTestSuite) TestRunsGotest(t *gotest.T) {
	// The suite's budget ends a nested run that never ends on its own.
	cmd := exec.CommandContext(t.Context(), os.Getenv("CROSSRUN_CLI"), "-json", "-run", "TestHoldTestSuite", "../hold/") //nolint:gosec // G204: the CLI the test under way built
	cmd.Env = append(os.Environ(), "CROSSRUN_ROLE=nested", "CROSSRUN_HOLD=0")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	_ = os.WriteFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "nested-stderr"), []byte(stderr.String()), 0o600)
	gotest.NoError(t, err, "stderr: %s", stderr.String())
}
