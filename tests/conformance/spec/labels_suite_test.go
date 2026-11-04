package spec_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecLabelsTestSuite: discover and every spec spell a behavior one way,
// underscores and connective included.
type SpecLabelsTestSuite struct {
	declared []string
	discover string
	live     map[string]bool
	static   map[string]bool
	replayed map[string]bool
}

const labelsFixture = "../testdata/e2elabels/"

// SuiteConfig: the live spec and the captured stream run the fixture through
// the whole pipeline.
func (s *SpecLabelsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SpecLabelsTestSuite) BeforeAll(t *gotest.T) {
	ctx := t.Context()
	disc, err := testkit.RunCLI(ctx, ".", "discover", labelsFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, disc.Exit, "discover: %s", disc.Stderr)
	s.discover = disc.Stdout
	s.declared = declaredLabels(t, disc.Stdout)

	live, err := testkit.RunCLI(ctx, ".", "spec", "--no-color", labelsFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, live.Exit, "spec: %s", live.Stderr)
	s.live = renderedLabels(live.Stdout)

	static, err := testkit.RunCLI(ctx, ".", "spec", "--static", "--no-color", labelsFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, static.Exit, "spec --static: %s", static.Stderr)
	s.static = renderedLabels(static.Stdout)

	stream, err := testkit.RunCLI(ctx, ".", "-json", labelsFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, stream.Exit, "-json: %s", stream.Stderr)
	events := filepath.Join(t.TempDir(), "events.json")
	gotest.NoError(t, os.WriteFile(events, []byte(stream.Stdout), 0o600))
	replay, err := testkit.RunCLI(ctx, ".", "spec", "--no-color", "--input="+events)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, replay.Exit, "spec --input: %s", replay.Stderr)
	s.replayed = renderedLabels(replay.Stdout)
}

func (s *SpecLabelsTestSuite) AfterAll(t *gotest.T) {}

type labelNode struct {
	Display  string      `json:"display"`
	Children []labelNode `json:"children"`
}

// declaredLabels is every behavior label discover reports, in source order.
func declaredLabels(t *gotest.T, stdout string) []string {
	var doc struct {
		Packages []struct {
			Suites []struct {
				Methods []struct {
					Behaviors []labelNode `json:"behaviors"`
				} `json:"methods"`
			} `json:"suites"`
		} `json:"packages"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(stdout), &doc), "discover: %s", stdout)
	var out []string
	var walk func([]labelNode)
	walk = func(ns []labelNode) {
		for _, n := range ns {
			out = append(out, n.Display)
			walk(n.Children)
		}
	}
	for _, p := range doc.Packages {
		for _, su := range p.Suites {
			for _, m := range su.Methods {
				walk(m.Behaviors)
			}
		}
	}
	return out
}

// renderedLabels is the set of rows a terminal spec shows, without their
// indentation, glyph and duration.
func renderedLabels(out string) map[string]bool {
	labels := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimLeft(strings.TrimSpace(line), "✓✗~ ")
		if i := strings.LastIndex(line, " ("); i > 0 && strings.HasSuffix(line, ")") {
			line = line[:i]
		}
		if line != "" {
			labels[line] = true
		}
	}
	return labels
}

func (s *SpecLabelsTestSuite) TestLabelsWithUnderscores(t *gotest.T) {
	t.When("a When and its Its hold underscores in their labels", func(w *gotest.T) {
		w.It("discover reports each label as written, the When's behind the word when", func(it *gotest.T) {
			gotest.Subset(it, s.declared, []string{
				"when the payload uses snake_case", "returns snake_case keys", "leaves CamelCase alone",
			}, "discover: %s", s.discover)
		})
		w.It("a run's spec renders each behavior under the label discover reports", func(it *gotest.T) {
			for _, label := range s.declared {
				gotest.True(it, s.live[label], "a run never renders %q; it renders %v", label, s.live)
			}
		})
		w.It("the spec read from source renders each behavior under that label", func(it *gotest.T) {
			for _, label := range s.declared {
				gotest.True(it, s.static[label], "--static never renders %q; it renders %v", label, s.static)
			}
		})
		w.It("the spec replayed from the run's -json stream renders each behavior under that label", func(it *gotest.T) {
			for _, label := range s.declared {
				gotest.True(it, s.replayed[label], "a replay never renders %q; it renders %v", label, s.replayed)
			}
		})
	})
}

func (s *SpecLabelsTestSuite) TestAConnectiveOfItsOwn(t *gotest.T) {
	t.When("a When's label opens with a connective of its own", func(w *gotest.T) {
		w.It("discover and every spec write it with no when in front", func(it *gotest.T) {
			gotest.Contains(it, s.declared, "with an empty payload", "discover: %s", s.discover)
			for name, surface := range map[string]map[string]bool{"run": s.live, "static": s.static, "replay": s.replayed} {
				gotest.True(it, surface["with an empty payload"], "the %s spec renders %v", name, surface)
				gotest.False(it, surface["when with an empty payload"], "the %s spec doubles the connective: %v", name, surface)
			}
		})
	})
}
