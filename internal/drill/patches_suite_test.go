package drill_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// PatchCheckTestSuite: the dry run that names every patch that no longer
// applies, without writing a file.
type PatchCheckTestSuite struct{}

func (s *PatchCheckTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *PatchCheckTestSuite) SuiteGuard() string {
	if _, err := exec.LookPath("patch"); err != nil {
		return "patch not found"
	}
	return ""
}

// write creates path's directory and the file.
func write(t *gotest.T, path, content string) {
	gotest.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	gotest.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func (s *PatchCheckTestSuite) TestAStalePatchIsNamedAndNothingIsWritten(t *gotest.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "pkg", "a.go"), "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	const diff = "--- a/pkg/a.go\n+++ b/pkg/a.go\n@@ -1,5 +1,5 @@\n package a\n \n func A() int {\n-\treturn %s\n+\treturn 0\n }\n"
	mutants := filepath.Join(root, "tests", "drill", "mutants")
	write(t, filepath.Join(mutants, "fresh.patch"), "Expect: x\nJudge: ./pkg\n"+fmt.Sprintf(diff, "1"))
	write(t, filepath.Join(mutants, "stale.patch"), "Expect: x\nJudge: ./pkg\n"+fmt.Sprintf(diff, "2"))

	stale, checked, err := drill.StalePatches(root)

	gotest.NoError(t, err)
	gotest.Equal(t, []string{"stale"}, stale)
	gotest.Equal(t, 2, checked)
	src, err := os.ReadFile(filepath.Join(root, "pkg", "a.go"))
	gotest.NoError(t, err)
	gotest.Equal(t, "package a\n\nfunc A() int {\n\treturn 1\n}\n", string(src))
	left, _ := filepath.Glob(filepath.Join(root, "pkg", "*"))
	gotest.Len(t, left, 1, "no .rej or .orig beside the source")
}

func (s *PatchCheckTestSuite) TestARootWithoutMutantsIsAnError(t *gotest.T) {
	_, _, err := drill.StalePatches(t.TempDir())
	gotest.ErrorContains(t, err, "no mutants under")
}
