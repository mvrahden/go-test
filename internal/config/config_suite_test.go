package config_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConfigTestSuite covers .gotest.yml loading: the file is looked up from the
// working directory up to the module root, and durations keep the difference
// between "unset" and "zero".
type ConfigTestSuite struct{}

func (s *ConfigTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type configCtx struct{ dir string }

func (s *ConfigTestSuite) BeforeEach(t *gotest.T) *configCtx {
	return &configCtx{dir: t.TempDir()}
}

func writeFile(t *gotest.T, dir, name, content string) {
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func durationOf(t *gotest.T, field string, got *config.Duration) time.Duration {
	gotest.NotZero(t, got, "%s: got nil", field)
	return got.Duration()
}

func (s *ConfigTestSuite) TestLoad_FullConfig(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `
tags: "integration,e2e"
setup-timeout: 2m
timeout: 20m
min-coverage: 80
parallel: 12
compile-parallel: 2
debounce: 500ms
bench:
  baseline: bench-baseline.json
  gate: 10.5
fuzz:
  harvest: false
lint:
  skip:
    - stdlib-test
    - testify
`)

	cfg, err := config.Load(ctx.dir)
	gotest.NoError(t, err)

	gotest.Equal(t, "integration,e2e", cfg.Tags)
	gotest.Equal(t, 2*time.Minute, durationOf(t, "setup-timeout", cfg.SetupTimeout))
	gotest.Equal(t, 20*time.Minute, durationOf(t, "timeout", cfg.Timeout))
	gotest.Equal(t, 80, cfg.MinCoverage)
	gotest.Equal(t, 12, cfg.Parallel)
	gotest.Equal(t, 2, cfg.CompileParallel)
	gotest.Equal(t, 500*time.Millisecond, durationOf(t, "debounce", cfg.Debounce))
	gotest.Equal(t, []string{"stdlib-test", "testify"}, cfg.Lint.Skip)
	gotest.Equal(t, "bench-baseline.json", cfg.Bench.Baseline)
	gotest.Equal(t, 10.5, cfg.Bench.Gate)
	gotest.NotZero(t, cfg.Fuzz.Harvest)
	gotest.False(t, *cfg.Fuzz.Harvest)
	gotest.False(t, cfg.Fuzz.HarvestSeeds())
}

func (s *ConfigTestSuite) TestLoad_NoFile_ReturnsZero(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")

	cfg, err := config.Load(ctx.dir)
	gotest.NoError(t, err)

	gotest.Empty(t, cfg.Tags)
	gotest.Zero(t, cfg.SetupTimeout)
	gotest.Zero(t, cfg.Timeout)
	gotest.Equal(t, 0, cfg.MinCoverage)
	gotest.Equal(t, 0, cfg.Parallel)
	gotest.Equal(t, 0, cfg.CompileParallel)
	gotest.Zero(t, cfg.Debounce)
	gotest.Empty(t, cfg.Lint.Skip)
	gotest.Empty(t, cfg.Bench.Baseline)
	gotest.Zero(t, cfg.Bench.Gate)
	gotest.Zero(t, cfg.Fuzz.Harvest)
	gotest.True(t, cfg.Fuzz.HarvestSeeds())
}

func (s *ConfigTestSuite) TestLoad_PartialConfig(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `
tags: "unit"
min-coverage: 60
`)

	cfg, err := config.Load(ctx.dir)
	gotest.NoError(t, err)

	gotest.Equal(t, "unit", cfg.Tags)
	gotest.Equal(t, 60, cfg.MinCoverage)
	gotest.Zero(t, cfg.SetupTimeout)
	gotest.Zero(t, cfg.Debounce)
}

func (s *ConfigTestSuite) TestLoad_WalksUpToGoMod(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `tags: "found"`)
	sub := filepath.Join(ctx.dir, "pkg", "deep")
	gotest.NoError(t, os.MkdirAll(sub, 0o755))

	cfg, err := config.Load(sub)
	gotest.NoError(t, err)

	gotest.Equal(t, "found", cfg.Tags)
}

func (s *ConfigTestSuite) TestLoad_StopsAtGoMod(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, config.FileName, `tags: "should-not-find"`)
	sub := filepath.Join(ctx.dir, "nested")
	gotest.NoError(t, os.MkdirAll(sub, 0o755))
	writeFile(t, sub, "go.mod", "module nested\n")

	cfg, err := config.Load(sub)
	gotest.NoError(t, err)

	gotest.Empty(t, cfg.Tags)
}

func (s *ConfigTestSuite) TestLoad_ZeroDuration_IsNotNil(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `
setup-timeout: 0s
timeout: 0s
debounce: 0s
`)

	cfg, err := config.Load(ctx.dir)
	gotest.NoError(t, err)

	gotest.Equal(t, time.Duration(0), durationOf(t, "setup-timeout", cfg.SetupTimeout))
	gotest.Equal(t, time.Duration(0), durationOf(t, "timeout", cfg.Timeout))
	gotest.Equal(t, time.Duration(0), durationOf(t, "debounce", cfg.Debounce))
}

func (s *ConfigTestSuite) TestLoad_InvalidYAML(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `{{{invalid`)

	_, err := config.Load(ctx.dir)
	gotest.Error(t, err)
}

func (s *ConfigTestSuite) TestLoad_InvalidDuration(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, `setup-timeout: "not-a-duration"`)

	_, err := config.Load(ctx.dir)
	gotest.Error(t, err)
}
