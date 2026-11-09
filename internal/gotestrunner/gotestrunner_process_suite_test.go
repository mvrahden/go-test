package gotestrunner_test

import (
	"context"

	"github.com/mvrahden/go-test/internal/gotestrunner"
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

func (s *GotestrunnerProcessTestSuite) TestSuiteCommand(t *gotest.T) {
	t.When("BuildSuiteCmd builds a suite's command", func(w *gotest.T) {
		ctx := context.Background()
		env := []string{"PATH=/usr/bin"}
		target := gotestrunner.SuiteTarget{
			SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFoo"},
			BinaryPath: "/tmp/pkg.test",
		}

		w.It("gives it the caller's environment and nothing else, the channel being attached at start", func(it *gotest.T) {
			cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, false)
			gotest.Equal(it, env, cmd.Env)
		})
		w.It("leaves process attributes to the process tree", func(it *gotest.T) {
			for _, test2json := range []bool{false, true} {
				cmd := gotestrunner.ExportBuildSuiteCmd(ctx, target, env, test2json)
				gotest.Zero(it, cmd.SysProcAttr, "NewManagedProcess sets them")
			}
		})
	})
}
