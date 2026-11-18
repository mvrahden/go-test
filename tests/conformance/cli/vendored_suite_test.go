package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIVendoredTestSuite: a vendored module that lacks the runtime the
// harness imports is told how to bring it in.
type CLIVendoredTestSuite struct{}

func (s *CLIVendoredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute}
}

func (s *CLIVendoredTestSuite) TestAModuleVendoredWithoutTheRuntime(t *gotest.T) {
	dir := t.TempDir()
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	gotest.NoError(t, testkit.WriteModule(root, dir, "example.com/vendored"))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "vendored_suite_test.go"), []byte(`package vendored_test

import "github.com/mvrahden/go-test/pkg/gotest"

type VendoredTestSuite struct{}

func (s *VendoredTestSuite) TestOne(t *gotest.T) { gotest.True(t, true) }
`), 0o600))
	cmd := exec.CommandContext(t.Context(), "go", "mod", "vendor")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "go mod vendor: %s", out)
	run, err := testkit.RunCLIWithEnv(t.Context(), dir, []string{"GOWORK=off", "GOFLAGS="}, ".")
	gotest.NoError(t, err)
	t.When("a suite runs in a module whose vendor directory lacks pkg/gotestruntime", func(w *gotest.T) {
		w.It("fails to build with exit 2, naming the blank import to add and -mod=mod", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, `import _ "github.com/mvrahden/go-test/pkg/gotestruntime"`)
			gotest.Contains(it, run.Stderr, "or run with -mod=mod")
		})
	})
}
