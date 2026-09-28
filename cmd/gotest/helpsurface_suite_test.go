package main_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// HelpSurfaceTestSuite is a drift guard: the help of a subcommand names every
// flag the subcommand accepts, and none it would refuse.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type HelpSurfaceTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *HelpSurfaceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HelpSurfaceTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

var helpFlagRe = regexp.MustCompile(`(?m)^\s+(--[a-z][a-z-]*)`)

func (s *HelpSurfaceTestSuite) TestHelpNamesTheFlagsASubcommandAccepts(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Name    string
		allowed map[string]bool
	}{
		{"test", ExportTestAllowed},
		{"spec", ExportSpecAllowed},
		{"summary", ExportSummaryAllowed},
		{"watch", ExportWatchAllowed},
		{"bench", ExportBenchAllowed},
		{"fuzz", ExportFuzzAllowed},
		{"migrate", ExportMigrateAllowed},
	}) {
		out, code := s.cli.runExit(t, "help", tc.Name)
		gotest.Equal(t, 0, code, "output:\n%s", out)

		listed := map[string]bool{}
		for _, m := range helpFlagRe.FindAllStringSubmatch(out, -1) {
			listed[m[1]] = true
		}
		gotest.Equal(t, sortedKeys(tc.allowed), sortedKeys(listed))
	}
}

// The discover document is an integration surface: every key it can carry is
// named where a consumer looks the schema up.
func (s *HelpSurfaceTestSuite) TestDiscoverSchemaIsDocumented(t *gotest.T) {
	keys := map[string]bool{}
	jsonKeys(reflect.TypeFor[ExportDiscoverOutput](), keys)

	help, code := s.cli.runExit(t, "help", "discover")
	gotest.Equal(t, 0, code, "output:\n%s", help)
	spec, err := os.ReadFile(filepath.Join(s.cli.repoRoot, "docs", "design", "spec.md"))
	gotest.NoError(t, err)
	_, section, found := strings.Cut(string(spec), "\n## Machine-Readable Discovery\n")
	gotest.True(t, found, "docs/design/spec.md has no Machine-Readable Discovery section")
	section, _, _ = strings.Cut(section, "\n## ")

	for _, key := range sortedKeys(keys) {
		t.It("names "+key, func(it *gotest.T) {
			gotest.Contains(it, help, `"`+key+`"`, "gotest help discover")
			gotest.Contains(it, section, `"`+key+`"`, "docs/design/spec.md, Discovery")
		})
	}
}

// jsonKeys collects the JSON keys of a struct type and of every struct it
// reaches through its fields.
func jsonKeys(typ reflect.Type, keys map[string]bool) {
	for typ.Kind() == reflect.Slice || typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	for i := range typ.NumField() {
		f := typ.Field(i)
		key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if key == "" || keys[key] && f.Type.Kind() != reflect.Slice {
			continue
		}
		seen := keys[key]
		keys[key] = true
		if !seen {
			jsonKeys(f.Type, keys)
		}
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
