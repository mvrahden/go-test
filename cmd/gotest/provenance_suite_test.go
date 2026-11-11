package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	main "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ProvenanceTestSuite: the module and the checkout a report names.
type ProvenanceTestSuite struct{}

func (s *ProvenanceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ProvenanceTestSuite) TestModulePath(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.24\n"), 0o600))
	nested := filepath.Join(dir, "a", "b")
	gotest.NoError(t, os.MkdirAll(nested, 0o750))
	t.It("reads the module of a directory below its go.mod", func(it *gotest.T) {
		gotest.Equal(it, "example.com/m", main.ExportModulePath(nested))
	})
	t.It("is empty outside a module", func(it *gotest.T) {
		gotest.Empty(it, main.ExportModulePath(filepath.Dir(dir)))
	})
}

func (s *ProvenanceTestSuite) TestReadProvenance(t *gotest.T) {
	t.It("is the zero value outside a checkout", func(it *gotest.T) {
		gotest.Zero(it, main.ExportReadProvenance(it.TempDir()))
	})
	t.It("names the checkout's commit", func(it *gotest.T) {
		got := main.ExportReadProvenance(".")
		want, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
		if err != nil {
			// Not a checkout, or no git: nothing to name.
			gotest.Zero(it, got)
			return
		}
		gotest.Equal(it, strings.TrimSpace(string(want)), got.Commit)
		gotest.NotZero(it, got.Branch)
	})
}
