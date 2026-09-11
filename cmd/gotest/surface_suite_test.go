package main_test

import (
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"go.yaml.in/yaml/v3"

	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SurfaceContractTestSuite checks that the CLI and the GitHub Action expose exactly the surface docs/design/spec.md tabulates.
type SurfaceContractTestSuite struct{}

func (s *SurfaceContractTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// TestCLISurfaceMatchesSpec is a drift guard: the CLI tables in docs/design/spec.md
// must stay in sync with the actual subcommand and flag registries.
func (s *SurfaceContractTestSuite) TestCLISurfaceMatchesSpec(t *gotest.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "design", "spec.md"))
	gotest.NoError(t, err)
	doc := string(data)

	t.When("comparing the Subcommands table", func(w *gotest.T) {
		documented := specTableEntries(doc, "### Subcommands")
		w.It("documents every registered subcommand", func(it *gotest.T) {
			for cmd := range ExportKnownSubcommands {
				gotest.True(it, documented[cmd], "subcommand %q missing from spec.md", cmd)
			}
		})
		w.It("documents no phantom subcommands", func(it *gotest.T) {
			for cmd := range documented {
				gotest.True(it, ExportKnownSubcommands[cmd], "spec.md documents unknown subcommand %q", cmd)
			}
		})
	})

	t.When("comparing the Flags table", func(w *gotest.T) {
		documented := specTableEntries(doc, "### Flags")
		w.It("documents every registered --flag", func(it *gotest.T) {
			for flag := range ExportGotestFlags {
				gotest.True(it, documented[flag], "flag %q missing from spec.md", flag)
			}
		})
		w.It("documents no phantom flags", func(it *gotest.T) {
			for flag := range documented {
				_, known := ExportGotestFlags[flag]
				gotest.True(it, known, "spec.md documents unknown flag %q", flag)
			}
		})
	})

	t.When("comparing the Linter rule tables", func(w *gotest.T) {
		documented := specTableEntriesUntil(doc, "## Linter", "\n## ")
		w.It("documents every registered lint rule", func(it *gotest.T) {
			for _, rule := range lint.RuleIDs() {
				gotest.True(it, documented[string(rule)], "lint rule %q missing from spec.md", rule)
			}
		})
		w.It("documents no phantom lint rules", func(it *gotest.T) {
			for id := range documented {
				gotest.True(it, lint.Known(lint.Rule(id)), "spec.md documents unknown lint rule %q", id)
			}
		})
	})
}

// TestActionSurfaceMatchesSpec is a drift guard: README.md's GitHub Actions
// inputs/outputs tables are the canonical documented action surface and must
// stay in sync with action.yml.
func (s *SurfaceContractTestSuite) TestActionSurfaceMatchesSpec(t *gotest.T) {
	actionRaw, err := os.ReadFile(filepath.Join("..", "..", "action.yml"))
	gotest.NoError(t, err)
	var action struct {
		Inputs  map[string]any `yaml:"inputs"`
		Outputs map[string]any `yaml:"outputs"`
	}
	gotest.NoError(t, yaml.Unmarshal(actionRaw, &action))

	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	gotest.NoError(t, err)
	doc := string(readme)

	t.When("comparing the Inputs table", func(w *gotest.T) {
		documented := specTableEntries(doc, "### Inputs")
		w.It("documents every action input", func(it *gotest.T) {
			for name := range action.Inputs {
				gotest.True(it, documented[name], "action input %q missing from README.md", name)
			}
		})
		w.It("documents no phantom inputs", func(it *gotest.T) {
			for name := range documented {
				_, known := action.Inputs[name]
				gotest.True(it, known, "README.md documents unknown action input %q", name)
			}
		})
	})

	t.When("comparing the Outputs table", func(w *gotest.T) {
		// The Outputs table is the last subsection of its ## section, so it
		// ends at the next ## heading, not at a ### one.
		documented := specTableEntriesUntil(doc, "### Outputs", "\n## ")
		w.It("documents every action output", func(it *gotest.T) {
			for name := range action.Outputs {
				gotest.True(it, documented[name], "action output %q missing from README.md", name)
			}
		})
		w.It("documents no phantom outputs", func(it *gotest.T) {
			for name := range documented {
				_, known := action.Outputs[name]
				gotest.True(it, known, "README.md documents unknown action output %q", name)
			}
		})
	})
}
