package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v4"
)

const FileName = ".gotest.yml"

// ProjectConfig holds settings loaded from .gotest.yml.
// CLI flags take precedence over these values; omitted keys fall back to
// defaults (duration keys are pointers, so an explicit 0 is honored as "disable").
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
	// Bench holds benchmark baseline/gate configuration.
	Bench BenchConfig `yaml:"bench"`
	// Fuzz holds fuzz seed-harvesting configuration.
	Fuzz FuzzConfig `yaml:"fuzz"`
}

// LintConfig controls which lint rules are disabled project-wide.
type LintConfig struct {
	// Skip lists lint rule names to disable (e.g. "stdlib-test", "testify").
	Skip []string `yaml:"skip"`
	// NolintAllow lists the names a //nolint directive may carry without
	// lint reporting them as unknown.
	NolintAllow []string `yaml:"nolint-allow"`
}

// BenchConfig controls default baseline comparison settings for
// "gotest bench". CLI flags (--against, --gate) take precedence over these.
type BenchConfig struct {
	// Baseline is the default path used for --against when not given on the
	// CLI.
	Baseline string `yaml:"baseline"`
	// Gate is the default regression gate percentage used for --gate when
	// not given on the CLI. Zero disables the gate.
	Gate float64 `yaml:"gate"`
}

// FuzzConfig controls default seed-harvesting behavior for generated fuzz
// targets. The CLI flag (--no-harvest) takes precedence over this value.
type FuzzConfig struct {
	// Harvest enables/disables harvesting literal seeds from table tests
	// into generated fuzz wrappers. Nil (the zero value) means harvesting
	// is ON by default; set to false to disable it project-wide.
	Harvest *bool `yaml:"harvest"`
}

// HarvestSeeds reports whether seed harvesting is enabled — true unless
// explicitly disabled via `fuzz: harvest: false`.
func (f FuzzConfig) HarvestSeeds() bool {
	return f.Harvest == nil || *f.Harvest
}

// Duration wraps time.Duration with human-readable YAML unmarshaling.
type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
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

	// Unknown keys are refused: a typo like `timeoutt` otherwise loads
	// cleanly and configures nothing. An empty or comment-only file is the
	// zero config, not an error.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg ProjectConfig
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return ProjectConfig{}, nil
		}
		return ProjectConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	// A later document that holds nothing, such as a trailing "---", is
	// read past; one with content is refused at its line.
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ProjectConfig{}, fmt.Errorf("%s: %w", path, err)
		}
		if line, ok := documentContent(&doc); ok {
			return ProjectConfig{}, fmt.Errorf("%s: line %d: holds more than one YAML document; gotest reads one", path, line)
		}
	}
	if err := checkStringLists(data); err != nil {
		return ProjectConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return ProjectConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// documentContent reports the line of a document's content; a document
// that is empty or holds only null has none.
func documentContent(doc *yaml.Node) (int, bool) {
	for _, n := range doc.Content {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
			continue
		}
		return n.Line, true
	}
	return 0, false
}

// validate refuses a value the key's flag would refuse. Zero stays legal for
// the counts, as an int cannot tell it from an omitted key.
func (c *ProjectConfig) validate() error {
	for _, kv := range []struct {
		key string
		n   int
	}{{"parallel", c.Parallel}, {"compile-parallel", c.CompileParallel}} {
		if kv.n < 0 {
			return fmt.Errorf("%s: %d is not a positive integer (omit the key for the default)", kv.key, kv.n)
		}
	}
	if c.MinCoverage < 0 || c.MinCoverage > 100 {
		return fmt.Errorf("min-coverage: %d is not a percentage from 0 to 100", c.MinCoverage)
	}
	return nil
}

// checkStringLists refuses a list element that is not a string: YAML
// decodes a number or a boolean into a string field without a word.
func checkStringLists(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return err
	}
	list := lookup(root.Content[0], "lint", "nolint-allow")
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	for _, el := range list.Content {
		if el.Kind != yaml.ScalarNode || el.Tag != "!!str" {
			return fmt.Errorf("line %d: lint.nolint-allow: %q is not a string", el.Line, el.Value)
		}
	}
	return nil
}

// lookup follows keys through nested mappings; nil when one is missing.
func lookup(n *yaml.Node, keys ...string) *yaml.Node {
	for _, key := range keys {
		if n.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				next = n.Content[i+1]
			}
		}
		if next == nil {
			return nil
		}
		n = next
	}
	return n
}
