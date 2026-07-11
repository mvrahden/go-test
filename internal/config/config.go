package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

const FileName = ".gotest.yml"

// ProjectConfig holds settings loaded from .gotest.yml.
// CLI flags take precedence over these values; nil/zero values are ignored.
type ProjectConfig struct {
	// Tags is a comma-separated list of build tags passed to go test (e.g. "integration,e2e").
	Tags string `yaml:"tags"`
	// SetupTimeout is the total budget for all shared fixture setup.
	// When omitted, the CLI default (2m) applies. Set to 0 to disable.
	SetupTimeout *Duration `yaml:"setup-timeout"`
	// Timeout is the global pipeline deadline.
	// When omitted, the CLI default (15m) applies. Set to 0 to disable.
	Timeout *Duration `yaml:"timeout"`
	// MinCoverage is the minimum coverage percentage (0-100). The run fails if coverage is below.
	MinCoverage int `yaml:"min-coverage"`
	// Parallel is the total concurrency budget (concurrent test methods across all
	// suite processes). Zero means use the default (2×GOMAXPROCS).
	Parallel int `yaml:"parallel"`
	// CompileParallel caps the number of concurrent `go test -c` compilation
	// processes. Zero means auto (NumCPU, halved when -race/-msan/-asan).
	CompileParallel int `yaml:"compile-parallel"`
	// Debounce is the delay before re-running tests in watch mode.
	// When omitted, the CLI default (200ms) applies.
	Debounce *Duration `yaml:"debounce"`
	// Lint holds lint-specific configuration.
	Lint LintConfig `yaml:"lint"`
}

// LintConfig controls which lint rules are disabled project-wide.
type LintConfig struct {
	// Skip lists lint rule names to disable (e.g. "stdlib-test", "testify").
	Skip []string `yaml:"skip"`
}

// Duration wraps time.Duration with human-readable YAML unmarshaling.
type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func Dur(d time.Duration) *Duration {
	v := Duration(d)
	return &v
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// Load finds and parses a .gotest.yml by walking from dir up to the
// filesystem root, stopping at the first match or at a go.mod boundary.
// Returns a zero ProjectConfig (not an error) if no config file exists.
func Load(dir string) (ProjectConfig, error) {
	path, err := find(dir)
	if err != nil {
		return ProjectConfig{}, err
	}
	if path == "" {
		return ProjectConfig{}, nil
	}
	return parse(path)
}

func find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func parse(path string) (ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ProjectConfig{}, nil
		}
		return ProjectConfig{}, err
	}

	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, err
	}
	return cfg, nil
}
