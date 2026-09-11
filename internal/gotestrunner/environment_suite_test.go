package gotestrunner_test

import (
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// EnvironmentTestSuite covers CI-mode detection from the environment.
// Sequential: Setenv.
type EnvironmentTestSuite struct{}

func (s *EnvironmentTestSuite) TestCIAutoDetection(t *gotest.T) {
	t.When("auto-detecting CI from environment", func(w *gotest.T) {
		w.It("sets CI when CI=true and GOTEST_CI is unset", func(it *gotest.T) {
			it.Setenv("CI", "true")
			it.Setenv(protocol.EnvCI, "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.True(it, cfg.CI)
		})

		w.It("does not override explicit --ci flag", func(it *gotest.T) {
			it.Setenv("CI", "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{CI: true})
			gotest.True(it, cfg.CI)
		})

		w.It("respects GOTEST_CI=0 opt-out", func(it *gotest.T) {
			it.Setenv("CI", "true")
			it.Setenv(protocol.EnvCI, "0")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.False(it, cfg.CI)
		})

		w.It("stays off when neither CI nor GOTEST_CI is set", func(it *gotest.T) {
			it.Setenv("CI", "")
			it.Setenv(protocol.EnvCI, "")

			cfg := gotestrunner.ExportAutoDetectCI(gotestrunner.PipelineConfig{})
			gotest.False(it, cfg.CI)
		})
	})

	t.When("propagating CI to subprocess env", func(w *gotest.T) {
		w.It("sets GOTEST_CI in extra env when CI is true", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{CI: true}, nil)
			gotest.Equal(it, "1", env[protocol.EnvCI])
		})

		w.It("omits GOTEST_CI in extra env when CI is false", func(it *gotest.T) {
			env := gotestrunner.ExportBuildExtraEnv(gotestrunner.PipelineConfig{}, nil)
			_, ok := env[protocol.EnvCI]
			gotest.False(it, ok)
		})

		w.It("appends GOTEST_CI to base env when CI is true", func(it *gotest.T) {
			it.Setenv("CI", "")
			it.Setenv(protocol.EnvCI, "")

			env := gotestrunner.ExportBuildBaseEnv(gotestrunner.PipelineConfig{CI: true})
			found := false
			for _, e := range env {
				if e == protocol.EnvCI+"=1" {
					found = true
					break
				}
			}
			gotest.True(it, found, "expected GOTEST_CI=1 in base env")
		})
	})
}
