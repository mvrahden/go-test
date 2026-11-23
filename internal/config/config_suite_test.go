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

// A key the loader does not know is a typo the user will never see
// otherwise: `timeoutt: 1m` loaded cleanly and silently did nothing.
func (s *ConfigTestSuite) TestLoad_UnknownKey(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	writeFile(t, ctx.dir, config.FileName, "timeoutt: 1m\n")

	_, err := config.Load(ctx.dir)

	t.It("is an error naming the key and the file", func(it *gotest.T) {
		gotest.ErrorContains(it, err, "timeoutt")
		gotest.ErrorContains(it, err, filepath.Join(ctx.dir, config.FileName))
	})

	t.It("applies to nested keys too", func(it *gotest.T) {
		writeFile(it, ctx.dir, config.FileName, "lint:\n  skipp: [focus]\n")
		_, err := config.Load(ctx.dir)
		gotest.ErrorContains(it, err, "skipp")
	})
}

func (s *ConfigTestSuite) TestLoad_EmptyFile(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")
	for sub, tc := range gotest.Each(t, []struct {
		Desc    string
		content string
	}{
		{"empty file", ""},
		{"comment only", "# nothing configured yet\n"},
	}) {
		writeFile(sub, ctx.dir, config.FileName, tc.content)
		cfg, err := config.Load(ctx.dir)
		gotest.NoError(sub, err)
		gotest.Equal(sub, config.ProjectConfig{}, cfg)
	}
}

func (s *ConfigTestSuite) TestLoad_NolintAllow(t *gotest.T, ctx *configCtx) {
	writeFile(t, ctx.dir, "go.mod", "module test\n")

	t.It("reads a list of names", func(it *gotest.T) {
		writeFile(it, ctx.dir, config.FileName, "lint:\n  nolint-allow: [gocritic, \"errcheck\"]\n")
		cfg, err := config.Load(ctx.dir)
		gotest.NoError(it, err)
		gotest.Equal(it, []string{"gocritic", "errcheck"}, cfg.Lint.NolintAllow)
	})

	for sub, tc := range gotest.Each(t, []struct{ Desc, yaml, want string }{
		{"a single name, not a list", "lint:\n  nolint-allow: gocritic\n", "line 2: cannot construct !!str `gocritic` into []string"},
		{"a number", "lint:\n  nolint-allow:\n    - gocritic\n    - 12\n", `line 4: lint.nolint-allow: "12" is not a string`},
		{"a boolean", "lint:\n  nolint-allow: [true]\n", `line 2: lint.nolint-allow: "true" is not a string`},
		{"an empty element", "lint:\n  nolint-allow: [~]\n", `line 2: lint.nolint-allow: "~" is not a string`},
		{"a mapping", "lint:\n  nolint-allow: [{a: b}]\n", "line 2: cannot construct !!map into string"},
	}) {
		writeFile(sub, ctx.dir, config.FileName, tc.yaml)
		_, err := config.Load(ctx.dir)
		gotest.ErrorContains(sub, err, tc.want)
		gotest.ErrorContains(sub, err, config.FileName)
	}
}

func (s *ConfigTestSuite) TestLoad_ValuesAFlagWouldRefuse(t *gotest.T, _ *configCtx) {
	for sub, tc := range gotest.Each(t, []struct{ Desc, yaml, want string }{
		{"a negative parallel", "parallel: -3\n", "parallel: -3 is not a positive integer (omit the key for the default)"},
		{"a negative compile-parallel", "compile-parallel: -1\n", "compile-parallel: -1 is not a positive integer (omit the key for the default)"},
		{"a negative min-coverage", "min-coverage: -5\n", "min-coverage: -5 is not a percentage from 0 to 100"},
		{"a min-coverage above 100", "min-coverage: 150\n", "min-coverage: 150 is not a percentage from 0 to 100"},
		{"a second document", "parallel: 4\n---\nparallel: 8\n", "line 3: holds more than one YAML document; gotest reads one"},
		{"a second document after an empty one", "parallel: 4\n---\n---\n# set\nparallel: 8\n", "line 5: holds more than one YAML document; gotest reads one"},
	}) {
		dir := sub.TempDir()
		writeFile(sub, dir, "go.mod", "module test\n")
		writeFile(sub, dir, config.FileName, tc.yaml)
		_, err := config.Load(dir)
		gotest.ErrorContains(sub, err, tc.want)
		gotest.ErrorContains(sub, err, config.FileName)
	}
}

func (s *ConfigTestSuite) TestLoad_AnEmptyTrailingDocument(t *gotest.T, _ *configCtx) {
	for sub, content := range gotest.Each(t, []struct{ Desc, yaml string }{
		{"a trailing ---", "parallel: 4\n---\n"},
		{"a trailing --- with blank lines and comments", "parallel: 4\n---\n\n# nothing more\n\n"},
		{"a trailing --- holding only null", "parallel: 4\n---\n~\n"},
	}) {
		dir := sub.TempDir()
		writeFile(sub, dir, "go.mod", "module test\n")
		writeFile(sub, dir, config.FileName, content.yaml)
		cfg, err := config.Load(dir)
		gotest.NoError(sub, err)
		gotest.Equal(sub, 4, cfg.Parallel)
	}
}

func (s *ConfigTestSuite) TestLoad_ValuesAtTheirBounds(t *gotest.T, _ *configCtx) {
	for sub, content := range gotest.Each(t, []string{
		"parallel: 12\ncompile-parallel: 1\nmin-coverage: 100\ntimeout: 0s\n",
		"min-coverage: 0\n",
	}) {
		dir := sub.TempDir()
		writeFile(sub, dir, "go.mod", "module test\n")
		writeFile(sub, dir, config.FileName, content)
		_, err := config.Load(dir)
		gotest.NoError(sub, err)
	}
}
