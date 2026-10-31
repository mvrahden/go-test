//go:build unix

package main_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

func (s *ScaffoldCommandTestSuite) TestSymlinkedPackagePath(t *gotest.T) {
	modDir, pkgDir := stageScaffoldModule(t, map[string]string{"gen.go": scaffoldGenericSrc})
	link := filepath.Join(t.TempDir(), "link")
	gotest.NoError(t, os.Symlink(modDir, link))
	run := s.scaffold(t, modDir, filepath.Join(link, "gen")+".Plain")
	gotest.Equal(t, 0, run.exit, "stdout: %s\nstderr: %s", run.stdout, run.stderr)
	_, err := os.Stat(filepath.Join(pkgDir, "plain_suite_test.go"))
	gotest.NoError(t, err, "the scaffold was not written into the package's directory")
}
