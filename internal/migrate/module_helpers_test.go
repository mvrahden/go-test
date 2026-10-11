package migrate_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/migrate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// pkgRun is a migrated module: its files after the run, by slash path.
type pkgRun struct {
	dir     string
	files   map[string]string
	results []migrate.MigrateResult
}

func (r pkgRun) markers() int {
	n := 0
	for i := range r.results {
		n += r.results[i].Markers
	}
	return n
}

// migrateModule writes files into a module of testify and the gotest under
// test, migrates what patterns (relative to the module root) name and reads
// every file back.
func migrateModule(t *gotest.T, mod *gotestcli.TestifyModuleSharedFixture, opts migrate.Options, files map[string]string, patterns ...string) pkgRun {
	root := t.TempDir()
	gotest.NoError(t, mod.Write(root, files))
	abs := make([]string, 0, len(patterns))
	for _, p := range patterns {
		abs = append(abs, filepath.Join(root, p))
	}
	results, err := migrate.MigratePackages(abs, opts)
	gotest.NoError(t, err)
	run := pkgRun{dir: root, files: map[string]string{}, results: results}
	for name := range files {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		gotest.NoError(t, err)
		run.files[name] = string(got)
	}
	return run
}
