//go:build linux

package gotestrunner_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ProcFilesTestSuite covers reading a run's identity from /proc.
type ProcFilesTestSuite struct{}

func (s *ProcFilesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ProcFilesTestSuite) TestABootID(t *gotest.T) {
	t.It("reads the id without its line end", func(it *gotest.T) {
		path := filepath.Join(it.TempDir(), "boot_id")
		gotest.NoError(it, os.WriteFile(path, []byte("6f1c0d2e-4b1a-4c8e-9a51-0c2b9e1f7d33\n"), 0o600))
		gotest.Equal(it, "6f1c0d2e-4b1a-4c8e-9a51-0c2b9e1f7d33", gotestrunner.ExportReadBootID(path))
	})
	// A writer that cannot read its boot says so, rather than naming none.
	t.It("is unknown when the file cannot be read", func(it *gotest.T) {
		gotest.Equal(it, "unknown", gotestrunner.ExportReadBootID(filepath.Join(it.TempDir(), "boot_id")))
	})
	t.It("is unknown when the file is empty", func(it *gotest.T) {
		path := filepath.Join(it.TempDir(), "boot_id")
		gotest.NoError(it, os.WriteFile(path, nil, 0o600))
		gotest.Equal(it, "unknown", gotestrunner.ExportReadBootID(path))
	})
}

// A writer that cannot read its pid namespace says so, rather than naming
// none: a reader that cannot read its own either would take them as one.
func (s *ProcFilesTestSuite) TestAPidNamespaceThatCannotBeRead(t *gotest.T) {
	gotest.Equal(t, "unknown", gotestrunner.ExportReadPIDNamespace(filepath.Join(t.TempDir(), "pid")))
}
