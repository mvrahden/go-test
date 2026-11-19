package docref_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ConfigTestSuite reads the config types of the real pkg/gotest: every field
// has a comment the reference is rendered from.
type ConfigTestSuite struct{}

func (s *ConfigTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

var pkgDir = filepath.Join("..", "..", "pkg", "gotest")

func (s *ConfigTestSuite) TestConfigFields(t *gotest.T) {
	for t, c := range gotest.Each(t, []struct {
		Name   string
		Fields []string
	}{
		{"SuiteConfig", []string{"Timeout", "SetupTimeout", "FailFast", "Parallel", "Exclusive", "ExclusiveGroup"}},
		{"FixtureConfig", []string{"Timeout", "Retries", "RetryDelay"}},
	}) {
		fields, err := docref.ConfigFields(pkgDir, c.Name)
		gotest.NoError(t, err)
		t.It("names the struct's fields in order", func(it *gotest.T) {
			names := make([]string, 0, len(fields))
			for _, f := range fields {
				names = append(names, f.Name)
			}
			gotest.Equal(it, c.Fields, names)
		})
		t.It("takes each field's comment", func(it *gotest.T) {
			for _, f := range fields {
				gotest.NotEmpty(it, f.Doc, "%s.%s has no comment in pkg/gotest; the reference is rendered from it", c.Name, f.Name)
			}
		})
	}
	t.It("reads a field's type and the first sentence of its comment", func(it *gotest.T) {
		fields, err := docref.ConfigFields(pkgDir, "SuiteConfig")
		gotest.NoError(it, err)
		gotest.Equal(it, docref.Field{Name: "Parallel", Type: "bool", Doc: "Parallel runs test methods concurrently."}, fields[3])
	})
	t.It("refuses a type the package does not declare", func(it *gotest.T) {
		_, err := docref.ConfigFields(pkgDir, "NoSuchConfig")
		gotest.ErrorContains(it, err, "NoSuchConfig is not a struct type of")
	})
}

func (s *ConfigTestSuite) TestPresets(t *gotest.T) {
	presets, err := docref.Presets(pkgDir)
	gotest.NoError(t, err)
	t.It("names every function returning a config", func(it *gotest.T) {
		var names []string
		for _, p := range presets {
			names = append(names, p.Name)
		}
		gotest.ElementsMatch(it, []string{"DefaultSuiteConfig", "IntegrationSuiteConfig", "DefaultFixtureConfig", "ContainerFixtureConfig"}, names)
	})
	t.It("names the type each returns", func(it *gotest.T) {
		for _, p := range presets {
			gotest.Regexp(it, "^(Suite|Fixture)Config$", p.Type)
			gotest.Regexp(it, p.Type+"$", p.Name, "%s returns %s", p.Name, p.Type)
		}
	})
}

func (s *ConfigTestSuite) TestConfigSections(t *gotest.T) {
	sections, err := docref.ConfigSections(pkgDir)
	gotest.NoError(t, err)
	t.It("lists the field names as inline code", func(it *gotest.T) {
		got, err := sections["suiteconfig/fields"]("list")
		gotest.NoError(it, err)
		gotest.Equal(it, "`Timeout`, `SetupTimeout`, `FailFast`, `Parallel`, `Exclusive`, `ExclusiveGroup`\n", got)
	})
	t.It("renders the fields as a table", func(it *gotest.T) {
		got, err := sections["fixtureconfig/fields"]("table")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^\\| Field \\| Type \\| Meaning \\|\n\\|---\\|---\\|---\\|\n\\| `Timeout` \\| `time.Duration` \\| Timeout is ", got)
	})
	t.It("renders the fields as HTML table rows", func(it *gotest.T) {
		got, err := sections["fixtureconfig/fields"]("html")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^<tr><td><code>Timeout</code></td><td><code>time.Duration</code></td><td>Timeout is ", got)
	})
	t.It("renders the presets as a table", func(it *gotest.T) {
		got, err := sections["config/presets"]("table")
		gotest.NoError(it, err)
		gotest.Contains(it, got, "| `DefaultSuiteConfig()` | `gotest.SuiteConfig` | DefaultSuiteConfig returns ")
	})
}
