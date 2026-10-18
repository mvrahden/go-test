package main_test

import (
	"os"
	"path/filepath"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CleanTestSuite covers which files clean takes for generated ones.
type CleanTestSuite struct{}

func (s *CleanTestSuite) TestGeneratedFiles(t *gotest.T) {
	t.When("a package holds current and legacy generated files", func(w *gotest.T) {
		dir := w.TempDir()
		for _, name := range []string{"gotest_psuite_test.go", "gotest_pxsuite_test.go", "ƒƒ_psuite_test.go", "ƒƒ_pxsuite_test.go", "own_test.go"} {
			gotest.NoError(w, os.WriteFile(filepath.Join(dir, name), []byte("package p\n"), 0o600))
		}
		var names []string
		for _, path := range ExportGeneratedFiles(dir) {
			names = append(names, filepath.Base(path))
		}
		gotest.ElementsMatch(w, []string{"gotest_psuite_test.go", "gotest_pxsuite_test.go", "ƒƒ_psuite_test.go", "ƒƒ_pxsuite_test.go"}, names)
	})
}
