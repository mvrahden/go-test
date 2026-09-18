package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// GotestrunnerProcessTestSuite tests how a suite subprocess is launched and how
// long it may take to tear down.
type GotestrunnerProcessTestSuite struct{}

func (s *GotestrunnerProcessTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *GotestrunnerProcessTestSuite) TestTeardownBudget(t *gotest.T) {
	t.When("reading budget file", func(w *gotest.T) {
		w.When("empty path", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				got := gotestrunner.ExportReadTeardownBudget("")
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("missing file", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				got := gotestrunner.ExportReadTeardownBudget("/nonexistent/budget")
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("valid duration", func(w *gotest.T) {
			w.It("returns parsed duration", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("2m30s\n"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				want := 2*time.Minute + 30*time.Second
				gotest.Equal(it, want, got)
			})
		})

		w.When("invalid duration", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("not-a-duration"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})

		w.When("zero duration", func(w *gotest.T) {
			w.It("returns default", func(it *gotest.T) {
				f := filepath.Join(it.TempDir(), "budget")
				_ = os.WriteFile(f, []byte("0s"), 0600)
				got := gotestrunner.ExportReadTeardownBudget(f)
				gotest.Equal(it, gotestrunner.GracefulShutdownDelay, got)
			})
		})
	})

	t.When("env injection in BuildSuiteCmd", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin"}

		w.When("BudgetFile is set", func(w *gotest.T) {
			w.It("sets GOTEST_TEARDOWN_BUDGET_FILE in env", func(it *gotest.T) {
				target := gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
					BinaryPath: "/tmp/pkg.test",
					BudgetFile: "/tmp/pkg.test.budget",
				}
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, false)
				gotest.Contains(it, cmd.Env, protocol.EnvTeardownBudgetFile+"=/tmp/pkg.test.budget", "GOTEST_TEARDOWN_BUDGET_FILE not found in cmd.Env")
			})
		})

		w.When("any target", func(w *gotest.T) {
			w.It("leaves process attributes to the process tree", func(it *gotest.T) {
				for _, test2json := range []bool{false, true} {
					target := gotestrunner.SuiteTarget{
						SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
						BinaryPath: "/tmp/pkg.test",
					}
					cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, test2json)
					gotest.Zero(it, cmd.SysProcAttr, "NewManagedProcess sets them")
				}
			})
		})

		w.When("BudgetFile is empty", func(w *gotest.T) {
			w.It("does not set GOTEST_TEARDOWN_BUDGET_FILE", func(it *gotest.T) {
				target := gotestrunner.SuiteTarget{
					SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
					BinaryPath: "/tmp/pkg.test",
				}
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, false)
				for _, e := range cmd.Env {
					gotest.False(it, strings.HasPrefix(e, protocol.EnvTeardownBudgetFile+"="),
						"unexpected GOTEST_TEARDOWN_BUDGET_FILE in env: %s", e)
				}
			})
		})
	})
}
