package gotestgen_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// BuildTagsTestSuite: a generated file builds under the tags it was
// generated under, and only under them.
type BuildTagsTestSuite struct{}

func (s *BuildTagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *BuildTagsTestSuite) TestGeneratedConstraint(t *gotest.T) {
	t.When("generation runs with -tags", func(w *gotest.T) {
		w.It("the generated file opens with a //go:build line naming every tag", func(it *gotest.T) {
			src := generateTestsuite(it, []string{"-tags=foo,bar"})
			gotest.Regexp(it, `^//go:build foo && bar\n\n// Code generated`, src)
		})
		w.It("tags given as a separate argument and space-separated constrain it the same way", func(it *gotest.T) {
			src := generateTestsuite(it, []string{"-tags", "foo bar"})
			gotest.Regexp(it, `^//go:build foo && bar\n\n// Code generated`, src)
		})
	})
	t.When("generation runs without -tags", func(w *gotest.T) {
		w.It("the generated file carries no constraint", func(it *gotest.T) {
			src := generateTestsuite(it, nil)
			gotest.NotContains(it, src, "//go:build")
		})
	})
	t.When("the generated source already opens with a constraint", func(w *gotest.T) {
		w.It("the tags are joined to it in one //go:build line", func(it *gotest.T) {
			out := gotestgen.ExportWithBuildTags([]byte("//go:build linux || darwin\n\npackage p\n"), gotestgen.ExportTagsFromBuildFlags([]string{"-tags=foo"}))
			gotest.Equal(it, "//go:build (linux || darwin) && foo\n\npackage p\n", string(out))
		})
	})
}

// generateTestsuite generates testdata_e2e/testsuite under buildFlags and
// returns its internal test file.
func generateTestsuite(t *gotest.T, buildFlags []string) string {
	cwd, err := os.Getwd()
	gotest.NoError(t, err)
	loaded, broken, err := gotestgen.LoadPackages([]string{filepath.Join(cwd, "testdata_e2e", "testsuite")}, buildFlags)
	gotest.NoError(t, err)
	gotest.Empty(t, broken)
	results, _, err := gotestgen.GenerateFromLoaded(loaded)
	gotest.NoError(t, err)
	gotest.Len(t, results, 1)
	return string(results[0].PTest)
}
